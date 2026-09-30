#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/env.sh
read -r -p 'E-posta: ' USER_EMAIL
read -r -s -p 'Parola (en az 12 karakter): ' USER_PASSWORD
printf '\n'
export USER_EMAIL USER_PASSWORD
go run ./cmd/gorev-atolyesi create-user
unset USER_PASSWORD
printf '%s\n' 'Hesap oluşturuldu. AUTH_ENABLED=true ile sunucuyu açabilirsiniz.'
