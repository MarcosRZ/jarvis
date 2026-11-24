// Package news reads the headlines of the news service. It formats them for
// speech as "Source: title." and never stores them.
package news

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Item is one headline returned by the service.
type Item struct {
	Source    string    `json:"source"`
	Title     string    `json:"title"`
	Link      string    `json:"link"`
	Published time.Time `json:"published"`
}

type response struct {
	Items       []Item `json:"items"`
	RefreshedAt string `json:"refreshedAt"`
}

// Client asks the news service for one headline per feed. It is safe for
// concurrent use, and Update swaps the configuration without a restart.
type Client struct {
	mu sync.RWMutex
	// URL is the base URL, for example http://127.0.0.1:8091.
	URL     string
	Timeout time.Duration
	// PerFeed is how many headlines each feed contributes. Zero uses one.
	PerFeed int
}

// Update applies a new configuration. In-flight requests keep the one they
// started with.
func (c *Client) Update(url string, timeout time.Duration, perFeed int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.URL = strings.TrimSpace(url)
	if timeout > 0 {
		c.Timeout = timeout
	}
	if perFeed > 0 {
		c.PerFeed = perFeed
	}
}

func (c *Client) settings() (string, time.Duration, int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.URL, c.Timeout, c.PerFeed
}

// Headlines returns the headlines in feed order, formatted for speech.
func (c *Client) Headlines(ctx context.Context) ([]string, error) {
	serviceURL, timeout, perFeed := c.settings()
	if serviceURL == "" {
		return nil, errors.New("news service URL is empty")
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	endpoint := strings.TrimRight(serviceURL, "/") + "/news"
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("news URL: %w", err)
	}
	query := parsed.Query()
	if perFeed <= 0 {
		perFeed = 1
	}
	query.Set("per_feed", fmt.Sprint(perFeed))
	parsed.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("news request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("news service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("news service: HTTP %d", resp.StatusCode)
	}
	var body response
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("news service: %w", err)
	}
	headlines := make([]string, 0, len(body.Items))
	seen := make(map[string]bool, len(body.Items))
	for _, item := range body.Items {
		source := strings.TrimSpace(item.Source)
		title := strings.TrimSpace(item.Title)
		if source == "" || title == "" {
			continue
		}
		if !strings.HasSuffix(title, ".") && !strings.HasSuffix(title, "?") && !strings.HasSuffix(title, "!") {
			title += "."
		}
		line := source + ": " + title
		if seen[line] {
			continue
		}
		seen[line] = true
		headlines = append(headlines, line)
	}
	if len(headlines) == 0 {
		return nil, errors.New("news service has no headlines")
	}
	return headlines, nil
}
