#!/usr/bin/env bash
# Wrap a macOS GUI binary into a minimal .app bundle.
# Usage: scripts/macos-app.sh <binary> <output-dir> [version]
set -euo pipefail
bin="$1"; out="$2"; version="${3:-dev}"
app="$out/Label Printer.app/Contents"
mkdir -p "$app/MacOS"
cp "$bin" "$app/MacOS/goqlprinter-gui"
cat > "$app/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Label Printer</string>
  <key>CFBundleIdentifier</key><string>com.goqlprinter.gui</string>
  <key>CFBundleExecutable</key><string>goqlprinter-gui</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>${version#v}</string>
  <key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST
echo "Built $out/Label Printer.app"
