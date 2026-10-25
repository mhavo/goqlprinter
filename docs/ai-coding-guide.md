# Coding Guide

## Critical Rules

**MUST:**
- Acquire `services.PrinterLock` before any printer I/O
- Flip images horizontally before rasterization (protocol requirement)
- Use build tags: `usb` for gousb, `!usb` for native
- Pad raster rows to full `RasterWidthBytes` (90 or 162)
- Handle `printer: "file"` for debug output to `debug_output/`
- Use `Backend` interface, never concrete types in handlers
- Use `crypto/subtle.ConstantTimeCompare` for token comparison
- Use build tag `gui` for Wails code

**MUST NOT:**
- Send concurrent commands to same printer (mutex required)
- Skip invalidation bytes before ESC @ reset
- Assume label height > 0 (0 = endless tape)
- Import `gousb` without `//go:build usb` tag
- Import Wails without `//go:build gui` tag
- Hardcode raster width (varies by model: 90 vs 162)
- Expose `server.token` in JSON responses

## Protocol Sequence

```
1. Send N null bytes (invalidate: 200-400)
2. ESC @ (initialize)
3. Drain stale responses
4. ESC i a 1 (switch mode, if supported)
5. ESC i z (media settings)
6. ESC i d (margins)
7. ESC i M/K (auto-cut, expanded mode)
8. Raster rows with 'g' prefix
9. 0x1A (print)
10. Read 32-byte status response
```

## Rasterization

- Threshold: pixel < 250 → black (1), >= 250 → white (0)
- PackBits compression for supported models
- 300 DPI resolution
