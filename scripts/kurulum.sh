#!/usr/bin/env bash
# Linux x86_64 için kullanıcı dizinine kurulum; sudo kullanmaz.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || { echo 'Bu betik Linux x86_64 içindir.'; exit 1; }
tools_dir="$HOME/.local/share/odev-tools"
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT
mkdir -p "$tools_dir"
if [[ ! -x "$tools_dir/go/bin/go" ]]; then
  curl -fL --retry 2 https://go.dev/dl/go1.27.1.linux-amd64.tar.gz -o "$tmp_dir/go.tar.gz"
  printf '%s  %s\n' 63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445 "$tmp_dir/go.tar.gz" | sha256sum -c -
  tar -xzf "$tmp_dir/go.tar.gz" -C "$tools_dir"
fi
if [[ ! -x "$tools_dir/kubectl" ]]; then
  curl -fL https://dl.k8s.io/release/v1.37.0/bin/linux/amd64/kubectl -o "$tmp_dir/kubectl"
  curl -fL https://dl.k8s.io/release/v1.37.0/bin/linux/amd64/kubectl.sha256 -o "$tmp_dir/kubectl.sha256"
  printf '%s  %s\n' "$(cat "$tmp_dir/kubectl.sha256")" "$tmp_dir/kubectl" | sha256sum -c -
  install -m 755 "$tmp_dir/kubectl" "$tools_dir/kubectl"
fi
source scripts/env.sh
GOBIN="$tools_dir" go install sigs.k8s.io/kind@v0.33.0
go mod download
if [[ ! -x "$CHROME_PATH" ]]; then
  echo "Chrome bulunamadı: Chrome/Chromium kurup CHROME_PATH değişkenini ayarlayın."
fi
printf '%s\n' 'Kurulum tamamlandı. Fish terminalinizde: source scripts/env.fish' 'Kubernetes için docker info komutunun başarılı olması gerekir.'
