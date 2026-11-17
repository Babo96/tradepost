# deploy

The production deployment is managed from `~/git/homelab-argo`, following the
same pattern as the other apps.

```
homelab-argo/babo-forever/
  namespace.yaml
  postgres.yaml              # StatefulSet + Service (Postgres 18, rook-rbd 10Gi)
  api.yaml                   # Deployment + Service
  ingress.yaml               # babo-forever.b4b0.de, cert-manager letsencrypt
  sealedsecret-db.yaml       # DB password + DATABASE_URL
  sealedsecret-registry.yaml # git.babocraft.de image pull secret
homelab-argo/argocd/apps/babo-forever.yaml
```

- API: `https://babo-forever.b4b0.de`
- Namespace: `babo-forever`
- Image: `git.babocraft.de/babo/babo-forever-server:sha-452af9f`
- ArgoCD destination server: `https://192.168.2.114:6443`
- Project: `home-prod`
- Sync: automated prune/selfHeal, `CreateNamespace`, `ServerSideApply`

The container is built by Forgejo Actions (`.forgejo/workflows/docker-build.yml`
in the code repo) and published to the Forgejo container registry. Tags:
`sha-<7>`, `develop` on `main`.

Local development still uses `server/docker-compose.yml` and
`deploy/docker-compose.yml` (when added).
