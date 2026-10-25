package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"time"

	"goqlprinter/internal/services"

	"github.com/gin-gonic/gin"
)

// SSEHub manages Server-Sent Events connections and broadcasts printer state changes.
type SSEHub struct {
	mu       sync.Mutex
	clients  map[chan string]struct{}
	printers *services.PrinterService
	lastJSON string // cached JSON of last known printer list
}

// NewSSEHub creates and starts an SSE hub that monitors printer changes.
func NewSSEHub(ps *services.PrinterService) *SSEHub {
	hub := &SSEHub{
		clients:  make(map[chan string]struct{}),
		printers: ps,
	}
	go hub.pollLoop()
	return hub
}

// pollLoop periodically checks for printer list changes and broadcasts to clients.
func (h *SSEHub) pollLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Initial snapshot
	h.refreshAndBroadcast()

	for range ticker.C {
		h.refreshAndBroadcast()
	}
}

func (h *SSEHub) refreshAndBroadcast() {
	printers, err := h.printers.FindPrinters()
	if err != nil {
		slog.Debug("SSE: printer discovery error", "error", err)
		return
	}

	// Keep default printer URI in sync (Linux /dev/usb/lp* changes on reconnect)
	h.printers.RefreshDefaultPrinter(printers)

	data, err := json.Marshal(gin.H{"printers": printers})
	if err != nil {
		return
	}
	jsonStr := string(data)

	h.mu.Lock()
	changed := jsonStr != h.lastJSON
	h.lastJSON = jsonStr
	h.mu.Unlock()

	if changed {
		h.broadcast("printers", jsonStr)
	}
}

func (h *SSEHub) broadcast(event, data string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	msg := formatSSE(event, data)
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
			// Client too slow, skip
		}
	}
}

func (h *SSEHub) addClient() chan string {
	ch := make(chan string, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	// Send current state immediately
	if h.lastJSON != "" {
		ch <- formatSSE("printers", h.lastJSON)
	}
	h.mu.Unlock()
	slog.Info("SSE client connected", "total", len(h.clients))
	return ch
}

func (h *SSEHub) removeClient(ch chan string) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
	slog.Info("SSE client disconnected", "total", len(h.clients))
}

func formatSSE(event, data string) string {
	return "event: " + event + "\ndata: " + data + "\n\n"
}

// HandleSSE is the Gin handler for the SSE endpoint.
func (h *SSEHub) HandleSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // disable nginx buffering

	ch := h.addClient()
	defer h.removeClient(ch)

	// Send headers right away so the client's onopen fires even when the
	// printer list never changes (addClient queues the current snapshot).
	_, _ = c.Writer.WriteString(": connected\n\n")
	c.Writer.Flush()

	// Periodic keepalive: a write to a closed connection fails and ends the
	// stream, also where the transport never cancels the request context.
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-ch:
			if !ok {
				return false
			}
			_, err := w.Write([]byte(msg))
			return err == nil
		case <-keepalive.C:
			_, err := w.Write([]byte(": ping\n\n"))
			return err == nil
		case <-c.Request.Context().Done():
			return false
		}
	})
}

// ForceRefresh triggers an immediate broadcast to all connected SSE clients.
// Call this after print operations or other actions that might change printer state.
func (h *SSEHub) ForceRefresh() {
	go h.refreshAndBroadcast()
}
