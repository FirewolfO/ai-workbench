package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"ai-workbench/internal/model"

	xhtml "golang.org/x/net/html"
)

type feedClient struct{ http *http.Client }

type feedDocument struct {
	Channel struct {
		Items []feedItem `xml:"item"`
	} `xml:"channel"`
	Entries []feedItem `xml:"entry"`
}

type feedItem struct {
	Title       string     `xml:"title"`
	Links       []feedLink `xml:"link"`
	GUID        string     `xml:"guid"`
	Description string     `xml:"description"`
	Summary     string     `xml:"summary"`
	Content     string     `xml:"content"`
	PubDate     string     `xml:"pubDate"`
	Published   string     `xml:"published"`
	Updated     string     `xml:"updated"`
	Date        string     `xml:"date"`
	Creator     string     `xml:"creator"`
	Author      atomAuthor `xml:"author"`
}

type feedLink struct {
	Text string `xml:",chardata"`
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

func newFeedClient() *feedClient {
	return &feedClient{http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *feedClient) fetch(ctx context.Context, source Source) ([]model.NewsArticle, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "AI-Workbench/1.0 (+enterprise news reader)")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	decoder := xml.NewDecoder(io.LimitReader(response.Body, 8<<20))
	var document feedDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	items := document.Channel.Items
	if len(items) == 0 {
		items = document.Entries
	}
	now := time.Now().UTC()
	result := make([]model.NewsArticle, 0, len(items))
	for _, item := range items {
		link := ""
		for _, candidate := range item.Links {
			if candidate.Rel == "" || candidate.Rel == "alternate" {
				link = strings.TrimSpace(first(candidate.Href, candidate.Text))
				if link != "" {
					break
				}
			}
		}
		title := cleanText(item.Title, 500)
		if link == "" || title == "" {
			continue
		}
		published := parseFeedTime(first(item.PubDate, item.Published, item.Updated, item.Date))
		if published.IsZero() {
			published = now
		}
		summary := cleanText(first(item.Description, item.Summary, item.Content), 700)
		digest := sha256.Sum256([]byte(link))
		result = append(result, model.NewsArticle{
			ID: "news_" + hex.EncodeToString(digest[:12]), SourceCode: source.Code, SourceName: source.Name,
			ExternalID: strings.TrimSpace(item.GUID), Title: title, Summary: summary, URL: link,
			Author: cleanText(first(item.Creator, item.Author.Name), 200), PublishedAt: published.UTC(), FetchedAt: now,
		})
	}
	return result, nil
}

func cleanText(value string, maximum int) string {
	value = html.UnescapeString(value)
	document, err := xhtml.Parse(strings.NewReader(value))
	if err == nil {
		var fragments []string
		var visit func(*xhtml.Node)
		visit = func(node *xhtml.Node) {
			if node.Type == xhtml.TextNode {
				fragments = append(fragments, node.Data)
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				visit(child)
			}
		}
		visit(document)
		value = strings.Join(fragments, " ")
	}
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > maximum {
		value = string(runes[:maximum]) + "..."
	}
	return value
}

func parseFeedTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, "Mon, 2 Jan 2006 15:04:05 -0700", "2006-01-02 15:04:05 -0700"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func first(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
