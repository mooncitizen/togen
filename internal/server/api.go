package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mooncitizen/togen/internal/examples"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/simulate"
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

// Always version 2, whatever the file holds; the file itself is only
// rewritten by the next PUT.
// The first-run screen names the directory a project would be created in.
func (s *Server) getWorkspace(w http.ResponseWriter, _ *http.Request) {
	dir := s.dir
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	writeJSON(w, http.StatusOK, map[string]string{"dir": dir, "name": filepath.Base(dir)})
}

func (s *Server) getLayout(w http.ResponseWriter, _ *http.Request) {
	layout, err := workspace.LoadLayout(s.dir)
	if err != nil {
		if !workspace.Exists(workspace.LayoutPath(s.dir)) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeRefusal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, layout)
}

func (s *Server) getViews(w http.ResponseWriter, _ *http.Request) {
	views, err := workspace.LoadViews(s.dir)
	if err != nil {
		writeRefusal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, views)
}

// A missing file is not a problem: it means every default (ADR 0007). Styles and
// usage keyed by node name are checked against the project when there is one to
// check against; without it the answer is the configuration alone.
func (s *Server) getConfig(w http.ResponseWriter, _ *http.Request) {
	config, note, err := workspace.LoadConfig(s.dir)
	if err != nil {
		writeRefusal(w, err)
		return
	}
	if project, errs, err := workspace.LoadProject(s.dir); err == nil && len(errs) == 0 {
		if errs := workspace.CheckConfig(config, project); len(errs) > 0 {
			writeErrors(w, http.StatusUnprocessableEntity, errs)
			return
		}
	}
	writeJSON(w, http.StatusOK, struct {
		workspace.Config
		Deprecated string `json:"deprecated,omitempty"`
	}{config, note})
}

// An icon in togen.yml is a path relative to the file, so it is served from the
// project root and nowhere else, and only in the two formats a browser draws.
func (s *Server) getIcon(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	path, ok := iconPath(s.dir, rel)
	if !ok {
		writeError(w, http.StatusBadRequest, "the icon must be an .svg or .png file under the project root")
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "no such icon: "+rel)
		return
	}
	w.Header().Set("Content-Type", iconTypes[strings.ToLower(filepath.Ext(path))])
	_, _ = w.Write(raw)
}

var iconTypes = map[string]string{".svg": "image/svg+xml", ".png": "image/png"}

func iconPath(root, rel string) (string, bool) {
	if rel == "" || filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return "", false
	}
	if _, ok := iconTypes[strings.ToLower(filepath.Ext(rel))]; !ok {
		return "", false
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	inside, err := filepath.Rel(root, path)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", false
	}
	return path, true
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
// A version 1 body is accepted and lands on disk as version 2.
func (s *Server) putLayout(w http.ResponseWriter, r *http.Request) {
	raw, ok := readBody(w, r)
	if !ok {
		return
	}
	layout, err := workspace.ParseLayout(raw)
	if err != nil {
		writeRefusal(w, err)
		return
	}
	s.saveJSON(w, workspace.LayoutPath(s.dir), layout)
}

// The node ids a view names can only be checked against a project that loads.
func (s *Server) putViews(w http.ResponseWriter, r *http.Request) {
	raw, ok := readBody(w, r)
	if !ok {
		return
	}
	views, err := workspace.ParseViews(raw)
	if err != nil {
		writeRefusal(w, err)
		return
	}
	project, errs, err := workspace.LoadProject(s.dir)
	if err != nil || len(errs) > 0 {
		writeErrors(w, http.StatusUnprocessableEntity, ir.Errors{{Message: "the views cannot be checked without a valid project"}})
		return
	}
	if errs := workspace.ValidateViews(views, project); len(errs) > 0 {
		writeErrors(w, http.StatusUnprocessableEntity, errs)
		return
	}
	s.saveJSON(w, workspace.ViewsPath(s.dir), views)
}

func (s *Server) getSimulation(w http.ResponseWriter, _ *http.Request) {
	sim, err := workspace.LoadSimulation(s.dir)
	if err != nil {
		writeRefusal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sim)
}

func (s *Server) putSimulation(w http.ResponseWriter, r *http.Request) {
	raw, ok := readBody(w, r)
	if !ok {
		return
	}
	sim, err := workspace.ParseSimulation(raw)
	if err != nil {
		writeRefusal(w, err)
		return
	}
	project, errs, err := workspace.LoadProject(s.dir)
	if err != nil || len(errs) > 0 {
		writeErrors(w, http.StatusUnprocessableEntity, ir.Errors{{Message: "the simulation cannot be checked without a valid project"}})
		return
	}
	if errs := simulate.Validate(sim, project); len(errs) > 0 {
		writeErrors(w, http.StatusUnprocessableEntity, errs)
		return
	}
	s.saveJSON(w, workspace.SimulationPath(s.dir), sim)
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
	written, _, err := workspace.Generate(s.dir, request.Target, "", false, false)
	if err != nil {
		var stranger *workspace.StrangerError
		if errors.As(err, &stranger) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeRefusal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"generated": written.Targets, "notGenerated": written.NotGenerated})
}

// The body is a sketch, name, provider, region and environment, or names a
// bundled example. Either way togen/ must not be there yet, and a togen.yml
// that is stays as it is.
func (s *Server) postInit(w http.ResponseWriter, r *http.Request) {
	raw, ok := readBody(w, r)
	if !ok {
		return
	}
	var request struct {
		Example string `json:"example"`
		workspace.Sketch
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		writeError(w, http.StatusBadRequest, "the request body must be a JSON object")
		return
	}
	if workspace.Exists(workspace.TogenDir(s.dir)) {
		writeError(w, http.StatusConflict, (&workspace.ExistsError{Path: "togen/", Dir: s.dir}).Error())
		return
	}
	var files map[string][]byte
	if request.Example != "" {
		files, ok = examples.Files(request.Example)
		if !ok {
			writeErrors(w, http.StatusUnprocessableEntity, ir.Errors{{Path: "example", Message: fmt.Sprintf("unknown example '%s'", request.Example)}})
			return
		}
	} else {
		var err error
		files, err = workspace.SketchFiles(s.dir, request.Sketch)
		if err != nil {
			writeRefusal(w, err)
			return
		}
	}
	written, err := workspace.CreateProject(s.dir, files)
	if err != nil {
		var exists *workspace.ExistsError
		if errors.As(err, &exists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.created(written, files)
	writeJSON(w, http.StatusCreated, map[string]any{"written": written})
}

func (s *Server) getExamples(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"examples": examples.List()})
}

func (s *Server) getCost(w http.ResponseWriter, _ *http.Request) {
	doc, _, err := workspace.Cost(s.dir)
	if err != nil {
		if !workspace.Exists(workspace.ProjectPath(s.dir)) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeRefusal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
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

func (s *Server) saveJSON(w http.ResponseWriter, path string, value any) {
	raw, err := workspace.Marshal(value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.saveFile(w, path, raw)
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
