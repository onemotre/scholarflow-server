package jobs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"scholarflow_server/internal/papers"
	"scholarflow_server/internal/sources"
)

// PaperIngester is the subset of papers.Service the harvester needs.
type PaperIngester interface {
	ExistsBySourceID(ctx context.Context, sourceID string) (bool, error)
	IngestPDF(ctx context.Context, info papers.SourceInfo, body io.Reader, size int64, contentType string) (papers.UploadResult, error)
}

// PDFFetcher downloads a PDF by URL into memory.
type PDFFetcher interface {
	Fetch(ctx context.Context, url string) ([]byte, error)
}

// HarvestPipeline pulls recent entries from each source/category, dedups, and
// ingests new papers into the standard processing pipeline. It is best-effort:
// per-category and per-entry failures are logged and skipped.
type HarvestPipeline struct {
	sources    []sources.Source
	categories []string
	maxResults int
	ingester   PaperIngester
	fetcher    PDFFetcher
}

func NewHarvestPipeline(srcs []sources.Source, categories []string, maxResults int, ingester PaperIngester, fetcher PDFFetcher) *HarvestPipeline {
	return &HarvestPipeline{
		sources:    srcs,
		categories: categories,
		maxResults: maxResults,
		ingester:   ingester,
		fetcher:    fetcher,
	}
}

func (h *HarvestPipeline) Harvest(ctx context.Context) error {
	ingested := 0
	for _, src := range h.sources {
		for _, cat := range h.categories {
			entries, err := src.FetchRecent(ctx, cat, h.maxResults)
			if err != nil {
				log.Printf("harvest: source=%s category=%s fetch failed: %v", src.Name(), cat, err)
				continue
			}
			for _, e := range entries {
				if err := h.ingestEntry(ctx, src, e); err != nil {
					log.Printf("harvest: source=%s id=%s skipped: %v", src.Name(), e.SourceID, err)
					continue
				}
				ingested++
			}
		}
	}
	log.Printf("harvest: completed, %d new paper(s) ingested", ingested)
	return nil
}

func (h *HarvestPipeline) ingestEntry(ctx context.Context, src sources.Source, e sources.Entry) error {
	if e.SourceID == "" || e.PDFURL == "" {
		return fmt.Errorf("entry missing source id or pdf url")
	}
	exists, err := h.ingester.ExistsBySourceID(ctx, e.SourceID)
	if err != nil {
		return fmt.Errorf("dedup check: %w", err)
	}
	if exists {
		return nil
	}
	data, err := h.fetcher.Fetch(ctx, e.PDFURL)
	if err != nil {
		return fmt.Errorf("download pdf: %w", err)
	}
	info := papers.SourceInfo{
		SourceType: src.Name(),
		SourceID:   e.SourceID,
		Filename:   filenameForEntry(e),
		Title:      e.Title,
		Abstract:   e.Abstract,
		DOI:        e.DOI,
		Year:       int32(e.Published.Year()),
	}
	if _, err := h.ingester.IngestPDF(ctx, info, bytes.NewReader(data), int64(len(data)), "application/pdf"); err != nil {
		return fmt.Errorf("ingest: %w", err)
	}
	return nil
}

func filenameForEntry(e sources.Entry) string {
	// arxiv ids may contain a slash (old style, e.g. cond-mat/0211034); flatten it.
	safe := make([]rune, 0, len(e.SourceID))
	for _, r := range e.SourceID {
		if r == '/' {
			r = '_'
		}
		safe = append(safe, r)
	}
	return string(safe) + ".pdf"
}

type httpPDFFetcher struct {
	httpClient *http.Client
	userAgent  string
}

func NewHTTPPDFFetcher(timeout time.Duration) *httpPDFFetcher {
	return &httpPDFFetcher{
		httpClient: &http.Client{Timeout: timeout},
		userAgent:  "scholarflow/0.1 (+https://github.com/)",
	}
}

func (f *httpPDFFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.userAgent)
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pdf download status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
