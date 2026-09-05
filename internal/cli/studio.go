package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/mooncitizen/togen/internal/server"
	"github.com/mooncitizen/togen/internal/workspace"
)

func Studio(ctx context.Context, cwd string, port int, openBrowser bool, ready func(url string)) error {
	if !workspace.Exists(workspace.ProjectPath(cwd)) {
		return &workspace.Error{Message: "togen/project.json not found. Run 'togen init' first."}
	}
	studio, err := server.New(server.Options{Dir: cwd})
	if err != nil {
		return err
	}
	defer func() { _ = studio.Close() }()

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	url := "http://" + listener.Addr().String()

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	serving := &http.Server{Handler: studio.Handler(), ReadHeaderTimeout: 10 * time.Second}
	failed := make(chan error, 1)
	go func() { failed <- serving.Serve(listener) }()

	if ready != nil {
		ready(url)
	}
	if openBrowser {
		browse(url)
	}

	select {
	case err := <-failed:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	closing, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return serving.Shutdown(closing)
}

func browse(url string) {
	var opener string
	switch runtime.GOOS {
	case "darwin":
		opener = "open"
	case "linux":
		opener = "xdg-open"
	default:
		return
	}
	_ = exec.Command(opener, url).Start()
}
