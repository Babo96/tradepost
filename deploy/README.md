# deploy

TradePost is deployed with GitOps. The production manifests live in the
external **`homelab-argo`** repository under `tradepost/`; this repository
contains the application and the build workflow.

```
homelab-argo/tradepost/
  namespace.yaml
  postgres.yaml              # Postgres 18 StatefulSet + Service (rook-rbd)
  api.yaml                   # API Deployment + Service
  ingress.yaml               # tradepost.b4b0.de, Traefik + cert-manager
  sealedsecret-db.yaml       # DB user/password + DATABASE_URL
homelab-argo/argocd/apps/tradepost.yaml
```

- URL: <https://tradepost.b4b0.de>
- Namespace: `tradepost`
- Image: `ghcr.io/babo96/tradepost-server` (public; no registry pull secret)
- Database: Postgres 18, `tradepost`/`tradepost` database, PVC on `rook-rbd`
- TLS: cert-manager `letsencrypt`, ingress class `traefik`
- Argo CD: automated sync with prune/self-heal, `CreateNamespace`,
  `ServerSideApply`
- Project: `home-prod`

## Image builds

GitHub Actions (`.github/workflows/docker-build.yml`) builds the server image
from `server/` and pushes it to GHCR:

- `sha-<short>` on every push (the Argo manifest pins one of these),
- `develop` on `main`.

The workflow is public; the image does not need a pull secret. To deploy a new
build, update the image tag in `homelab-argo/tradepost/api.yaml`, commit, and
let Argo CD sync.

## Database

The database credentials are stored in a sealed secret (`tradepost-db`) with
keys `POSTGRES_USER`, `POSTGRES_DB`, `POSTGRES_PASSWORD` and `DATABASE_URL`.
The API reads `TRADEPOST_DATABASE_URL` from that secret. Migrations are
embedded in the server binary and run at startup.

The database was migrated from the previous deployment once; the current stack
owns the data. There is no automatic migration from the old namespace.

## Local development

Use the server's Compose file and Make targets instead of the cluster:

```sh
make db-up
make server-run
```

See [../server/README.md](../server/README.md) for environment variables,
endpoints and tests.
