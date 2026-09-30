#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/env.sh
if [[ "${1:-}" == "assign" ]]; then
  shift
  exec python3 scripts/atolye_assign.py "$@"
fi
exec python3 scripts/atolye_control.py "$@"
