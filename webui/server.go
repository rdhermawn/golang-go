package webui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"golang-refine/internal/refine"
)

func NewHandler(hub *Hub) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/events/recent", func(w http.ResponseWriter, _ *http.Request) {
		payload, err := hub.RecentJSON()
		if err != nil {
			http.Error(w, "failed to encode events", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(payload)
	})
	mux.HandleFunc("/api/events/all", func(w http.ResponseWriter, _ *http.Request) {
		payload, err := hub.AllJSON()
		if err != nil {
			http.Error(w, "failed to encode events", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(payload)
	})
	mux.HandleFunc("/api/events/stream", hub.serveStream)
	mux.HandleFunc("/api/icons/", serveItemIcon)
	mux.Handle("/", serveFrontendAssets(frontendFS()))
	return mux
}

func Listen(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

func Start(ctx context.Context, listener net.Listener, hub *Hub) error {
	server := &http.Server{
		Handler:           NewHandler(hub),
		ReadHeaderTimeout: 5 * time.Second,
	}

	shutdownErrCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErrCh <- server.Shutdown(shutdownCtx)
	}()

	err := server.Serve(listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	shutdownErr := <-shutdownErrCh
	if shutdownErr != nil && !errors.Is(shutdownErr, context.Canceled) {
		return fmt.Errorf("web shutdown failed: %w", shutdownErr)
	}
	return nil
}

func serveItemIcon(w http.ResponseWriter, r *http.Request) {
	itemID := strings.TrimPrefix(r.URL.Path, "/api/icons/")
	itemID = strings.TrimSuffix(itemID, ".png")
	if itemID == "" || strings.Contains(itemID, "/") {
		http.NotFound(w, r)
		return
	}
	for _, ch := range itemID {
		if ch < '0' || ch > '9' {
			http.NotFound(w, r)
			return
		}
	}

	iconPath := refine.GetItemIconPath(itemID)
	if iconPath == "" {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, iconPath)
}

func (h *Hub) serveStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	stream, cancel := h.Subscribe()
	defer cancel()

	fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()

	keepAlive := time.NewTicker(20 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-stream:
			if !ok {
				return
			}
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: refine\n")
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		case <-keepAlive.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}
