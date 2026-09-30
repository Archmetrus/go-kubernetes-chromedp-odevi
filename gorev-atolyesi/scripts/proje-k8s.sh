#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/env.sh
export KUBECONFIG="$PWD/.kubeconfig"
if ! docker info >/dev/null; then
  echo 'Docker erişimi yok. Servisi ve mevcut oturumun docker grup üyeliğini kontrol edin.' >&2
  exit 1
fi
if ! kind get clusters | grep -qx go-odev; then
  kind create cluster --name go-odev --wait 120s
fi
kind export kubeconfig --name go-odev --kubeconfig "$KUBECONFIG"
# Unique image tag ensures each build triggers a real rollout.
image_tag="gorev-atolyesi:$(date +%Y%m%d%H%M%S)-$$"
docker build -f deploy/gorev-atolyesi/Dockerfile -t "$image_tag" .
kind load docker-image "$image_tag" --name go-odev
sed "s|gorev-atolyesi:1.0|$image_tag|" deploy/gorev-atolyesi/kubernetes.yaml | kubectl --context kind-go-odev apply -f -
kubectl --context kind-go-odev -n gorev-atolyesi rollout status deployment/gorev-atolyesi --timeout=180s
printf '%s\n' 'Hazır. Fish terminalinde: bash scripts/proje-ac.sh'
