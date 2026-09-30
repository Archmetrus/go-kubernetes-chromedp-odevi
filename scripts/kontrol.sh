#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/env.sh
go test -race ./...
go vet ./...
go run ./01-go/giris/temeller
go run ./01-go/giris/struct-map
go run ./01-go/orta/worker-pool
go run ./03-chromedp/giris
go run ./03-chromedp/orta
