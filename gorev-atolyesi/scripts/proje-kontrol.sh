#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/env.sh
go test -race ./...
go vet ./...
go run ./cmd/gorev-e2e
go run ./cmd/identity-e2e
go run ./cmd/teams-e2e
go run ./cmd/automation-e2e
python3 -m unittest discover -s scripts -p 'test_atolye*.py' -v
