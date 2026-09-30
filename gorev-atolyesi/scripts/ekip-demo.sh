#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/env.sh
export APP_ENV=development
unset DATABASE_URL APP_ORIGIN
export DB_PATH="$PWD/data/demo/ekip.db"
export ATOLYE_CONTROL_DIR="$PWD/.control/demo"
export ATOLYE_LOCAL_ONLY=true
export MFA_KEY_FILE="$ATOLYE_CONTROL_DIR/mfa.key"
export DEMO_ACCOUNTS_FILE="$ATOLYE_CONTROL_DIR/accounts.json"
export AUTH_ENABLED=true ACCOUNT_APPROVAL=true ALLOW_SIGNUP=true
export ADDR=127.0.0.1:8092
export ATOLYE_API_URL=http://127.0.0.1:8092
export ATOLYE_API_TOKEN_FILE="$ATOLYE_CONTROL_DIR/automation.token"
umask 077
mkdir -p "$ATOLYE_CONTROL_DIR" "$PWD/data/demo"
case "${1:-tui}" in
 assign) shift; exec bash scripts/atolye.sh assign "$@" --preview-file "$ATOLYE_CONTROL_DIR/assignment-preview.json" ;;
 seed)
  if [[ -e "$DEMO_ACCOUNTS_FILE" ]]; then
   echo "Demo zaten oluşturulmuş; hesaplar ve görevler korundu. Menü: bash scripts/ekip-demo.sh tui"
   exit 0
  fi
  export ALLOW_DEMO_SEED=true
  exec go run ./cmd/gorev-atolyesi demo-seed ;;
 accounts)
  exec python3 scripts/demo_accounts.py ;;
 tui) exec bash scripts/atolye.sh control tui ;;
 up|down|restart|status|logs|open) exec bash scripts/atolye.sh system "$1" --profile local ;;
 *) echo "Kullanım: bash scripts/ekip-demo.sh seed|accounts|tui|up|down|restart|status|logs|open|assign" >&2; exit 2 ;;
esac
