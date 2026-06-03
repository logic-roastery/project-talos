package handlers

import (
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/logic-roastery/project-talos/internal/runtime/docker"
	"github.com/logic-roastery/project-talos/internal/store"
)

type LogHandler struct {
	apps   store.AppStore
	docker *docker.Client
	logger *slog.Logger
}

func NewLogHandler(apps store.AppStore, docker *docker.Client, logger *slog.Logger) *LogHandler {
	return &LogHandler{
		apps:   apps,
		docker: docker,
		logger: logger,
	}
}

// StreamLogs opens an SSE stream that tails the active container's stdout/stderr.
func (h *LogHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	appID, err := parseID(r, "appID")
	if err != nil {
		http.Error(w, "invalid app id", http.StatusBadRequest)
		return
	}

	app, err := h.apps.GetApp(r.Context(), appID)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	containerName := app.LiveContainerName
	if containerName == "" {
		containerName = fmt.Sprintf("talos-%s", app.Name)
	}

	reader, err := h.docker.StreamLogs(r.Context(), containerName, "100")
	if err != nil {
		h.logger.Error("stream logs failed", "app", app.Name, "error", err)
		http.Error(w, "failed to open log stream", http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial connected event
	fmt.Fprintf(w, "event: status\ndata: connected\n\n")
	flusher.Flush()

	// Demux and stream
	hdr := make([]byte, 8)
	for {
		// Read the 8-byte multiplexed header
		if _, err := io.ReadFull(reader, hdr); err != nil {
			if err != io.EOF {
				h.logger.Debug("log stream ended", "app", app.Name, "error", err)
			}
			break
		}

		// byte 0: stream type (1=stdout, 2=stderr)
		// bytes 4-7: uint32 big-endian payload size
		frameSize := binary.BigEndian.Uint32(hdr[4:8])
		if frameSize == 0 {
			continue
		}

		payload := make([]byte, frameSize)
		if _, err := io.ReadFull(reader, payload); err != nil {
			h.logger.Debug("log stream read payload failed", "app", app.Name, "error", err)
			break
		}

		streamType := "stdout"
		if hdr[0] == 2 {
			streamType = "stderr"
		}

		// Split by newlines and send each line as an SSE event
		lines := strings.Split(strings.TrimRight(string(payload), "\n"), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", streamType, line)
		}
		flusher.Flush()
	}

	// Stream ended (container stopped or client disconnected)
	fmt.Fprintf(w, "event: status\ndata: stopped\n\n")
	flusher.Flush()
}
