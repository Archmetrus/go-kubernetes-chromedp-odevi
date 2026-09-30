#!/usr/bin/env bash
# Explicit context and environment prevent accidental use of the local cluster.
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/env.sh
: "${DEPLOY_CONTEXT:?Hedef kubectl context adı gerekli}"
: "${DEPLOY_ENV:?staging veya production gerekli}"
[[ "$DEPLOY_ENV" == staging || "$DEPLOY_ENV" == production ]]
bash scripts/canli-hazirla.sh
namespace="gorev-$DEPLOY_ENV"
kube=(kubectl --context "$DEPLOY_CONTEXT" --namespace "$namespace")
"${kube[@]}" get secret gorev-secrets gorev-tls >/dev/null
# Production migration must follow a successful off-cluster backup.
if [[ "$DEPLOY_ENV" == production ]]; then
  backup_job="gorev-before-$RELEASE_VERSION"
  "${kube[@]}" create job "$backup_job" --from=cronjob/gorev-yedek
  "${kube[@]}" wait --for=condition=complete "job/$backup_job" --timeout=1800s
fi
"${kube[@]}" apply -f "deploy/rendered/$DEPLOY_ENV/migrate.yaml"
"${kube[@]}" wait --for=condition=complete "job/gorev-migrate-$RELEASE_VERSION" --timeout=120s
"${kube[@]}" apply -f "deploy/rendered/$DEPLOY_ENV/app.yaml"
"${kube[@]}" rollout status deployment/gorev-atolyesi --timeout=180s
curl --fail --silent --show-error "https://$APP_DOMAIN/readyz"
printf '\n%s\n' 'Dağıtım ve HTTPS hazır olma kontrolü başarılı.'
