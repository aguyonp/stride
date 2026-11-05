# Stride

Stride récupère vos séances et vos données de récupération (Garmin Connect Personal API, Strava, météo Open-Meteo), calcule biomécanique, charge et récupération, et génère en un clic un prompt Markdown à coller dans votre coach IA. Service multi-utilisateur, connexion par clés d'accès (passkeys) uniquement.

## Démarrage

```bash
cp .env.example .env
openssl rand -base64 32        # à coller dans .env : STRIDE_MASTER_KEY=…
docker compose up -d --build
open http://localhost:8080
```

- **Le premier compte créé devient administrateur**, sans invitation. Les suivants passent par une invitation (`/admin`) ou par l'inscription ouverte.
- **Les passkeys exigent HTTPS**, sauf sur `localhost`. Derrière un reverse proxy, renseignez `STRIDE_PUBLIC_URL=https://…`.

## Configuration essentielle (`.env`)

| Variable | Défaut | Rôle |
|---|---|---|
| `STRIDE_MASTER_KEY` | **obligatoire** | Chiffre les clés Garmin et jetons Strava (`make keygen`) |
| `STRIDE_PUBLIC_URL` | `http://localhost:8080` | URL publique (passkeys, OAuth, liens d'invitation) |
| `STRIDE_REGISTRATION` | `invite` | Inscription initiale : `invite` ou `open` |
| `STRIDE_STRAVA_CLIENT_ID` / `_SECRET` | — | Active Strava |
| `STRIDE_HOST_BIND` / `STRIDE_HOST_PORT` | `127.0.0.1` / `8080` | Port publié par compose |

Toutes les variables sont décrites dans `.env.example` et dans [docs/V2_SPEC.md](docs/V2_SPEC.md) §9.

## Développement

```bash
make dev            # API Go sur :8080, cache mémoire
make dev-frontend   # Vite sur :5173
make test
```
