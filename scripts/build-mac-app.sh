#!/usr/bin/env bash
# Gera dist/findDevClean.app: um app nativo de macOS. Ele abre uma janela
# própria (WebKit) com a interface, sobe o servidor Go embutido e o encerra ao sair.
#
# Uso:
#   scripts/build-mac-app.sh             gera dist/findDevClean.app
#   scripts/build-mac-app.sh --install   gera e copia para ~/Applications
#
# Requisitos: Go, Node/npm (compila a UI) e as ferramentas do Xcode/CLT (swiftc, sips, iconutil).
set -euo pipefail

cd "$(dirname "$0")/.."

APP_NAME="findDevClean"
APP="dist/${APP_NAME}.app"
VERSION=$(git describe --tags --always 2>/dev/null || echo "0.1.0")

if [[ "$(uname)" != "Darwin" ]]; then
  echo "Este script gera um app para macOS. Rode-o em um Mac." >&2
  exit 1
fi

echo "==> Compilando a interface e o binário"
make build >/dev/null

echo "==> Montando ${APP}"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp bin/findDevClean "$APP/Contents/Resources/findDevClean"

# Janela nativa: scripts/mac-app/main.swift (WKWebView + controle do servidor).
echo "==> Compilando a janela nativa"
swiftc -O -target "$(uname -m)-apple-macos13.0" \
  -framework Cocoa -framework WebKit \
  -o "$APP/Contents/MacOS/${APP_NAME}" scripts/mac-app/main.swift

# Ícone: o logo SVG vira PNG (qlmanage) e depois um .icns (iconutil).
echo "==> Gerando o ícone"
TMP=$(mktemp -d)
qlmanage -t -s 1024 -o "$TMP" web/public/logo.svg >/dev/null 2>&1
SRC="$TMP/logo.svg.png"
ICONSET="$TMP/AppIcon.iconset"
mkdir -p "$ICONSET"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" "$SRC" --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
  sips -z "$((size * 2))" "$((size * 2))" "$SRC" --out "$ICONSET/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/AppIcon.icns"
rm -rf "$TMP"

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>${APP_NAME}</string>
	<key>CFBundleDisplayName</key><string>${APP_NAME}</string>
	<key>CFBundleIdentifier</key><string>local.findclean.app</string>
	<key>CFBundleVersion</key><string>${VERSION}</string>
	<key>CFBundleShortVersionString</key><string>${VERSION}</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleExecutable</key><string>${APP_NAME}</string>
	<key>CFBundleIconFile</key><string>AppIcon</string>
	<key>LSMinimumSystemVersion</key><string>13.0</string>
	<key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST

chmod +x "$APP/Contents/MacOS/${APP_NAME}"

# Assinatura ad-hoc: o app roda localmente sem aviso de "desenvolvedor não identificado".
codesign --force --sign - "$APP" >/dev/null 2>&1 || true

echo "==> Pronto: $APP"

if [[ "${1:-}" == "--install" ]]; then
  mkdir -p "$HOME/Applications"
  rm -rf "$HOME/Applications/${APP_NAME}.app"
  cp -R "$APP" "$HOME/Applications/"
  echo "==> Instalado em ~/Applications/${APP_NAME}.app"
fi
