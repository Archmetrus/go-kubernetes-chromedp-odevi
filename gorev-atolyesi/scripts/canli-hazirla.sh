#!/usr/bin/env bash
# Generates reviewable manifests; never applies or publishes anything.
set -euo pipefail
cd "$(dirname "$0")/.."
: "${APP_DOMAIN:?APP_DOMAIN gerekli}"
: "${APP_IMAGE:?APP_IMAGE gerekli (registry/repo:tag veya @sha256)}"
: "${RELEASE_VERSION:?RELEASE_VERSION gerekli}"
environment="${DEPLOY_ENV:-staging}"
[[ "$environment" == staging || "$environment" == production ]]
[[ "$APP_DOMAIN" =~ ^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$ && "$APP_DOMAIN" == *.* && "$APP_DOMAIN" != *..* ]]
[[ "$APP_IMAGE" =~ ^[a-zA-Z0-9./:@_-]+$ && "$APP_IMAGE" != *:latest && ( "$APP_IMAGE" == *:* || "$APP_IMAGE" == *@sha256:* ) ]]
[[ "$RELEASE_VERSION" =~ ^[a-z0-9][a-z0-9-]{0,39}$ ]]
mkdir -p "deploy/rendered/$environment"
for name in app migrate; do
  sed -e "s|APP_DOMAIN|$APP_DOMAIN|g" -e "s|APP_IMAGE|$APP_IMAGE|g" -e "s|RELEASE_VERSION|$RELEASE_VERSION|g" -e "s|ENVIRONMENT|$environment|g" \
    "deploy/live/$name.yaml" > "deploy/rendered/$environment/$name.yaml"
done
if [[ -n "${BACKUP_IMAGE:-}" ]]; then
  [[ "$BACKUP_IMAGE" =~ ^[a-zA-Z0-9./:@_-]+$ && "$BACKUP_IMAGE" != *:latest && "$BACKUP_IMAGE" == *:* ]]
  sed "s|BACKUP_IMAGE|$BACKUP_IMAGE|g" deploy/live/backup.yaml > "deploy/rendered/$environment/backup.yaml"
fi
printf '%s\n' "Hazır: deploy/rendered/$environment. Dağıtım yapılmadı. Önce docs/CANLIYA-HAZIRLIK.md dosyasını okuyun."
