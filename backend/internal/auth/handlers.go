package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"stride/internal/httpx"
	"stride/internal/store"
)

const (
	nameMax          = 64
	webauthnBodyMax  = 64 << 10
	onUserCreatedMax = time.Minute
)

type Me struct {
	ID                  int64     `json:"id"`
	DisplayName         string    `json:"displayName"`
	Role                string    `json:"role"`
	CreatedAt           time.Time `json:"createdAt"`
	OnboardingCompleted bool      `json:"onboardingCompleted"`
}

func MeFrom(u *store.User) Me {
	return Me{ID: u.ID, DisplayName: u.DisplayName, Role: u.Role, CreatedAt: u.CreatedAt, OnboardingCompleted: u.OnboardingCompleted}
}

type Passkey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	BackedUp   bool       `json:"backedUp"`
}

func passkeyFrom(c store.Credential) Passkey {
	return Passkey{ID: base64.RawURLEncoding.EncodeToString(c.ID), Name: c.Name, CreatedAt: c.CreatedAt, LastUsedAt: c.LastUsedAt, BackedUp: c.BackupState}
}

type State struct {
	Authenticated    bool   `json:"authenticated"`
	User             *Me    `json:"user"`
	RegistrationMode string `json:"registrationMode"`
	FirstUser        bool   `json:"firstUser"`
	RPID             string `json:"rpId"`
}

// Register mounts /api/auth/* and /api/me* on mux.
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/state", s.state)
	mux.HandleFunc("POST /api/auth/register/begin", s.registerBegin)
	mux.HandleFunc("POST /api/auth/register/finish", s.registerFinish)
	mux.HandleFunc("POST /api/auth/login/begin", s.loginBegin)
	mux.HandleFunc("POST /api/auth/login/finish", s.loginFinish)
	mux.HandleFunc("POST /api/auth/login/recovery", s.loginRecovery)
	mux.HandleFunc("POST /api/auth/logout", s.logout)

	mux.Handle("GET /api/me", s.RequireUser(http.HandlerFunc(s.getMe)))
	mux.Handle("PATCH /api/me", s.RequireUser(http.HandlerFunc(s.patchMe)))
	mux.Handle("DELETE /api/me", s.RequireUser(http.HandlerFunc(s.deleteMe)))
	mux.Handle("GET /api/me/recovery", s.RequireUser(http.HandlerFunc(s.recoveryStatus)))
	mux.Handle("POST /api/me/recovery", s.RequireUser(http.HandlerFunc(s.regenerateRecovery)))
	mux.Handle("GET /api/me/passkeys", s.RequireUser(http.HandlerFunc(s.listPasskeys)))
	mux.Handle("POST /api/me/passkeys/begin", s.RequireUser(http.HandlerFunc(s.addPasskeyBegin)))
	mux.Handle("POST /api/me/passkeys/finish", s.RequireUser(http.HandlerFunc(s.addPasskeyFinish)))
	mux.Handle("PATCH /api/me/passkeys/{id}", s.RequireUser(http.HandlerFunc(s.renamePasskey)))
	mux.Handle("DELETE /api/me/passkeys/{id}", s.RequireUser(http.HandlerFunc(s.deletePasskey)))
}

func (s *Service) internal(w http.ResponseWriter, r *http.Request, what string, err error) {
	s.log.Error("❌ "+what, "path", r.URL.Path, "err", err.Error())
	httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "Erreur interne du serveur.")
}

func (s *Service) state(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st := State{RPID: s.rpID}
	if u, err := s.lookup(w, r); err == nil {
		me := MeFrom(u)
		st.Authenticated, st.User = true, &me
	}
	mode, err := s.store.RegistrationMode(ctx)
	if err != nil {
		s.internal(w, r, "Lecture du mode d'inscription", err)
		return
	}
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		s.internal(w, r, "Comptage des utilisateurs", err)
		return
	}
	st.RegistrationMode, st.FirstUser = mode, n == 0
	httpx.JSON(w, http.StatusOK, st)
}

