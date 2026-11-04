package auth

import "context"

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// User is the authenticated principal injected by the session middleware.
type User struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

type ctxKey struct{}

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}
