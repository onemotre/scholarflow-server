package arxiv

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"scholarflow_server/internal/sources"
)

const defaultUserAgent = "scholarflow/0.1 (+https://github.com/; mailto:noreply@example.com)"

// Client queries the arXiv Query API (Atom feed) and implements sources.Source.
type Client struct {
	baseURL    string
	httpClient *http.Client
	userAgent  string
	delay      time.Duration
}

// NewClient builds a client. baseURL is the query endpoint
// (e.g. http://export.arxiv.org/api/query); delay is the politeness sleep
// applied before each request (arXiv asks for ~3s between requests).
func NewClient(baseURL string, delay time.Duration) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		userAgent:  defaultUserAgent,
		delay:      delay,
	}
}

func (c *Client) Name() string { return "arxiv" }

func (c *Client) FetchRecent(ctx context.Context, category string, maxResults int) ([]sources.Entry, error) {
	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	q := url.Values{}
	q.Set("search_query", "cat:"+category)
	q.Set("sortBy", "submittedDate")
	q.Set("sortOrder", "descending")
	q.Set("start", "0")
	q.Set("max_results", fmt.Sprintf("%d", maxResults))

	reqURL := c.baseURL + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build arxiv request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("arxiv request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("arxiv status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read arxiv body: %w", err)
	}
	return parseFeed(body)
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID        string `xml:"id"`
	Title     string `xml:"title"`
	Summary   string `xml:"summary"`
	Published string `xml:"published"`
	Authors   []struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Links []struct {
		Href  string `xml:"href,attr"`
		Type  string `xml:"type,attr"`
		Title string `xml:"title,attr"`
		Rel   string `xml:"rel,attr"`
	} `xml:"link"`
	DOI             string `xml:"http://arxiv.org/schemas/atom doi"`
	PrimaryCategory struct {
		Term string `xml:"term,attr"`
	} `xml:"http://arxiv.org/schemas/atom primary_category"`
}

func parseFeed(body []byte) ([]sources.Entry, error) {
	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse arxiv feed: %w", err)
	}
	entries := make([]sources.Entry, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		published, _ := time.Parse(time.RFC3339, strings.TrimSpace(e.Published))
		authors := make([]string, 0, len(e.Authors))
		for _, a := range e.Authors {
			authors = append(authors, strings.TrimSpace(a.Name))
		}
		entries = append(entries, sources.Entry{
			SourceID:        normalizeArxivID(e.ID),
			Title:           cleanText(e.Title),
			Abstract:        cleanText(e.Summary),
			DOI:             strings.TrimSpace(e.DOI),
			PrimaryCategory: e.PrimaryCategory.Term,
			Authors:         authors,
			Published:       published,
			PDFURL:          pdfURL(e),
		})
	}
	return entries, nil
}

func pdfURL(e atomEntry) string {
	for _, l := range e.Links {
		if l.Title == "pdf" || l.Type == "application/pdf" {
			return l.Href
		}
	}
	return ""
}

var versionSuffix = regexp.MustCompile(`v\d+$`)

// normalizeArxivID extracts the bare arxiv id from an abs URL and strips the
// version suffix so dedup is stable across paper revisions.
func normalizeArxivID(idURL string) string {
	id := idURL
	if i := strings.Index(id, "/abs/"); i >= 0 {
		id = id[i+len("/abs/"):]
	}
	return versionSuffix.ReplaceAllString(id, "")
}

func cleanText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
