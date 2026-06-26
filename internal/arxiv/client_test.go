package arxiv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:arxiv="http://arxiv.org/schemas/atom">
  <entry>
    <id>http://arxiv.org/abs/2301.00001v2</id>
    <title>A Great Paper</title>
    <summary>We do great things.</summary>
    <published>2023-01-02T10:00:00Z</published>
    <author><name>Ada Lovelace</name></author>
    <author><name>Grace Hopper</name></author>
    <arxiv:doi>10.1234/great</arxiv:doi>
    <arxiv:primary_category term="cs.CL"/>
    <link href="http://arxiv.org/abs/2301.00001v2" rel="alternate" type="text/html"/>
    <link title="pdf" href="http://arxiv.org/pdf/2301.00001v2" rel="related" type="application/pdf"/>
    <category term="cs.CL"/>
  </entry>
</feed>`

func TestFetchRecentParsesFeed(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(sampleFeed))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 0)
	entries, err := c.FetchRecent(context.Background(), "cs.CL", 25)
	if err != nil {
		t.Fatalf("FetchRecent error: %v", err)
	}
	if c.Name() != "arxiv" {
		t.Fatalf("Name() = %q, want arxiv", c.Name())
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.SourceID != "2301.00001" {
		t.Fatalf("SourceID = %q, want 2301.00001 (version stripped)", e.SourceID)
	}
	if e.Title != "A Great Paper" {
		t.Fatalf("Title = %q", e.Title)
	}
	if e.Abstract != "We do great things." {
		t.Fatalf("Abstract = %q", e.Abstract)
	}
	if e.DOI != "10.1234/great" {
		t.Fatalf("DOI = %q", e.DOI)
	}
	if e.PrimaryCategory != "cs.CL" {
		t.Fatalf("PrimaryCategory = %q", e.PrimaryCategory)
	}
	if len(e.Authors) != 2 || e.Authors[0] != "Ada Lovelace" {
		t.Fatalf("Authors = %#v", e.Authors)
	}
	if e.Published.Year() != 2023 {
		t.Fatalf("Published = %v", e.Published)
	}
	if e.PDFURL != "http://arxiv.org/pdf/2301.00001v2" {
		t.Fatalf("PDFURL = %q", e.PDFURL)
	}
	for _, want := range []string{"max_results=25", "search_query=cat%3Acs.CL", "sortBy=submittedDate", "sortOrder=descending"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
}

func TestNormalizeArxivID(t *testing.T) {
	cases := map[string]string{
		"http://arxiv.org/abs/2301.00001v2":       "2301.00001",
		"http://arxiv.org/abs/2301.00001":         "2301.00001",
		"http://arxiv.org/abs/cond-mat/0211034v1": "cond-mat/0211034",
	}
	for in, want := range cases {
		if got := normalizeArxivID(in); got != want {
			t.Fatalf("normalizeArxivID(%q) = %q, want %q", in, got, want)
		}
	}
}
