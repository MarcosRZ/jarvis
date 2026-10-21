package logging

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Serve shows the lines of ring as plain text on addr until ctx ends. It
// listens before it returns, so a busy port is an error of Serve.
//
//	GET /              all kept lines
//	GET /?n=100        the last 100 lines
//	GET /?after=1234   the lines after line 1234
//
// The header X-Log-Last gives the number of the last line, for the next "after".
func Serve(ctx context.Context, addr string, ring *Ring) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("log web page: %w", err)
	}
	srv := &http.Server{Handler: handler(ring), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	go srv.Serve(ln)
	return nil
}

func handler(ring *Ring) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "read only", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		after, err1 := uintParam(r, "after")
		n, err2 := uintParam(r, "n")
		if err := errors.Join(err1, err2); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		lines, last := ring.After(after, int(n))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Log-Last", strconv.FormatUint(last, 10))
		for _, l := range lines {
			fmt.Fprintln(w, l.Text)
		}
	})
}

func uintParam(r *http.Request, name string) (uint64, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: use a whole number, 0 or greater", name, v)
	}
	return n, nil
}
