#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/env.sh
export KUBECONFIG="$PWD/.kubeconfig"
kubectl --context kind-go-odev -n gorev-atolyesi scale deployment/gorev-atolyesi --replicas=1
kubectl --context kind-go-odev -n gorev-atolyesi rollout status deployment/gorev-atolyesi --timeout=180s
printf '%s\n' 'Tarayıcıda http://127.0.0.1:8090 adresini açın. Durdurmak için Ctrl+C.'
exec kubectl --context kind-go-odev -n gorev-atolyesi port-forward service/gorev-atolyesi 8090:80
