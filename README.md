# Stride

Extracteur Garmin Connect (Personal API) orienté rééducation fémoro-patellaire : sélection d'une séance, analyse selon 3 modes, et export en un clic d'un rapport markdown compact à coller dans un chat IA.

- **Backend** : Go (stdlib `net/http`, décodage FIT `muktihari/fit`), appels Garmin parallélisés, cache disque/mémoire, logs emoji.
- **Frontend** : React 19 + Vite + Tailwind 4, glassmorphism, mobile-first, embarqué dans le binaire Go.
- **Livraison** : une seule image Docker distroless (~12 Mo), un seul port.

## Démarrage

```bash
cp .env.example .env          # puis renseigner GARMIN_API_TOKEN
docker compose up -d --build
open http://127.0.0.1:8080
```

La clé se crée dans Garmin Connect Web → *Personal API Key* → *Create Key* (affichée une seule fois, expiration 30 j à 1 an). Elle peut aussi être saisie ou remplacée depuis l'app (⚙️ Paramètres). Dans ce cas elle est prioritaire sur `.env` et stockée dans `data/settings.json` (droits 0600).

## Modes

| Mode | Unité | Focus |
|---|---|---|
| Rééducation SFP | séance | Équilibre G/D, cadence (cible 175-182), GCT, oscillation/ratio vertical, foulée, D−, dérive cardiaque, blocs course/marche, nuit précédente |
| Santé & Récup | jour | VFC 7 j vs baseline, sommeil, FC repos, Body Battery, pas, **étages**, activités du jour |
| Performance | séance | Zones FC, puissance moy/normalisée, temps aux allures 10 km/marathon, ACWR (charge aiguë/chronique), best efforts |

Les seuils (cadence, OV, D−, écart G/D, FC repos, allures cibles, côté blessé) se règlent dans les paramètres.

> **Équilibre G/D** : la montre actuelle ne l'enregistre pas (il faut un HRM-Pro, un HRM 600 ou un Running Dynamics Pod). Le rapport affiche alors `N/D`, sans valeur inventée.

## Configuration (`.env`)

| Variable | Défaut | Rôle |
|---|---|---|
| `GARMIN_API_TOKEN` | — | Clé Personal API (alias acceptés : `GARMIN_PERSONAL_API_KEY`, `GARMIN_API_KEY`) |
| `APP_AUTH_USER` / `APP_AUTH_PASSWORD` | vides | Active le Basic Auth sur toute l'app (sauf `/api/health`) |
| `STRIDE_BIND` | `127.0.0.1` | Interface d'écoute publiée par compose |
| `STRIDE_PORT` | `8080` | Port publié |
| `LOG_LEVEL` | `info` | `debug` affiche les hits/miss du cache |
| `LOG_FORMAT` | `pretty` | `json` pour la prod |
| `TZ` | `Europe/Paris` | Fuseau horaire |

**Accès depuis le téléphone** : définir `STRIDE_BIND=0.0.0.0` **et** `APP_AUTH_USER`/`APP_AUTH_PASSWORD`. Sans cela, n'importe qui sur le réseau peut lire les données de santé et changer la clé.

## API (pour scripts / V2)

```bash
curl -s localhost:8080/api/activities?limit=5
curl -s "localhost:8080/api/activities/<id>/report?mode=sfp&history=14"   # rapport texte
curl -s localhost:8080/api/days/today/report                            # bilan du jour
```

Le contrat complet (JSON, formats de rapport, seuils d'alertes, faits vérifiés sur l'API Garmin) se trouve dans [docs/API_CONTRACT.md](docs/API_CONTRACT.md).

## Développement

```bash
make dev-backend     # Go sur :8080 (lit le .env racine)
make dev-frontend    # Vite sur :5173, proxy /api → :8080
cd frontend && npm run dev:mock   # UI seule sur fixtures (?mock=nokey|unauthorized|empty|slow)
make test            # go test + vitest
make docker-logs
```

## Données locales

`data/` (monté dans le conteneur) contient `settings.json`, `token.json` (JWT 24 h) et `cache/` (fichiers FIT, réponses Garmin). Les FIT contiennent du GPS. `data/` et `.env` sont exclus par `.gitignore` et `.dockerignore`. Le dossier peut être supprimé sans risque, il se reconstruit au premier appel.
