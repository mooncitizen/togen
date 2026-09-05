package server

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/mooncitizen/togen/internal/workspace"
)

const (
	debounce    = 100 * time.Millisecond
	pingEvery   = 30 * time.Second
	pingTimeout = 10 * time.Second
)

// Editors save through a rename, so the directory is watched rather than the files.
func (s *Server) watch() {
	defer s.watched.Done()
	pending := map[string]string{}
	var settled <-chan time.Time
	for {
		select {
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			path, name, ok := s.eventFor(event.Name)
			if !ok {
				continue
			}
			pending[path] = name
			settled = time.After(debounce)
		case _, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
		case <-settled:
			settled = nil
			for path, name := range pending {
				if s.changed(path) {
					s.hub.broadcast(name)
				}
			}
			clear(pending)
		}
	}
}

func (s *Server) eventFor(changed string) (path, name string, ok bool) {
	switch filepath.Base(changed) {
	case "project.json":
		return workspace.ProjectPath(s.dir), "project-changed", true
	case "layout.json":
		return workspace.LayoutPath(s.dir), "layout-changed", true
	case "views.json":
		return workspace.ViewsPath(s.dir), "views-changed", true
	case workspace.ConfigName:
		return workspace.ConfigPath(s.dir), "config-changed", true
	}
	return "", "", false
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	ctx := conn.CloseRead(r.Context())
	messages := s.hub.join()
	defer s.hub.leave(messages)
	ping := time.NewTicker(pingEvery)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			_ = conn.Close(websocket.StatusGoingAway, "togen studio is stopping")
			return
		case message := <-messages:
			if err := conn.Write(ctx, websocket.MessageText, message); err != nil {
				return
			}
		case <-ping.C:
			pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				// A half-open peer never answers; treat it the same as
				// the peer leaving rather than blocking the hub slot.
				return
			}
		}
	}
}

type hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newHub() *hub { return &hub{clients: map[chan []byte]struct{}{}} }

func (h *hub) join() chan []byte {
	ch := make(chan []byte, 8)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[ch] = struct{}{}
	return ch
}

func (h *hub) leave(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, ch)
}

// A client that has stopped reading loses events rather than holding up the watcher.
func (h *hub) broadcast(name string) {
	message, err := json.Marshal(struct {
		Event string `json:"event"`
	}{name})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- message:
		default:
		}
	}
}
