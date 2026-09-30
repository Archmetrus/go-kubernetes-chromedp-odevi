#!/usr/bin/env bash
# pg_dump gives a consistent snapshot while the app remains online.
set -euo pipefail
umask 077
: "${DATABASE_URL:?DATABASE_URL gerekli}"
command -v pg_dump >/dev/null
target_dir="${BACKUP_DIR:-$PWD/backups}"
mkdir -p "$target_dir"
backup_file=$(mktemp "$target_dir/atolye-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX")
export PGDATABASE="$DATABASE_URL"
if ! pg_dump --dbname="$PGDATABASE" --format=custom --no-owner --no-acl --file="$backup_file"; then
  printf '%s\n' "Yedek başarısız; eksik dosya: $backup_file" >&2
  exit 1
fi
pg_restore --list "$backup_file" >/dev/null
printf '%s\n' "Yedek hazır: $backup_file"
if [[ -n "${BACKUP_S3_URI:-}" ]]; then
  [[ "$BACKUP_S3_URI" == s3://* ]] || { printf '%s\n' 'BACKUP_S3_URI s3:// ile başlamalı' >&2; exit 1; }
  aws s3 cp "$backup_file" "${BACKUP_S3_URI%/}/$(basename "$backup_file")" --only-show-errors
  printf '%s\n' 'Nesne depolamaya gönderildi.'
fi
