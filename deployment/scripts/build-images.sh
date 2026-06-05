#!/usr/bin/env bash
# Build and optionally push PEP container images for Kubernetes.
# Usage: REGISTRY=ghcr.io/your-org/pep TAG=sit ./deployment/scripts/build-images.sh [--push]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
REGISTRY="${REGISTRY:-ghcr.io/your-org/pep}"
TAG="${TAG:-sit}"
PUSH=false

if [[ "${1:-}" == "--push" ]]; then
  PUSH=true
fi

cd "$ROOT"

GO_SERVICES=(
  dataset-replay:dataset-replay-service
  recommendation:recommendation-service
  engagement:engagement-orchestrator
  retention-analytics:retention-analytics-service
  ai-insights:ai-insights-service
)

for pair in "${GO_SERVICES[@]}"; do
  name="${pair%%:*}"
  dir="${pair##*:}"
  image="${REGISTRY}/${name}:${TAG}"
  echo "==> Building ${image}"
  docker build -f Dockerfile.go --build-arg SERVICE="${dir}" -t "${image}" .
  if $PUSH; then
    docker push "${image}"
  fi
done

frontend_image="${REGISTRY}/frontend:${TAG}"
echo "==> Building ${frontend_image}"
docker build -f frontend-dashboard/Dockerfile \
  --build-arg VITE_REPLAY_URL="" \
  --build-arg VITE_RECS_URL="" \
  --build-arg VITE_ENGAGEMENT_URL="" \
  --build-arg VITE_ANALYTICS_URL="" \
  --build-arg VITE_AI_URL="" \
  --build-arg VITE_WS_URL="" \
  -t "${frontend_image}" frontend-dashboard

if $PUSH; then
  docker push "${frontend_image}"
fi

echo "Done. Images tagged with ${TAG} under ${REGISTRY}"