func (s *Service) registerBegin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body struct {
		DisplayName string `json:"displayName"`
		InviteCode  string `json:"inviteCode"`
	}
	if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
		return
	}
	name := NormalizeName(body.DisplayName, nameMax)
	if name == "" {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Nom d'affichage requis (64 caractères maximum).")
		return
	}
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		s.internal(w, r, "Comptage des utilisateurs", err)
		return
	}
	mode, err := s.store.RegistrationMode(ctx)
	if err != nil {
		s.internal(w, r, "Lecture du mode d'inscription", err)
		return
	}
	var invite []byte
	if body.InviteCode != "" {
		invite = InviteHash(body.InviteCode)
	}
	if n > 0 && mode == store.RegistrationInvite {
		if invite == nil {
			httpx.Fail(w, http.StatusForbidden, httpx.CodeInviteRequired, "Une invitation est nécessaire pour créer un compte.")
			return
		}
		if err := s.store.CheckInvitation(ctx, invite, s.now()); err != nil {
			if errors.Is(err, store.ErrInviteInvalid) {
				httpx.Fail(w, http.StatusForbidden, httpx.CodeInviteRequired, "Invitation invalide, expirée ou déjà utilisée.")
				return
			}
			s.internal(w, r, "Vérification d'invitation", err)
			return
		}
	}
	handle := randomBytes(32)
	creation, sess, err := s.wa.BeginRegistration(&waUser{handle: handle, name: name},
		webauthn.WithExtensions(webauthn.WithExtensionCredProps()))
	if err != nil {
		s.internal(w, r, "Début d'inscription WebAuthn", err)
		return
	}
	if err := s.saveCeremony(ctx, w, ceremony{Kind: kindRegister, Session: *sess, DisplayName: name, Handle: handle, InviteHash: invite}); err != nil {
		s.internal(w, r, "Sauvegarde de cérémonie WebAuthn", err)
		return
	}
	httpx.JSON(w, http.StatusOK, creation)
}

func (s *Service) ceremonyError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errCeremony) {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Opération expirée ou invalide, veuillez recommencer.")
		return
	}
	s.internal(w, r, "Lecture de cérémonie WebAuthn", err)
}

func (s *Service) verifyFailed(w http.ResponseWriter, r *http.Request, err error) {
	var pe *protocol.Error
	detail := err.Error()
	if errors.As(err, &pe) {
		detail = pe.Type + ": " + pe.Details
	}
	s.log.Warn("🔐 Vérification WebAuthn refusée", "path", r.URL.Path, "reason", detail)
	httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "La clé d'accès n'a pas pu être vérifiée.")
}

func (s *Service) registerFinish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := s.takeCeremony(ctx, w, r, kindRegister)
	if err != nil {
		s.ceremonyError(w, r, err)
		return
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(http.MaxBytesReader(w, r.Body, webauthnBodyMax))
	if err != nil {
		s.verifyFailed(w, r, err)
		return
	}
	cred, err := s.wa.CreateCredential(&waUser{handle: c.Handle, name: c.DisplayName}, c.Session, parsed)
	if err != nil {
		s.verifyFailed(w, r, err)
		return
	}
	nc, err := newStoredCredential(cred, defaultPasskeyName(r.UserAgent(), s.now()))
	if err != nil {
		s.internal(w, r, "Encodage de la clé d'accès", err)
		return
	}
	u, err := s.store.RegisterUser(ctx, store.NewUser{WebAuthnID: c.Handle, DisplayName: c.DisplayName, InviteHash: c.InviteHash, Credential: nc}, s.now())
	switch {
	case errors.Is(err, store.ErrInviteRequired):
		httpx.Fail(w, http.StatusForbidden, httpx.CodeInviteRequired, "Une invitation est nécessaire pour créer un compte.")
		return
	case errors.Is(err, store.ErrInviteInvalid):
		httpx.Fail(w, http.StatusForbidden, httpx.CodeInviteRequired, "Invitation invalide, expirée ou déjà utilisée.")
		return
	case errors.Is(err, store.ErrDuplicateCredential):
		httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, "Cette clé d'accès est déjà enregistrée.")
		return
	case err != nil:
		s.internal(w, r, "Création du compte", err)
		return
	}
	firstAdmin := u.Role == store.RoleAdmin
	s.log.Info("👤 Nouveau compte créé", "userId", u.ID, "role", u.Role)
	if s.onCreated != nil {
		hctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), onUserCreatedMax)
		if err := s.onCreated(hctx, u.ID, firstAdmin); err != nil {
			s.log.Warn("⚠️ Initialisation des données du compte incomplète", "userId", u.ID, "err", err.Error())
		}
		cancel()
	}
	if err := s.issueSession(ctx, w, r, u.ID); err != nil {
		s.internal(w, r, "Création de session", err)
		return
	}
	httpx.JSON(w, http.StatusOK, MeFrom(u))
}

func (s *Service) loginBegin(w http.ResponseWriter, r *http.Request) {
	assertion, sess, err := s.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationPreferred))
	if err != nil {
		s.internal(w, r, "Début de connexion WebAuthn", err)
		return
	}
	if err := s.saveCeremony(r.Context(), w, ceremony{Kind: kindLogin, Session: *sess}); err != nil {
		s.internal(w, r, "Sauvegarde de cérémonie WebAuthn", err)
		return
	}
	httpx.JSON(w, http.StatusOK, assertion)
}

