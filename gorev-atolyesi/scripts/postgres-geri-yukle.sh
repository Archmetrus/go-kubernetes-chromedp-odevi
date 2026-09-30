#!/usr/bin/env bash
# Restore only to an empty, separately provisioned database. No --clean/drop.
set -euo pipefail
: "${RESTORE_DATABASE_URL:?Ayrı boş hedef için RESTORE_DATABASE_URL gerekli}"
[[ $# -eq 1 && -f "$1" ]] || { printf '%s\n' 'Kullanım: bash scripts/postgres-geri-yukle.sh yedek.dump' >&2; exit 1; }
export PGDATABASE="$RESTORE_DATABASE_URL"
tables=$(psql --dbname="$PGDATABASE" -XAt -v ON_ERROR_STOP=1 -c "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'")
[[ "$tables" == 0 ]] || { printf '%s\n' 'Hedef boş değil; üzerine yazılmadı.' >&2; exit 1; }
pg_restore --exit-on-error --single-transaction --no-owner --no-acl --dbname="$PGDATABASE" "$1"
psql --dbname="$PGDATABASE" -X -v ON_ERROR_STOP=1 -c 'SELECT count(*) AS tasks FROM tasks; SELECT max(version) AS schema_version FROM schema_migrations;'
printf '%s\n' 'Yedek boş hedefe geri yüklendi. Uygulama bağlantısı otomatik değiştirilmedi.'
