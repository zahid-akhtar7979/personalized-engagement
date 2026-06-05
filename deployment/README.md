# Deployment

| Environment | Method | Location |
|-------------|--------|----------|
| **Local** | Docker Compose (unchanged) | Root [`docker-compose.yml`](../docker-compose.yml) |
| **SIT** | Kubernetes | [`deployment/sit/`](sit/) |
| **PROD** | Kubernetes | [`deployment/prod/`](prod/) |

Local development is **not** affected by these manifests. Continue using:

```bash
docker compose up --build
```

---

## Kubernetes layout

```
deployment/
├── local/          # Points to root Docker Compose
├── sit/            # SIT namespace: pep-sit
│   ├── shared/     # Cross-service ConfigMap + Secret templates
│   ├── dataset-replay/
│   ├── recommendation/
│   ├── engagement/
│   ├── retention-analytics/
│   ├── ai-insights/
│   ├── frontend/   # Includes Ingress (public entrypoint)
│   └── kustomization.yaml
└── prod/           # PROD namespace: pep-prod (2 replicas, higher limits)
    └── (same structure)
```

Each service folder contains:

- `deployment.yaml` — Deployment with resource requests/limits, liveness & readiness probes
- `service.yaml` — ClusterIP Service
- `configmap.yaml` — Non-sensitive configuration (ports, broker URLs, models)
- `secret.yaml` — **Template** with `CHANGE_ME_*` placeholders (replace before apply)
- `ingress.yaml` — Frontend only (nginx in-pod proxies `/api/*` and `/ws/*` to backends)

---

## Prerequisites

1. Kubernetes cluster (1.24+) with an Ingress controller (e.g. NGINX Ingress).
2. Container registry access.
3. Managed or in-cluster **PostgreSQL**, **Redis**, and **Kafka** — update broker/DSN values in ConfigMaps and Secrets.
4. `kubectl` and `kustomize` (built into kubectl 1.14+).

---

## Build and push images

From the repository root:

```bash
REGISTRY=ghcr.io/your-org/pep
TAG=sit   # or prod

# Go microservices (shared Dockerfile.go)
for svc in dataset-replay recommendation engagement retention-analytics ai-insights; do
  docker build -f Dockerfile.go --build-arg SERVICE=${svc}-service \
    -t ${REGISTRY}/${svc}:${TAG} .
  docker push ${REGISTRY}/${svc}:${TAG}
done

# Frontend — empty VITE_* URLs = same-origin via nginx (matches local Docker Compose)
docker build -f frontend-dashboard/Dockerfile \
  --build-arg VITE_REPLAY_URL="" \
  --build-arg VITE_RECS_URL="" \
  --build-arg VITE_ENGAGEMENT_URL="" \
  --build-arg VITE_ANALYTICS_URL="" \
  --build-arg VITE_AI_URL="" \
  --build-arg VITE_WS_URL="" \
  -t ${REGISTRY}/frontend:${TAG} frontend-dashboard
docker push ${REGISTRY}/frontend:${TAG}
```

Update image names in `deployment/sit/kustomization.yaml` and `deployment/prod/kustomization.yaml` to match your registry.

---

## Configure secrets (required)

**Do not commit real secrets.** Edit placeholders in each `secret.yaml`, or create secrets via CI/CD:

```bash
# SIT example
kubectl create namespace pep-sit --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret generic recommendation-secret -n pep-sit \
  --from-literal=PEP_POSTGRES_DSN='host=... user=... password=... dbname=pep sslmode=require'

kubectl create secret generic ai-insights-secret -n pep-sit \
  --from-literal=PEP_POSTGRES_DSN='host=...' \
  --from-literal=PEP_OPENAI_API_KEY='sk-...'
```

Repeat for `retention-analytics-secret` and shared secrets as needed.

---

## Deploy SIT

```bash
# 1. Review and update ConfigMaps (Kafka, Redis, Postgres hostnames)
# 2. Replace secret placeholders
# 3. Apply all manifests
kubectl apply -k deployment/sit/

# 4. Load Retailrocket events.csv for dataset-replay (after PVC is bound)
kubectl -n pep-sit cp ./fallback-data/events.csv dataset-replay-<pod-id>:/data/events.csv

# 5. Verify
kubectl -n pep-sit get pods,svc,ingress
kubectl -n pep-sit rollout status deployment/recommendation
```

Ingress host (default): `pep-sit.example.com` — point DNS to your ingress controller and update `frontend/ingress.yaml` if needed.

---

## Deploy PROD

Same steps using `deployment/prod/`:

```bash
kubectl apply -k deployment/prod/
```

PROD differences:

- Namespace `pep-prod`
- **2 replicas** for stateless services (recommendation, engagement, retention-analytics, ai-insights, frontend)
- Higher CPU/memory requests and limits
- Ingress host: `pep.example.com`
- Image tag: `prod`

---

## Infrastructure endpoints

ConfigMaps reference in-cluster DNS names by default:

| Component | SIT default | PROD default |
|-----------|-------------|--------------|
| Kafka | `pep-kafka.pep-sit.svc.cluster.local:9092` | `pep-kafka.pep-prod.svc.cluster.local:9092` |
| Redis | `pep-redis.pep-sit.svc.cluster.local:6379` | `pep-redis.pep-prod.svc.cluster.local:6379` |
| Postgres | via `PEP_POSTGRES_DSN` secret | via `PEP_POSTGRES_DSN` secret |

For managed cloud services (RDS, ElastiCache, MSK), replace these values in the per-service `configmap.yaml` and `secret.yaml` files — nothing is hardcoded in application code.

---

## Health checks

All Go services expose `GET /health` on their service port. Probes are configured in each `deployment.yaml`:

- **Liveness** — restarts unhealthy pods
- **Readiness** — removes pods from Service endpoints until ready

Frontend uses `GET /` on port 80.

---

## Frontend routing

The frontend pod mounts a Kubernetes-specific nginx config (`frontend/configmap.yaml`) that proxies API traffic to internal ClusterIP services — same behavior as local `frontend-dashboard/nginx.conf`, but using cluster DNS names.

Public traffic enters only through **Ingress → frontend Service**. Backend services are not exposed externally.