func (s *Service) loginFinish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := s.takeCeremony(ctx, w, r, kindLogin)
	if err != nil {
		s.ceremonyError(w, r, err)
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(http.MaxBytesReader(w, r.Body, webauthnBodyMax))
	if err != nil {
		s.verifyFailed(w, r, err)
		return
	}
	var (
		owner     *store.User
		lookupErr error
	)
	_, cred, err := s.wa.ValidatePasskeyLogin(func(_, userHandle []byte) (webauthn.User, error) {
		u, err := s.store.GetUserByWebAuthnID(ctx, userHandle)
		if err != nil {
			if !errors.Is(err, store.ErrUnknownUser) {
				lookupErr = err
			}
			return nil, err
		}
		owner = u
		wu, err := s.loadWAUser(ctx, u)
		if err != nil {
			lookupErr = err
			return nil, err
		}
		return wu, nil
	}, c.Session, parsed)
	if lookupErr != nil {
		s.internal(w, r, "Vérification de connexion", lookupErr)
		return
	}
	if err != nil {
		s.log.Warn("🔐 Connexion refusée", "reason", err.Error())
		httpx.Fail(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "Clé d'accès non reconnue.")
		return
	}
	if owner.Status != store.StatusActive {
		s.log.Warn("🔐 Connexion refusée : compte désactivé", "userId", owner.ID)
		httpx.Fail(w, http.StatusForbidden, httpx.CodeForbidden, "Ce compte est désactivé.")
		return
	}
	if cred.Authenticator.CloneWarning {
		s.log.Warn("⚠️ Compteur de signature incohérent (clé clonée ?)", "userId", owner.ID)
	}
	now := s.now()
	data, err := newStoredCredential(cred, "")
	if err == nil {
		err = s.store.UpdateCredentialUse(ctx, cred.ID, data.Data, cred.Flags.BackupState, now)
	}
	if err != nil {
		s.internal(w, r, "Mise à jour de la clé d'accès", err)
		return
	}
	if err := s.store.TouchUserLogin(ctx, owner.ID, now); err != nil {
		s.log.Warn("⚠️ Mise à jour de la dernière connexion impossible", "err", err.Error())
	}
	if err := s.issueSession(ctx, w, r, owner.ID); err != nil {
		s.internal(w, r, "Création de session", err)
		return
	}
	s.log.Info("🔑 Connexion", "userId", owner.ID)
	httpx.JSON(w, http.StatusOK, MeFrom(owner))
}

func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" && len(c.Value) <= 128 {
		if err := s.store.DeleteSession(r.Context(), tokenHash(c.Value)); err != nil {
			s.log.Warn("⚠️ Suppression de session impossible", "err", err.Error())
		}
	}
	s.clearSession(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) getMe(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, MeFrom(sessionUser(r)))
}

func (s *Service) patchMe(w http.ResponseWriter, r *http.Request) {
	u := sessionUser(r)
	var body struct {
		DisplayName         *string `json:"displayName"`
		OnboardingCompleted *bool   `json:"onboardingCompleted"`
	}
	if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
		return
	}
	if body.DisplayName != nil {
		name := NormalizeName(*body.DisplayName, nameMax)
		if name == "" {
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Nom d'affichage requis (64 caractères maximum).")
			return
		}
		body.DisplayName = &name
	}
	if err := s.store.UpdateUserProfile(r.Context(), u.ID, body.DisplayName, body.OnboardingCompleted); err != nil {
		s.internal(w, r, "Mise à jour du profil", err)
		return
	}
	nu, err := s.store.GetUser(r.Context(), u.ID)
	if err != nil {
		s.internal(w, r, "Lecture du profil", err)
		return
	}
	httpx.JSON(w, http.StatusOK, MeFrom(nu))
}

func (s *Service) deleteMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := sessionUser(r)
	if u.Role == store.RoleAdmin {
		admins, err := s.store.CountAdmins(ctx)
		if err != nil {
			s.internal(w, r, "Comptage des administrateurs", err)
			return
		}
		total, err := s.store.CountUsers(ctx)
		if err != nil {
			s.internal(w, r, "Comptage des utilisateurs", err)
			return
		}
		if admins <= 1 && total > 1 {
			httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, "Vous êtes le seul administrateur : nommez un autre administrateur avant de supprimer votre compte.")
			return
		}
	}
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		s.internal(w, r, "Suppression du compte", err)
		return
	}
	s.log.Info("🗑️ Compte supprimé par son titulaire", "userId", u.ID)
	s.clearSession(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) listPasskeys(w http.ResponseWriter, r *http.Request) {
	creds, err := s.store.ListCredentials(r.Context(), sessionUser(r).ID)
	if err != nil {
		s.internal(w, r, "Lecture des clés d'accès", err)
		return
	}
	out := make([]Passkey, 0, len(creds))
	for _, c := range creds {
		out = append(out, passkeyFrom(c))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (s *Service) addPasskeyBegin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := sessionUser(r)
	var body struct {
		Name string `json:"name"`
	}
	if r.ContentLength != 0 {
		if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
			return
		}
	}
	name := ""
	if body.Name != "" {
		if name = NormalizeName(body.Name, nameMax); name == "" {
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Nom de clé invalide (64 caractères maximum).")
			return
		}
	}
	wu, err := s.loadWAUser(ctx, u)
	if err != nil {
		s.internal(w, r, "Lecture des clés d'accès", err)
		return
	}
	creation, sess, err := s.wa.BeginRegistration(wu,
		webauthn.WithExclusions(webauthn.Credentials(wu.creds).CredentialDescriptors()),
		webauthn.WithExtensions(webauthn.WithExtensionCredProps()))
	if err != nil {
		s.internal(w, r, "Début d'ajout de clé WebAuthn", err)
		return
	}
	if err := s.saveCeremony(ctx, w, ceremony{Kind: kindAdd, Session: *sess, UserID: u.ID, Name: name}); err != nil {
		s.internal(w, r, "Sauvegarde de cérémonie WebAuthn", err)
		return
	}
	httpx.JSON(w, http.StatusOK, creation)
}

