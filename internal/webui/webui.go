package webui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
)

const DefaultAddr = "127.0.0.1:0"

//go:embed static
var staticFiles embed.FS

func Serve(ctx context.Context, stdout io.Writer, result analyze.Result) error {
	listener, err := net.Listen("tcp", DefaultAddr)
	if err != nil {
		return fmt.Errorf("listen web ui on %s: %w", DefaultAddr, err)
	}
	defer listener.Close()

	handler, err := NewHandler(result)
	if err != nil {
		return err
	}

	server := &http.Server{Handler: handler}
	errc := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errc <- err
			return
		}
		errc <- nil
	}()

	if _, err := fmt.Fprintf(stdout, "TokDoctor UI: http://%s\n", listener.Addr().String()); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		if err := server.Shutdown(context.Background()); err != nil {
			return fmt.Errorf("shutdown web ui: %w", err)
		}
		return ctx.Err()
	case err := <-errc:
		if err != nil {
			return fmt.Errorf("serve web ui: %w", err)
		}
		return nil
	}
}

func NewHandler(result analyze.Result) (http.Handler, error) {
	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, fmt.Errorf("load embedded web ui: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/result", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			http.Error(w, "encode result", http.StatusInternalServerError)
		}
	})
	mux.Handle("/", http.FileServer(http.FS(static)))

	return mux, nil
}
