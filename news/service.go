package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultRefreshSeconds = 300
	minRefreshSeconds     = 60
	maxRefreshSeconds     = 86400
	maxItemsPerFeed       = 200
	maxDays               = 365
	maxFeeds              = 20
	fetchTimeout          = 15 * time.Second
	maxBodyBytes          = 2 << 20
)

// defaultFeeds are the Spanish sources read when no configuration is saved.
func defaultFeeds() []Feed {
	return []Feed{
		{Name: "elDiario.es", URL: "https://www.eldiario.es/rss/"},
		{Name: "infoLibre", URL: "https://www.infolibre.es/rss/"},
		{Name: "La Marea", URL: "https://www.lamarea.com/feed/"},
		{Name: "The Conversation ES", URL: "https://theconversation.com/es/articles.atom"},
		{Name: "Civio", URL: "https://civio.es/feed/"},
	}
}

// DefaultConfig is used when no configuration file exists.
func DefaultConfig() Config {
	return Config{
		Feeds:          defaultFeeds(),
		RefreshSeconds: defaultRefreshSeconds,
		Retention:      Retention{Items: 20, Days: 7},
	}
}

// Feed is one RSS or Atom source.
type Feed struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Retention bounds what the service keeps in memory. LatestOnly replaces the
// stored items with the ones of the most recent fetch.
type Retention struct {
	Items      int  `json:"items"`
	Days       int  `json:"days"`
	LatestOnly bool `json:"latestOnly"`
}

// Config is the persisted service configuration. Items are never persisted.
type Config struct {
	Feeds          []Feed    `json:"feeds"`
	RefreshSeconds int       `json:"refreshSeconds"`
	Retention      Retention `json:"retention"`
}

// Item is one headline. Source is the configured feed name, not the feed's
// own title, so the spoken "Fuente: titular." stays familiar.
type Item struct {
	Source    string    `json:"source"`
	Title     string    `json:"title"`
	Link      string    `json:"link"`
	Published time.Time `json:"published"`
}

type feedStatus struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Items int    `json:"items"`
	Error string `json:"error,omitempty"`
}

// Status is the dashboard view of the service.
type Status struct {
	RefreshedAt time.Time    `json:"refreshedAt"`
	Refreshing  bool         `json:"refreshing"`
	Feeds       []feedStatus `json:"feeds"`
	Config      Config       `json:"config"`
}

type cachedFeed struct {
	etag         string
	lastModified string
}

// Service holds the current configuration and the in-memory items.
type Service struct {
	logger     *slog.Logger
	configFile string
	client     *http.Client

	mu          sync.RWMutex
	cfg         Config
	byFeed      map[string][]Item
	order       []string
	refreshedAt time.Time
	errors      map[string]string
	cached      map[string]cachedFeed
	refreshing  bool
	kick        chan struct{}
}

// ServiceOptions configures NewService.
type ServiceOptions struct {
	ConfigFile string
	Logger     *slog.Logger
}

// NewService loads the saved configuration, or the defaults.
func NewService(options ServiceOptions) (*Service, error) {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	service := &Service{
		logger:     logger,
		configFile: options.ConfigFile,
		client:     &http.Client{Timeout: fetchTimeout},
		byFeed:     map[string][]Item{},
		errors:     map[string]string{},
		cached:     map[string]cachedFeed{},
		kick:       make(chan struct{}, 1),
	}
	cfg, err := loadConfig(options.ConfigFile)
	if err != nil {
		return nil, err
	}
	service.cfg = cfg
	for _, feed := range cfg.Feeds {
		service.order = append(service.order, feed.Name)
	}
	return service, nil
}

// Start refreshes once and then keeps the items current in the background.
func (s *Service) Start(ctx context.Context) error {
	go s.loop(ctx)
	return nil
}

func (s *Service) loop(ctx context.Context) {
	s.Refresh(ctx)
	for {
		wait := s.refreshInterval()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.kick:
			timer.Stop()
		case <-timer.C:
			s.Refresh(ctx)
		}
	}
}

func (s *Service) refreshInterval() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Duration(s.cfg.RefreshSeconds) * time.Second
}

// Config returns a copy of the current configuration.
func (s *Service) Config() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneConfig(s.cfg)
}

func cloneConfig(cfg Config) Config {
	out := cfg
	out.Feeds = append([]Feed(nil), cfg.Feeds...)
	return out
}

