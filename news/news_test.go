package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const rssTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Feed</title>%s</channel></rss>`

func rssItemXML(title, link, date string) string {
	return fmt.Sprintf(`<item><title>%s</title><link>%s</link><pubDate>%s</pubDate></item>`, title, link, date)
}

func TestParseFeedReadsRSSAndAtom(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	raw := fmt.Sprintf(rssTemplate,
		rssItemXML("Primera &amp; corta", "https://example.com/1", now.Format(time.RFC1123Z))+
			rssItemXML("Segunda", "https://example.com/2", now.Add(-time.Hour).Format(time.RFC1123Z)))
	items, err := parseFeed(strings.NewReader(raw), "Fuente")
	if err != nil {
		t.Fatalf("parseFeed: %v", err)
	}
	if len(items) != 2 || items[0].Title != "Primera & corta" || items[0].Source != "Fuente" {
		t.Fatalf("items %+v", items)
	}
	if !items[0].Published.Equal(now) {
		t.Errorf("published %v", items[0].Published)
	}

	atom := `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
 <title>Atom</title>
 <entry>
  <title>Una noticia</title>
  <link rel="alternate" href="https://example.com/a"/>
  <published>2026-11-24T10:00:00Z</published>
 </entry>
</feed>`
	items, err = parseFeed(strings.NewReader(atom), "Otra")
	if err != nil {
		t.Fatalf("atom: %v", err)
	}
	if len(items) != 1 || items[0].Link != "https://example.com/a" || items[0].Source != "Otra" {
		t.Fatalf("atom items %+v", items)
	}
	if _, err := parseFeed(strings.NewReader(`<html></html>`), "x"); err == nil {
		t.Error("an unknown document must fail")
	}
}

func feedServer(t *testing.T, bodies map[string]string, etag string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if etag != "" {
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", etag)
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, body)
	}))
}

func TestServiceKeepsOneHeadlinePerFeedInOrder(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	bodies := map[string]string{
		"/a": fmt.Sprintf(rssTemplate,
			rssItemXML("A vieja", "https://example.com/a1", now.Add(-2*time.Hour).Format(time.RFC1123Z))+
				rssItemXML("A nueva", "https://example.com/a2", now.Format(time.RFC1123Z))),
		"/b": fmt.Sprintf(rssTemplate,
			rssItemXML("B única", "https://example.com/b1", now.Format(time.RFC1123Z))),
	}
	server := feedServer(t, bodies, "")
	defer server.Close()
	service := &Service{
		logger:      testLogger(),
		client:      server.Client(),
		byFeed:      map[string][]Item{},
		errors:      map[string]string{},
		cached:      map[string]cachedFeed{},
		kick:        make(chan struct{}, 1),
		refreshedAt: time.Time{},
	}
	service.cfg = Config{
		Feeds: []Feed{
			{Name: "Alfa", URL: server.URL + "/a"},
			{Name: "Beta", URL: server.URL + "/b"},
		},
		RefreshSeconds: 300,
		Retention:      Retention{Items: 20, Days: 7},
	}
	service.order = []string{"Alfa", "Beta"}
	service.Refresh(context.Background())

	items, _ := service.Headlines(1)
	if len(items) != 2 || items[0].Title != "A nueva" || items[1].Title != "B única" {
		t.Fatalf("headlines %+v", items)
	}
	status := service.Status()
	if len(status.Feeds) != 2 || status.Feeds[0].Items != 2 || status.Feeds[0].Error != "" {
		t.Errorf("status %+v", status)
	}
}

func TestServiceMergesAndHonoursLatestOnly(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	bodies := map[string]string{
		"/a": fmt.Sprintf(rssTemplate, rssItemXML("A", "https://example.com/a", now.Format(time.RFC1123Z))),
	}
	server := feedServer(t, bodies, "")
	defer server.Close()
	service := &Service{
		logger: testLogger(),
		client: server.Client(),
		byFeed: map[string][]Item{},
		errors: map[string]string{},
		cached: map[string]cachedFeed{},
		kick:   make(chan struct{}, 1),
	}
	service.cfg = Config{
		Feeds:          []Feed{{Name: "Alfa", URL: server.URL + "/a"}},
		RefreshSeconds: 300,
		Retention:      Retention{Items: 20, Days: 7},
	}
	service.order = []string{"Alfa"}
	service.Refresh(context.Background())
	bodies["/a"] = fmt.Sprintf(rssTemplate, rssItemXML("A nueva", "https://example.com/a2", now.Add(time.Hour).Format(time.RFC1123Z)))
	service.Refresh(context.Background())
	if items := service.Items(10); len(items) != 2 {
		t.Fatalf("merge kept %d items, want 2", len(items))
	}

	service.mu.Lock()
	service.cfg.Retention.LatestOnly = true
	service.mu.Unlock()
	bodies["/a"] = fmt.Sprintf(rssTemplate, rssItemXML("Solo esta", "https://example.com/a3", now.Add(2*time.Hour).Format(time.RFC1123Z)))
	service.Refresh(context.Background())
	items := service.Items(10)
	if len(items) != 1 || items[0].Title != "Solo esta" {
		t.Fatalf("latest only kept %+v", items)
	}
}

func TestServiceKeepsItemsOnNotModified(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	bodies := map[string]string{
		"/a": fmt.Sprintf(rssTemplate, rssItemXML("A", "https://example.com/a", now.Format(time.RFC1123Z))),
	}
	server := feedServer(t, bodies, `"v1"`)
	defer server.Close()
	service := &Service{
		logger: testLogger(),
		client: server.Client(),
		byFeed: map[string][]Item{},
		errors: map[string]string{},
		cached: map[string]cachedFeed{},
		kick:   make(chan struct{}, 1),
	}
	service.cfg = Config{
		Feeds:          []Feed{{Name: "Alfa", URL: server.URL + "/a"}},
		RefreshSeconds: 300,
		Retention:      Retention{Items: 20, Days: 7},
	}
	service.order = []string{"Alfa"}
	service.Refresh(context.Background())
	service.Refresh(context.Background())
	if items := service.Items(10); len(items) != 1 {
		t.Fatalf("items after 304: %+v", items)
	}
}

func TestServicePersistsTheConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "news.json")
	service, err := NewService(ServiceOptions{ConfigFile: path})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if len(service.Config().Feeds) != len(defaultFeeds()) {
		t.Fatalf("default feeds %+v", service.Config().Feeds)
	}
	cfg := Config{
		Feeds:          []Feed{{Name: "Uno", URL: "https://example.com/feed"}},
		RefreshSeconds: 120,
		Retention:      Retention{Items: 5, Days: 0},
	}
	if _, err := service.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	reloaded, err := NewService(ServiceOptions{ConfigFile: path})
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := reloaded.Config()
	if len(got.Feeds) != 1 || got.Feeds[0].Name != "Uno" || got.RefreshSeconds != 120 || got.Retention.Items != 5 {
		t.Fatalf("reloaded %+v", got)
	}
	if _, err := service.SetConfig(Config{Feeds: []Feed{{Name: "x", URL: "ftp://example.com"}}}); err == nil {
		t.Error("a non-http URL must fail")
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
