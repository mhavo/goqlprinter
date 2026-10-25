//go:build gui

package cmd

import (
	"context"
	"fmt"
	iofs "io/fs"
	"net/http"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Start the desktop GUI",
	RunE:  runGUI,
}

func init() {
	rootCmd.AddCommand(guiCmd)
	defaultCommand = "gui"
}

// streamWriter adds http.Flusher and http.CloseNotifier to the Wails response
// writer, which has neither. Gin's c.Stream (used by the SSE endpoint) panics
// without them. Wails writes straight through to the webview on Linux and
// macOS, so Flush is a no-op; on Windows the response is buffered until the
// handler returns, so /api/events is refused there and the frontend falls
// back to polling (useSSE pollFallback).
type streamWriter struct {
	http.ResponseWriter
	done <-chan struct{}
}

func (w streamWriter) Flush() {}

func (w streamWriter) CloseNotify() <-chan bool {
	ch := make(chan bool, 1)
	go func() {
		<-w.done
		ch <- true
	}()
	return ch
}

func runGUI(cmd *cobra.Command, args []string) error {
	// No network listener in GUI mode, so bearer token auth is not applied.
	_, r, err := setupApp(false)
	if err != nil {
		return err
	}

	distFS, err := iofs.Sub(EmbeddedFiles, "frontend/dist")
	if err != nil {
		return fmt.Errorf("failed to create sub filesystem for dist: %w", err)
	}

	// Wails serves GET requests from Assets first and falls back to Handler
	// for everything else, so /api/* and the SPA fallback reach Gin.
	return wails.Run(&options.App{
		Title:     "Brother QL Label Printer",
		Width:     1200,
		Height:    560,
		MinWidth:  900,
		MinHeight: 360,
		AssetServer: &assetserver.Options{
			Assets: distFS,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				// Windows buffers the stream forever: refuse it so the frontend
				// switches to polling instead of leaving a hung handler behind.
				if runtime.GOOS == "windows" && req.URL.Path == "/api/events" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				// Wails requests carry context.Background(), so derive one that
				// ends with the handler to stop the CloseNotify goroutine.
				ctx, cancel := context.WithCancel(req.Context())
				defer cancel()
				r.ServeHTTP(streamWriter{w, ctx.Done()}, req.WithContext(ctx))
			}),
		},
	})
}
