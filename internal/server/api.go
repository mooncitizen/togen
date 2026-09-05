package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/workspace"
)

const maxBody = 8 << 20

// A page on any origin can point a browser at localhost and fire a simple
// request; PUT and POST under /api/ change things, so they need proof the
// call came from the studio's own page. Origin and Sec-Fetch-Site are both
// set by the browser and cannot be forged by the page itself. A request
// with neither header (curl, the tests, an editor's HTTP client) passes.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method == http.MethodPut || r.Method == http.MethodPost) && strings.HasPrefix(r.URL.Path, "/api/") {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					writeError(w, http.StatusForbidden, "cross-origin request refused")
					return
				}
			}
			if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
				writeError(w, http.StatusForbidden, "cross-origin request refused")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) getProject(w http.ResponseWriter, _ *http.Request) {
	s.sendFile(w, workspace.ProjectPath(s.dir))
}

func (s *Server) getLayout(w http.ResponseWriter, _ *http.Request) {
	s.sendFile(w, workspace.LayoutPath(s.dir))
}

func (s *Server) getConfig(w http.ResponseWriter, _ *http.Request) {
	config, err := workspace.LoadConfig(s.dir)
	if err != nil {
		writeError(w, missingOrBroken(workspace.ConfigPath(s.dir)), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (s *Server) putProject(w http.ResponseWriter, r *http.Request) {
	raw, ok := readBody(w, r)
	if !ok {
		return
	}
	_, errs, err := workspace.ValidateRaw(raw)
	switch {
	case err != nil:
		writeRefusal(w, err)
		return
	case len(errs) > 0:
		writeErrors(w, http.StatusUnprocessableEntity, errs)
		return
	}
	s.saveFile(w, workspace.ProjectPath(s.dir), raw)
}

// The canvas owns the layout, so it is checked for shape and nothing more.
func (s *Server) putLayout(w http.ResponseWriter, r *http.Request) {
	raw, ok := readBody(w, r)
	if !ok {
		return
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		writeError(w, http.StatusBadRequest, "the layout must be a JSON object")
		return
	}
	if version, ok := doc["version"].(float64); !ok || int(version) != 1 {
		writeErrors(w, http.StatusUnprocessableEntity, ir.Errors{{Path: "version", Message: "the layout must have version 1"}})
		return
	}
	s.saveFile(w, workspace.LayoutPath(s.dir), raw)
}

func (s *Server) postGenerate(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read the request body")
		return
	}
	var request struct {
		Target string `json:"target"`
	}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			writeError(w, http.StatusBadRequest, "the request body is not valid JSON")
			return
		}
	}
	generated, err := workspace.Generate(s.dir, request.Target, "", false)
	if err != nil {
		var stranger *workspace.StrangerError
		if errors.As(err, &stranger) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeRefusal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"generated": generated})
}

func (s *Server) sendFile(w http.ResponseWriter, path string) {
	raw, err := workspace.ReadJSONFile(path, s.dir)
	if err != nil {
		writeError(w, missingOrBroken(path), err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

func (s *Server) saveFile(w http.ResponseWriter, path string, raw []byte) {
	if err := s.save(path, raw); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// A file present but not valid JSON means the caller cannot act on it either,
// but it is not a server fault: the request is answerable, the file is not.
func missingOrBroken(path string) int {
	if workspace.Exists(path) {
		return http.StatusUnprocessableEntity
	}
	return http.StatusNotFound
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read the request body")
		return nil, false
	}
	if !json.Valid(raw) {
		writeError(w, http.StatusBadRequest, "the request body is not valid JSON")
		return nil, false
	}
	return raw, true
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "no such endpoint: "+r.URL.Path)
}

// Errors the caller can do something about (a stale schema version, an unsupported
// target) travel in the same shape as validation errors.
func writeRefusal(w http.ResponseWriter, err error) {
	var errs ir.Errors
	var refusal *workspace.Error
	switch {
	case errors.As(err, &errs):
		writeErrors(w, http.StatusUnprocessableEntity, errs)
	case errors.As(err, &refusal):
		writeErrors(w, http.StatusUnprocessableEntity, ir.Errors{{Message: refusal.Message}})
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}

func writeErrors(w http.ResponseWriter, code int, errs ir.Errors) {
	writeJSON(w, code, map[string]any{"errors": errs})
}
