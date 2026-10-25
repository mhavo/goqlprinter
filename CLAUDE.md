# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go-based backend + embedded React frontend for Brother QL label printer web GUI. RESTful APIs for printer discovery, text/SVG/PNG/QR label printing, and configuration.

## Architecture

### Core Components
- **`main.go`**: Entry point, `//go:embed all:frontend/dist`, delegates to `cmd.Execute()`
- **`cmd/`**: Cobra CLI commands (serve, print, printers, labels, fonts, status)
  - `root.go`: Global state, persistent setup (logging, config, backend init)
  - `serve.go`: Gin web server, route setup, embedded frontend serving
  - `backend.go`: Backend provider selection (auto/usb/native)
  - `gui.go`: Desktop app (`gui` build tag), Wails v2 window serving the Gin router (no network port)
- **`brotherql/`**: Core printer protocol implementation
  - `brotherql.go`: Main printing logic with Brother QL protocol
  - `models.go`: Printer models, label sizes, configuration data
  - `raster.go`: Image processing and rasterization
  - `backend.go`: `Backend`, `BackendProvider`, `StatusProvider` interfaces
  - `native_*.go`: Platform-specific backends (Linux/macOS/Windows)
  - `usb_backend.go`: USB communication (gousb, CGO)
- **`api/`**: REST handlers with `Handlers` DI struct (`PrinterService`, `FontService`, `Config`)
  - `middleware.go`: Bearer token auth (constant-time comparison)
  - `sse.go`: Server-Sent Events hub for real-time printer status
- **`internal/services/`**: `PrinterService` (discovery), `FontService` (fonts), `PrinterLock` (mutex)
- **`internal/config/`**: Viper-based config (JSON files + env vars)
- **`frontend/`**: React 18 + TypeScript + Tailwind CSS (Vite)

## Development Commands

```bash
just serve          # Dev server on :8000
just dev            # Concurrent Go + Vite dev servers
just build          # Build for current platform
just build-frontend # Bundle React frontend
just build-all      # All platform targets
just gui            # Desktop app (Wails window, needs frontend build)
just build-gui      # Desktop app for current platform
go test ./...       # Run tests
go mod tidy         # Clean up dependencies
```

### Configuration
Config priority (low→high):
1. Defaults → 2. `/etc/labelprinter/config.json` → 3. `~/.labelprinter/config.json` → 4. `./config/config.json` → 5. `LABELPRINTER_*` env vars

### Security
- **HTTPS**: `server.tls=true` + `server.cert_file` + `server.key_file` (env: `LABELPRINTER_SERVER_TLS/CERT_FILE/KEY_FILE`)
- **Auth**: `server.token` sets Bearer token for all `/api/*` routes (env: `LABELPRINTER_SERVER_TOKEN`)

### Debug
- `printer: "file"` in API requests → saves PNG to `debug_output/`
- Verbose logging enabled by default

## API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/config` | Server configuration |
| GET | `/api/printers` | Discover printers |
| GET | `/api/fonts` | List fonts |
| GET | `/api/label-sizes` | All label formats |
| GET | `/api/label-sizes/:id` | Single label details |
| GET | `/api/events` | SSE printer status stream |
| POST | `/api/print` | Print text/SVG label |
| POST | `/api/print_svg` | Print SVG (needs rsvg-convert) |
| POST | `/api/print_png` | Print base64 PNG |
| POST | `/api/print_png_raw` | Print uploaded PNG |
| POST | `/api/print_qr` | Print QR code |
| POST | `/api/preview` | Return base64 PNG preview |
| POST | `/api/status` | Printer status |
| POST | `/api/test/*` | Debug: invalidate/init/feed |
| GET | `/swagger/*` | Swagger UI docs |

## Key Rules

**MUST:**
- Acquire `services.PrinterLock` before any printer I/O
- Flip images horizontally before rasterization
- Use build tags: `usb` for gousb, `!usb` for native
- Pad raster rows to full `RasterWidthBytes` (90 or 162)
- Use `Backend` interface, never concrete types in handlers
- Use build tag `gui` for Wails code

**MUST NOT:**
- Send concurrent commands to same printer
- Skip invalidation bytes before ESC @ reset
- Assume label height > 0 (0 = endless tape)
- Import `gousb` without `//go:build usb` tag
- Import Wails without `//go:build gui` tag
- Hardcode raster width (varies by model)

## Common Issues
- **USB permissions**: Ensure user has USB device access
- **rsvg-convert missing**: `apt install librsvg2-bin`
- **Fonts missing**: Verify font paths in configuration

## AI Context Docs

Compressed documentation for AI assistants in `docs/ai-*.md`:

| Doc | Content |
|-----|---------|
| `ai-system.md` | Architecture, components, patterns |
| `ai-dataflow.md` | Print pipeline, data structures |
| `ai-cli-reference.md` | API endpoints, build/CLI commands |
| `ai-coding-guide.md` | Protocol rules, MUST/MUST NOT |
| `ai-tech-stack.md` | Dependencies, hardware support |

### Load Full Context
```bash
claude --append-system-prompt "$(cat docs/ai-*.md)"
```