// Refresh fetches every feed and merges the results with the stored items.
func (s *Service) Refresh(ctx context.Context) {
	s.mu.Lock()
	if s.refreshing {
		s.mu.Unlock()
		return
	}
	s.refreshing = true
	cfg := cloneConfig(s.cfg)
	cached := make(map[string]cachedFeed, len(s.cached))
	for key, value := range s.cached {
		cached[key] = value
	}
	s.mu.Unlock()

	type result struct {
		feed  Feed
		items []Item
		etag  string
		last  string
		err   error
	}
	results := make(chan result, len(cfg.Feeds))
	for _, feed := range cfg.Feeds {
		go func(feed Feed) {
			items, validators, err := s.fetchFeed(ctx, feed, cached[feed.URL])
			results <- result{feed: feed, items: items, etag: validators.etag, last: validators.lastModified, err: err}
		}(feed)
	}
	fetched := make([]result, 0, len(cfg.Feeds))
	for range cfg.Feeds {
		fetched = append(fetched, <-results)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshing = false
	s.refreshedAt = time.Now()
	for _, res := range fetched {
		if res.err != nil {
			s.errors[res.feed.Name] = res.err.Error()
			s.logger.Warn("feed", "name", res.feed.Name, "error", res.err)
			continue
		}
		delete(s.errors, res.feed.Name)
		if res.etag != "" || res.last != "" {
			s.cached[res.feed.URL] = cachedFeed{etag: res.etag, lastModified: res.last}
		}
		if res.items == nil {
			// Not modified: keep the stored items.
			continue
		}
		if cfg.Retention.LatestOnly {
			s.byFeed[res.feed.Name] = trimFeed(res.items, cfg.Retention, s.refreshedAt)
			continue
		}
		s.byFeed[res.feed.Name] = trimFeed(mergeItems(s.byFeed[res.feed.Name], res.items), cfg.Retention, s.refreshedAt)
	}
	// Drop items of feeds that no longer exist.
	known := make(map[string]bool, len(cfg.Feeds))
	for _, feed := range cfg.Feeds {
		known[feed.Name] = true
	}
	for name := range s.byFeed {
		if !known[name] {
			delete(s.byFeed, name)
		}
	}
	s.logger.Info("news refreshed", "feeds", len(cfg.Feeds), "errors", len(s.errors))
}

type validators struct {
	etag         string
	lastModified string
}

func (s *Service) fetchFeed(ctx context.Context, feed Feed, cached cachedFeed) ([]Item, validators, error) {
	requestCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, feed.URL, nil)
	if err != nil {
		return nil, validators{}, err
	}
	request.Header.Set("User-Agent", "M4nhattanNews/1.0 (voice assistant headlines)")
	request.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.8")
	if cached.etag != "" {
		request.Header.Set("If-None-Match", cached.etag)
	}
	if cached.lastModified != "" {
		request.Header.Set("If-Modified-Since", cached.lastModified)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, validators{}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified {
		return nil, validators{etag: cached.etag, lastModified: cached.lastModified}, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, validators{}, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	items, err := parseFeed(response.Body, feed.Name)
	if err != nil {
		return nil, validators{}, err
	}
	return items, validators{
		etag:         response.Header.Get("ETag"),
		lastModified: response.Header.Get("Last-Modified"),
	}, nil
}

func mergeItems(old, fresh []Item) []Item {
	seen := make(map[string]bool, len(old)+len(fresh))
	merged := make([]Item, 0, len(old)+len(fresh))
	for _, item := range append(append([]Item(nil), fresh...), old...) {
		key := item.Link
		if key == "" {
			key = item.Source + "|" + item.Title
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, item)
	}
	sortItems(merged)
	return merged
}

func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].Published, items[j].Published
		if a.IsZero() != b.IsZero() {
			return b.IsZero()
		}
		if a.Equal(b) {
			return items[i].Title < items[j].Title
		}
		return a.After(b)
	})
}

func trimFeed(items []Item, retention Retention, now time.Time) []Item {
	kept := items[:0]
	for _, item := range items {
		if retention.Days > 0 && !item.Published.IsZero() && now.Sub(item.Published) > time.Duration(retention.Days)*24*time.Hour {
			continue
		}
		kept = append(kept, item)
	}
	if retention.Items > 0 && len(kept) > retention.Items {
		kept = kept[:retention.Items]
	}
	return kept
}

// Headlines returns the requested number of headlines per feed, in feed order.
func (s *Service) Headlines(perFeed int) ([]Item, time.Time) {
	if perFeed <= 0 {
		perFeed = 1
	}
	if perFeed > 10 {
		perFeed = 10
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Item, 0, len(s.order)*perFeed)
	for _, name := range s.order {
		items := s.byFeed[name]
		if len(items) > perFeed {
			items = items[:perFeed]
		}
		out = append(out, items...)
	}
	return out, s.refreshedAt
}

