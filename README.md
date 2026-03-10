# goqlprinter

Lightweight web UI and REST API for Brother QL label printers. Turn any QL printer into a network printer, automate label printing from scripts, or just print labels faster than P-touch Editor. Single binary, no dependencies.

![Brother QL series](https://img.shields.io/badge/Brother-QL_series-blue)
![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)
![License](https://img.shields.io/badge/License-MIT-green)

## Features

- Web UI with live preview — print from any device on the network
- CLI for scripting and automation
- REST API for integration with other tools
- Text labels with font/alignment control, multiline support
- QR code generation and printing
- SVG and PNG image printing
- Auto-discovers connected Brother QL printers
- 20+ label sizes (endless tape, die-cut, round)
- HTTPS + token authentication for secure remote access
- Single binary with embedded frontend — no external dependencies

![goqlprinter screenshot](docs/screenshot.png)

## Supported Printers

Brother QL-500, 550, 560, 570, 580N, 650TD, 700, 710W, 720NW, 800, 810W, 820NWB, 1050, 1060N, 1100, 1110NWB

## Requirements

- Go 1.24+
- Node.js 18+ (frontend build)
- [`just`](https://github.com/casey/just) (build tool)
- USB access permissions for direct printer communication

## Getting Started

```bash
# Install frontend dependencies
just install-frontend

# Run development server (Go backend + Vite dev server)
just dev
```

This starts two servers concurrently:
- **http://localhost:5173** — Vite frontend with hot reload (use this in dev)
- **http://localhost:8000** — Go backend API

Open http://localhost:5173 in your browser during development. In production, the binary serves everything from port 8000.

## Building

```bash
# Build for current platform
just build

# Cross-compile for all platforms
just build-all
```

| Target | Command |
|--------|---------|
| Linux | `just build-linux-native` |
| Linux (ARM64) | `just build-linux-arm-native` |
| Windows | `just build-windows-native` |
| macOS (amd64 + arm64) | `just build-darwin-native` |

The binary embeds the frontend — no separate web server needed.

The native builds use OS printer interfaces and work with standard printer drivers. Printer status reporting varies by platform:

| Platform | Printing | Status reporting |
|----------|----------|------------------|
| Linux | Full | Full (bidirectional `/dev/usb/lp*`) |
| macOS | Full | Basic (printer state via CUPS/IPP, no media info) |
| Windows | Full | Minimal (connection status only, no detailed errors) |

<details>
<summary>Advanced: USB (libusb) builds</summary>

USB builds communicate directly with the printer over USB, bypassing OS drivers. This gives full bidirectional status reporting on all platforms, but requires CGO, libusb-dev, and that no OS driver claims the device.

| Target | Command |
|--------|---------|
| Linux USB | `just build-linux-usb` |
| Windows USB | `just build-windows-usb` |

Windows USB builds require replacing the Brother driver with WinUSB using [Zadig](https://zadig.akeo.ie/).
</details>

## Configuration

Configuration is loaded in priority order (highest wins):

1. `LABELPRINTER_*` environment variables
2. `./config/config.json`
3. `~/.labelprinter/config.json`
4. `/etc/labelprinter/config.json`
5. Defaults

Example `config.json`:

```json
{
  "server": {
    "port": 8000,
    "host": "localhost",
    "tls": false,
    "cert_file": "./certs/dev.crt",
    "key_file": "./certs/dev.key",
    "token": ""
  },
  "app": {
    "backend": "auto",
    "default_printer": "QL-570",
    "font_dirs": ["./fonts", "~/.fonts"]
  }
}
```

## CLI Usage

```bash
# Start the web server
./goqlprinter serve

# List connected printers
./goqlprinter printers

# Print text directly
./goqlprinter print "Hello, World!" -l 62

# List available label sizes and fonts
./goqlprinter labels
./goqlprinter fonts
```

## USB Permissions (Linux)

Add a udev rule so the printer is accessible without root:

```
SUBSYSTEM=="usb", ATTR{idVendor}=="04f9", MODE="0666"
```

Save to `/etc/udev/rules.d/50-brother-ql.rules` and reload: `sudo udevadm control --reload-rules`

## macOS

The native build works if you have the Brother P-touch driver installed (available on the App Store). For driverless operation, use the **USB build** which communicates directly with the printer:

```bash
brew install libusb
just build-darwin-usb
sudo ./dist/darwin-usb-arm64/goqlprinter serve
```

Root access (`sudo`) is required because macOS restricts direct USB device access.

## Windows

Works with the standard Brother printer driver — no extra setup needed.

## API

The server exposes a REST API at port 8000 (default):

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/printers` | List connected printers |
| GET | `/api/label-sizes` | List supported label sizes |
| GET | `/api/fonts` | List available fonts |
| POST | `/api/print` | Print text label |
| POST | `/api/print_qr` | Print QR code |
| POST | `/api/print_svg` | Print SVG |
| POST | `/api/print_png` | Print PNG |
| POST | `/api/preview` | Get PNG preview |
| POST | `/api/status` | Query printer status |

Swagger UI available at `/swagger/index.html`.

## HTTPS & API Security

goqlprinter supports HTTPS and token-based API authentication, making it safe to expose on a network for automated printing from other systems — patient label printing from hospital information systems, asset tags from inventory databases, shipping labels from warehouse management, etc.

### Quick Start (Development)

A self-signed certificate is included in `certs/` for development. Enable HTTPS:

```json
{
  "server": {
    "tls": true,
    "cert_file": "./certs/dev.crt",
    "key_file": "./certs/dev.key",
    "token": "my-secret-token"
  }
}
```

```bash
# Print from a script
curl -k https://localhost:8000/api/print \
  -H "Authorization: Bearer my-secret-token" \
  -H "Content-Type: application/json" \
  -d '{"text": "Patient: John Doe\nDOB: 1990-01-15", "label_size": "62"}'
```

### Production Setup

For production, use a proper certificate from your CA or Let's Encrypt:

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8443,
    "tls": true,
    "cert_file": "/etc/ssl/certs/labelprinter.pem",
    "key_file": "/etc/ssl/private/labelprinter.key",
    "token": "your-secure-token-here"
  }
}
```

All settings can also be set via environment variables:

```bash
LABELPRINTER_SERVER_TLS=true
LABELPRINTER_SERVER_CERT_FILE=/path/to/cert.pem
LABELPRINTER_SERVER_KEY_FILE=/path/to/key.pem
LABELPRINTER_SERVER_TOKEN=your-secure-token-here
```

### Token Authentication

When `server.token` is set, all `/api/*` endpoints require a `Bearer` token in the `Authorization` header. The token is compared using constant-time comparison to prevent timing attacks. The token is never exposed via the `/api/config` endpoint.

Without a token, the API is open — suitable for local use or when behind a reverse proxy that handles authentication.

### Using with a Reverse Proxy

If you prefer to handle TLS termination externally (e.g., with Caddy or nginx), run goqlprinter in plain HTTP mode and let the proxy handle encryption:

```
# Caddyfile
labels.example.com {
    reverse_proxy localhost:8000
}
```

## Debug Mode

Set `"printer": "file"` in any print request to write the rasterized image to `debug_output/` instead of sending it to a printer.

## Acknowledgements

This project was inspired by and builds on ideas from:

- [brother_ql_web](https://github.com/pklaus/brother_ql_web) — Web interface for Brother QL printers (Python)
- [brother_ql](https://github.com/pklaus/brother_ql) — Brother QL printer protocol library (Python)

Proudly built with [Claude](https://claude.ai/).

## License

MIT — see [LICENSE](LICENSE)
