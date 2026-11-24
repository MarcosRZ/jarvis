package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	GUID    string `xml:"guid"`
	PubDate string `xml:"pubDate"`
	Date    string `xml:"date"`
}

type rssDocument struct {
	XMLName xml.Name
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
	Items []rssItem `xml:"item"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	Links     []atomLink `xml:"link"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
}

type atomDocument struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

var tagPattern = regexp.MustCompile(`<[^>]*>`)

// parseFeed reads an RSS 2.0, RSS 1.0 or Atom feed and returns its items,
// newest first. source is the configured feed name.
func parseFeed(reader io.Reader, source string) ([]Item, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxBodyBytes {
		return nil, fmt.Errorf("feed is larger than %d bytes", maxBodyBytes)
	}
	var root struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	items := make([]Item, 0, 32)
	switch root.XMLName.Local {
	case "rss":
		var document rssDocument
		if err := xml.Unmarshal(raw, &document); err != nil {
			return nil, fmt.Errorf("parse RSS: %w", err)
		}
		items = append(items, rssItems(document, source)...)
	case "RDF":
		var document rssDocument
		if err := xml.Unmarshal(raw, &document); err != nil {
			return nil, fmt.Errorf("parse RSS 1.0: %w", err)
		}
		items = append(items, rssItems(document, source)...)
	case "feed":
		var document atomDocument
		if err := xml.Unmarshal(raw, &document); err != nil {
			return nil, fmt.Errorf("parse Atom: %w", err)
		}
		for _, entry := range document.Entries {
			items = append(items, atomItem(entry, source))
		}
	default:
		return nil, fmt.Errorf("unknown feed format %q", root.XMLName.Local)
	}
	kept := items[:0]
	for _, item := range items {
		if item.Title != "" {
			kept = append(kept, item)
		}
	}
	sortItems(kept)
	return kept, nil
}

func rssItems(document rssDocument, source string) []Item {
	all := append(append([]rssItem(nil), document.Channel.Items...), document.Items...)
	items := make([]Item, 0, len(all))
	for _, entry := range all {
		item := Item{
			Source:    source,
			Title:     cleanTitle(entry.Title),
			Link:      cleanLink(entry.Link),
			Published: parseDate(entry.PubDate),
		}
		if item.Link == "" && strings.HasPrefix(entry.GUID, "http") {
			item.Link = cleanLink(entry.GUID)
		}
		if item.Published.IsZero() {
			item.Published = parseDate(entry.Date)
		}
		items = append(items, item)
	}
	return items
}

func atomItem(entry atomEntry, source string) Item {
	link := ""
	for _, candidate := range entry.Links {
		if candidate.Rel == "" || candidate.Rel == "alternate" {
			link = candidate.Href
			break
		}
	}
	if link == "" && len(entry.Links) > 0 {
		link = entry.Links[0].Href
	}
	published := parseDate(entry.Published)
	if published.IsZero() {
		published = parseDate(entry.Updated)
	}
	return Item{
		Source:    source,
		Title:     cleanTitle(entry.Title),
		Link:      cleanLink(link),
		Published: published,
	}
}

func cleanTitle(title string) string {
	title = strings.TrimSpace(title)
	title = tagPattern.ReplaceAllString(title, "")
	title = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(title)
	return strings.Join(strings.Fields(title), " ")
}

func cleanLink(link string) string {
	return strings.TrimSpace(link)
}

var dateLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	time.RFC1123Z,
	time.RFC1123,
	time.RFC822Z,
	time.RFC822,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"2 Jan 2006 15:04:05 -0700",
	"2006-01-02T15:04:05Z0700",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

func parseDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range dateLayouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
