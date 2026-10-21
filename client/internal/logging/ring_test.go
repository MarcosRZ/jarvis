package logging

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func texts(lines []Line) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Text
	}
	return out
}

func TestRingKeepsTheLastLines(t *testing.T) {
	r := NewRing(3)
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(r, "line %d\n", i)
	}
	lines, last := r.After(0, 0)
	if got := strings.Join(texts(lines), ","); got != "line 3,line 4,line 5" || last != 5 {
		t.Errorf("lines %q, last %d", got, last)
	}
	if lines[0].Seq != 3 {
		t.Errorf("first kept line has number %d, want 3", lines[0].Seq)
	}
	if lines, _ := r.After(4, 0); strings.Join(texts(lines), ",") != "line 5" {
		t.Errorf("after 4: %q", texts(lines))
	}
	if lines, _ := r.After(0, 2); strings.Join(texts(lines), ",") != "line 4,line 5" {
		t.Errorf("limit 2: %q", texts(lines))
	}
}

func TestRingJoinsPartialWrites(t *testing.T) {
	r := NewRing(10)
	io.WriteString(r, "a=1 ")
	io.WriteString(r, "b=2\nc=3\nd=")
	lines, _ := r.After(0, 0)
	if got := strings.Join(texts(lines), "|"); got != "a=1 b=2|c=3" {
		t.Errorf("lines %q, want only the complete ones", got)
	}
}

func TestRingConcurrentWrites(t *testing.T) {
	r := NewRing(1000)
	logger := New(r, 0)
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 50 {
				logger.Info("state", "to", "LISTENING")
			}
		})
	}
	wg.Wait()
	lines, last := r.After(0, 0)
	if len(lines) != 500 || last != 500 {
		t.Fatalf("%d lines, last %d, want 500", len(lines), last)
	}
	for _, l := range lines {
		if !strings.HasSuffix(l.Text, "msg=state to=LISTENING") {
			t.Fatalf("mixed line %q", l.Text)
		}
	}
}

func TestServe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	r := NewRing(100)
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(r, "level=INFO msg=heard text=\"¿Qué hora es? %d\"\n", i)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := Serve(ctx, addr, r); err != nil {
		t.Fatal(err)
	}
	if err := Serve(ctx, addr, r); err == nil {
		t.Error("a second Serve on the same port did not fail")
	}

	get := func(path string) (*http.Response, string) {
		t.Helper()
		resp, err := http.Get("http://" + addr + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	resp, body := get("/?after=2")
	if resp.StatusCode != 200 || resp.Header.Get("X-Log-Last") != "4" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
		t.Errorf("status %d, headers %v", resp.StatusCode, resp.Header)
	}
	if want := "level=INFO msg=heard text=\"¿Qué hora es? 3\"\nlevel=INFO msg=heard text=\"¿Qué hora es? 4\"\n"; body != want {
		t.Errorf("body %q, want %q", body, want)
	}
	if _, body := get("/?n=1"); strings.Count(body, "\n") != 1 || !strings.Contains(body, "4") {
		t.Errorf("n=1: %q", body)
	}
	if resp, _ := get("/?after=x"); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad after: status %d", resp.StatusCode)
	}
	if resp, _ := get("/other"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("other path: status %d", resp.StatusCode)
	}
	resp, err = http.Post("http://"+addr+"/", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST: status %d", resp.StatusCode)
	}
}