// Items returns every retained item, newest first.
func (s *Service) Items(limit int) []Item {
	s.mu.RLock()
	all := make([]Item, 0, len(s.byFeed)*4)
	for _, items := range s.byFeed {
		all = append(all, items...)
	}
	s.mu.RUnlock()
	sortItems(all)
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// Status returns the feed errors and the current configuration.
func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := Status{
		RefreshedAt: s.refreshedAt,
		Refreshing:  s.refreshing,
		Config:      cloneConfig(s.cfg),
	}
	for _, feed := range s.cfg.Feeds {
		status.Feeds = append(status.Feeds, feedStatus{
			Name:  feed.Name,
			URL:   feed.URL,
			Items: len(s.byFeed[feed.Name]),
			Error: s.errors[feed.Name],
		})
	}
	return status
}

// SetConfig validates and persists a new configuration, and refreshes now.
func (s *Service) SetConfig(cfg Config) (Config, error) {
	cfg, err := validateConfig(cfg)
	if err != nil {
		return Config{}, err
	}
	if err := saveConfig(s.configFile, cfg); err != nil {
		return Config{}, err
	}
	s.mu.Lock()
	s.cfg = cloneConfig(cfg)
	s.order = s.order[:0]
	for _, feed := range cfg.Feeds {
		s.order = append(s.order, feed.Name)
	}
	s.mu.Unlock()
	select {
	case s.kick <- struct{}{}:
	default:
	}
	return cloneConfig(cfg), nil
}

func validateConfig(cfg Config) (Config, error) {
	if len(cfg.Feeds) == 0 {
		return Config{}, errors.New("at least one feed is required")
	}
	if len(cfg.Feeds) > maxFeeds {
		return Config{}, fmt.Errorf("at most %d feeds are allowed", maxFeeds)
	}
	seen := make(map[string]bool, len(cfg.Feeds))
	feeds := make([]Feed, 0, len(cfg.Feeds))
	for _, feed := range cfg.Feeds {
		name := strings.TrimSpace(feed.Name)
		raw := strings.TrimSpace(feed.URL)
		if name == "" || raw == "" {
			return Config{}, errors.New("a feed needs a name and a URL")
		}
		if seen[name] {
			return Config{}, fmt.Errorf("duplicate feed name %q", name)
		}
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return Config{}, fmt.Errorf("feed %q: use an http or https URL", name)
		}
		seen[name] = true
		feeds = append(feeds, Feed{Name: name, URL: raw})
	}
	if cfg.RefreshSeconds == 0 {
		cfg.RefreshSeconds = defaultRefreshSeconds
	}
	if cfg.RefreshSeconds < minRefreshSeconds || cfg.RefreshSeconds > maxRefreshSeconds {
		return Config{}, fmt.Errorf("refreshSeconds must be between %d and %d", minRefreshSeconds, maxRefreshSeconds)
	}
	if cfg.Retention.Items < 0 || cfg.Retention.Items > maxItemsPerFeed {
		return Config{}, fmt.Errorf("retention.items must be between 0 and %d", maxItemsPerFeed)
	}
	if cfg.Retention.Days < 0 || cfg.Retention.Days > maxDays {
		return Config{}, fmt.Errorf("retention.days must be between 0 and %d", maxDays)
	}
	if cfg.Retention.Items == 0 && cfg.Retention.Days == 0 && !cfg.Retention.LatestOnly {
		cfg.Retention.Items = 20
		cfg.Retention.Days = 7
	}
	cfg.Feeds = feeds
	return cfg, nil
}

func loadConfig(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		return DefaultConfig(), nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	if len(cfg.Feeds) == 0 {
		return DefaultConfig(), nil
	}
	return validateConfig(cfg)
}

func saveConfig(path string, cfg Config) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	temporary := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(temporary, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func (s *Service) handleNews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	perFeed := 1
	if raw := r.URL.Query().Get("per_feed"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			perFeed = parsed
		}
	}
	items, refreshed := s.Headlines(perFeed)
	writeJSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"refreshedAt": refreshed,
	})
}

func (s *Service) handleItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.Items(limit)})
}

func (s *Service) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	writeJSON(w, http.StatusOK, s.Status())
}

func (s *Service) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.Config())
	case http.MethodPost, http.MethodPut:
		var cfg Config
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
		saved, err := s.SetConfig(cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.logger.Info("news configuration saved", "feeds", len(saved.Feeds), "refresh", saved.RefreshSeconds)
		writeJSON(w, http.StatusOK, saved)
	default:
		writeError(w, http.StatusMethodNotAllowed, "use GET or POST")
	}
}
