#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/env.sh
export KUBECONFIG="$PWD/.kubeconfig"
docker info >/dev/null
if ! kind get clusters | grep -qx 'go-odev'; then
  kind create cluster --name go-odev --wait 120s
fi
kind export kubeconfig --name go-odev --kubeconfig "$KUBECONFIG"
docker build -t go-odev:1.0 .
kind load docker-image go-odev:1.0 --name go-odev
kubectl --context kind-go-odev apply -f 02-kubernetes/namespace.yaml
kubectl --context kind-go-odev apply -f 02-kubernetes/giris/pod.yaml
kubectl --context kind-go-odev apply -f 02-kubernetes/orta/uygulama.yaml
kubectl --context kind-go-odev -n odev wait --for=condition=Ready pod/merhaba-go --timeout=120s
kubectl --context kind-go-odev -n odev rollout status deployment/go-api --timeout=120s
