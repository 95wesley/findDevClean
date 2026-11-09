#!/usr/bin/env bash
# Gera os pacotes de instalação em dist/release/:
#   findDevClean-<versão>-windows-amd64.zip
#   findDevClean-<versão>-linux-amd64.tar.gz
#   findDevClean-<versão>-linux-arm64.tar.gz
#   findDevClean-<versão>-macos-arm64.zip   (app nativo; só em um Mac)
#   SHA256SUMS.txt
#
# Uso: scripts/build-release.sh [versão]   (padrão: a versão do git ou 0.1.0)
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${1:-$(git describe --tags --always 2>/dev/null || echo 0.1.0)}"
VERSION="${VERSION#v}"
OUT="dist/release"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

rm -rf "$OUT" && mkdir -p "$OUT"

# A interface web é embutida no binário, então ela é compilada antes.
echo "==> Compilando a interface"
make ui >/dev/null

build_go() { # $1 = GOOS, $2 = GOARCH, $3 = nome do binário
  local dir="$STAGE/$1-$2"
  mkdir -p "$dir"
  GOOS="$1" GOARCH="$2" CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o "$dir/$3" ./cmd/findDevClean
  cp LICENSE README.md "$dir/"
}

echo "==> Windows amd64"
build_go windows amd64 findDevClean.exe
(cd "$STAGE/windows-amd64" && zip -q -r "$OLDPWD/$OUT/findDevClean-$VERSION-windows-amd64.zip" .)

echo "==> Linux amd64"
build_go linux amd64 findDevClean
tar -C "$STAGE/linux-amd64" -czf "$OUT/findDevClean-$VERSION-linux-amd64.tar.gz" .

echo "==> Linux arm64"
build_go linux arm64 findDevClean
tar -C "$STAGE/linux-arm64" -czf "$OUT/findDevClean-$VERSION-linux-arm64.tar.gz" .

if [[ "$(uname)" == "Darwin" ]]; then
  echo "==> macOS (app nativo)"
  ./scripts/build-mac-app.sh >/dev/null
  ditto -c -k --keepParent --norsrc --noextattr --noqtn dist/findDevClean.app "$OUT/findDevClean-$VERSION-macos-arm64.zip"
else
  echo "==> macOS pulado: o app só pode ser gerado em um Mac"
fi

(cd "$OUT" && shasum -a 256 ./*.zip ./*.tar.gz > SHA256SUMS.txt 2>/dev/null || true)
echo "==> Pronto em $OUT"
ls -la "$OUT"