func (s *Service) addPasskeyFinish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := sessionUser(r)
	c, err := s.takeCeremony(ctx, w, r, kindAdd)
	if err == nil && c.UserID != u.ID {
		err = errCeremony
	}
	if err != nil {
		s.ceremonyError(w, r, err)
		return
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(http.MaxBytesReader(w, r.Body, webauthnBodyMax))
	if err != nil {
		s.verifyFailed(w, r, err)
		return
	}
	wu, err := s.loadWAUser(ctx, u)
	if err != nil {
		s.internal(w, r, "Lecture des clés d'accès", err)
		return
	}
	cred, err := s.wa.CreateCredential(wu, c.Session, parsed)
	if err != nil {
		s.verifyFailed(w, r, err)
		return
	}
	name := c.Name
	if name == "" {
		name = defaultPasskeyName(r.UserAgent(), s.now())
	}
	nc, err := newStoredCredential(cred, name)
	if err != nil {
		s.internal(w, r, "Encodage de la clé d'accès", err)
		return
	}
	switch err := s.store.AddCredential(ctx, u.ID, nc, s.now()); {
	case errors.Is(err, store.ErrDuplicateCredential):
		httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, "Cette clé d'accès est déjà enregistrée.")
		return
	case err != nil:
		s.internal(w, r, "Enregistrement de la clé d'accès", err)
		return
	}
	s.log.Info("🔑 Clé d'accès ajoutée", "userId", u.ID)
	s.writePasskey(w, r, u.ID, cred.ID, http.StatusOK)
}

func (s *Service) writePasskey(w http.ResponseWriter, r *http.Request, userID int64, id []byte, status int) {
	creds, err := s.store.ListCredentials(r.Context(), userID)
	if err != nil {
		s.internal(w, r, "Lecture des clés d'accès", err)
		return
	}
	for _, c := range creds {
		if string(c.ID) == string(id) {
			httpx.JSON(w, status, passkeyFrom(c))
			return
		}
	}
	httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Clé d'accès introuvable.")
}

func passkeyID(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	id, err := base64.RawURLEncoding.DecodeString(r.PathValue("id"))
	if err != nil || len(id) == 0 {
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Clé d'accès introuvable.")
		return nil, false
	}
	return id, true
}

func (s *Service) renamePasskey(w http.ResponseWriter, r *http.Request) {
	u := sessionUser(r)
	id, ok := passkeyID(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
		return
	}
	name := NormalizeName(body.Name, nameMax)
	if name == "" {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Nom de clé requis (64 caractères maximum).")
		return
	}
	switch err := s.store.RenameCredential(r.Context(), u.ID, id, name); {
	case errors.Is(err, store.ErrUnknownCredential):
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Clé d'accès introuvable.")
		return
	case err != nil:
		s.internal(w, r, "Renommage de la clé d'accès", err)
		return
	}
	s.writePasskey(w, r, u.ID, id, http.StatusOK)
}

func (s *Service) deletePasskey(w http.ResponseWriter, r *http.Request) {
	u := sessionUser(r)
	id, ok := passkeyID(w, r)
	if !ok {
		return
	}
	switch err := s.store.DeleteCredential(r.Context(), u.ID, id); {
	case errors.Is(err, store.ErrUnknownCredential):
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Clé d'accès introuvable.")
		return
	case errors.Is(err, store.ErrLastCredential):
		httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, "Impossible de supprimer votre dernière clé d'accès.")
		return
	case err != nil:
		s.internal(w, r, "Suppression de la clé d'accès", err)
		return
	}
	s.log.Info("🗑️ Clé d'accès supprimée", "userId", u.ID)
	w.WriteHeader(http.StatusNoContent)
}
