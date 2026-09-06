package server

import (
	"crypto/sha256"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"

	"github.com/mooncitizen/togen/internal/workspace"
)

type Options struct {
	Dir string
	UI  fs.FS
}

type Server struct {
	dir     string
	ui      fs.FS
	files   http.Handler
	handler http.Handler
	watcher *fsnotify.Watcher
	hub     *hub
	done    chan struct{}
	closing sync.Once
	watched sync.WaitGroup

	mu     sync.Mutex
	hashes map[string][sha256.Size]byte
}

func New(opts Options) (*Server, error) {
	ui := opts.UI
	if ui == nil {
		ui = UI()
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	// togen.yml sits at the root, the project files a directory down. A directory
	// with no project yet has no togen/; the watcher picks it up when init creates it.
	dirs := []string{opts.Dir}
	if workspace.Exists(workspace.TogenDir(opts.Dir)) {
		dirs = append(dirs, workspace.TogenDir(opts.Dir))
	}
	for _, dir := range dirs {
		if err := watcher.Add(dir); err != nil {
			_ = watcher.Close()
			return nil, err
		}
	}

	s := &Server{
		dir:     opts.Dir,
		ui:      ui,
		files:   http.FileServerFS(ui),
		watcher: watcher,
		hub:     newHub(),
		done:    make(chan struct{}),
		hashes:  map[string][sha256.Size]byte{},
	}
	for _, p := range []string{
		workspace.ProjectPath(opts.Dir),
		workspace.LayoutPath(opts.Dir),
		workspace.ViewsPath(opts.Dir),
		workspace.ConfigPath(opts.Dir),
	} {
		if hash, ok := fileHash(p); ok {
			s.hashes[p] = hash
		}
	}
	s.handler = noStore(sameOrigin(s.routes()))
	s.watched.Add(1)
	go s.watch()
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) Close() error {
	var err error
	s.closing.Do(func() {
		close(s.done)
		err = s.watcher.Close()
		s.watched.Wait()
	})
	return err
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/project", s.getProject)
	mux.HandleFunc("PUT /api/project", s.putProject)
	mux.HandleFunc("POST /api/project/init", s.postInit)
	mux.HandleFunc("GET /api/examples", s.getExamples)
	mux.HandleFunc("GET /api/layout", s.getLayout)
	mux.HandleFunc("PUT /api/layout", s.putLayout)
	mux.HandleFunc("GET /api/views", s.getViews)
	mux.HandleFunc("PUT /api/views", s.putViews)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("GET /api/icon", s.getIcon)
	mux.HandleFunc("POST /api/generate", s.postGenerate)
	mux.HandleFunc("GET /api/cost", s.getCost)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("/api/", notFound)
	mux.HandleFunc("/", s.serveUI)
	return mux
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// Anything the embedded app does not have is a route the client-side router owns,
// so the app itself is the answer.
func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" {
		if _, err := fs.Stat(s.ui, name); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
	}
	s.files.ServeHTTP(w, r)
}

func (s *Server) created(written []string, files map[string][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range written {
		s.hashes[filepath.Join(s.dir, filepath.FromSlash(name))] = sha256.Sum256(files[name])
	}
}

func (s *Server) save(path string, raw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hashes[path] = sha256.Sum256(raw)
	return workspace.WriteRaw(path, raw)
}

// A write of ours comes back from the watcher a moment later; that is not news.
func (s *Server) changed(path string) bool {
	hash, ok := fileHash(path)
	if !ok {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hashes[path] == hash {
		return false
	}
	s.hashes[path] = hash
	return true
}

func fileHash(path string) ([sha256.Size]byte, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return [sha256.Size]byte{}, false
	}
	return sha256.Sum256(raw), true
}
