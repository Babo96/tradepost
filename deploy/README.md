# deploy

Phase 5.

- `k8s/` — namespace, API Deployment/Service, Postgres StatefulSet + PVC,
  Traefik IngressRoute, cert-manager Certificate, SealedSecrets, backup
  CronJob, Prometheus/Gatus annotations.
- `docker-compose.yml` — local API + Postgres for development.

The ArgoCD Application will live in `~/git/homelab-argo/babo-forever/` and point
at this repository's `deploy/k8s` path. Hostname: `babo-forever.b4b0.de`.

Nothing here is deployed yet.
