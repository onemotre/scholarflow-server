package jobs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
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
	sources       []sources.Source
	categories    []string
	maxResults    int
	downloadDelay time.Duration
	ingester      PaperIngester
	fetcher       PDFFetcher
}

func NewHarvestPipeline(srcs []sources.Source, categories []string, maxResults int, downloadDelay time.Duration, ingester PaperIngester, fetcher PDFFetcher) *HarvestPipeline {
	return &HarvestPipeline{
		sources:       srcs,
		categories:    categories,
		maxResults:    maxResults,
		downloadDelay: downloadDelay,
		ingester:      ingester,
		fetcher:       fetcher,
	}
}

// Harvest runs one harvest pass. When categoriesOverride is non-empty it is used
// instead of the configured categories (the manual-trigger API path); otherwise
// the worker's configured categories are used (the scheduled-cron path).
func (h *HarvestPipeline) Harvest(ctx context.Context, categoriesOverride []string) error {
	categories := h.categories
	if len(categoriesOverride) > 0 {
		categories = categoriesOverride
	}
	ingested := 0
	for _, src := range h.sources {
		for _, cat := range categories {
			entries, err := src.FetchRecent(ctx, cat, h.maxResults)
			if err != nil {
				log.Printf("harvest: source=%s category=%s fetch failed: %v", src.Name(), cat, err)
				continue
			}
			for _, e := range entries {
				ok, err := h.ingestEntry(ctx, src, e)
				if err != nil {
					log.Printf("harvest: source=%s id=%s skipped: %v", src.Name(), e.SourceID, err)
					continue
				}
				if ok {
					ingested++
				}
			}
		}
	}
	log.Printf("harvest: completed, %d new paper(s) ingested", ingested)
	return nil
}

func (h *HarvestPipeline) ingestEntry(ctx context.Context, src sources.Source, e sources.Entry) (bool, error) {
	if e.SourceID == "" || e.PDFURL == "" {
		return false, fmt.Errorf("entry missing source id or pdf url")
	}
	exists, err := h.ingester.ExistsBySourceID(ctx, e.SourceID)
	if err != nil {
		return false, fmt.Errorf("dedup check: %w", err)
	}
	if exists {
		return false, nil
	}
	// Politeness delay — only applied when actually about to download, not for dedup-skipped entries.
	if h.downloadDelay > 0 {
		select {
		case <-time.After(h.downloadDelay):
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	data, err := h.fetcher.Fetch(ctx, e.PDFURL)
	if err != nil {
		return false, fmt.Errorf("download pdf: %w", err)
	}
	// Best-effort guard: reject payloads that are not PDFs.
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		return false, fmt.Errorf("source=%s: not a pdf", e.SourceID)
	}
	// Guard zero time so we don't store a bogus year (year 1 from time.Time{}).
	year := int32(0)
	if !e.Published.IsZero() {
		year = int32(e.Published.Year())
	}
	info := papers.SourceInfo{
		SourceType:      src.Name(),
		SourceID:        e.SourceID,
		Filename:        filenameForEntry(e),
		Title:           e.Title,
		Abstract:        e.Abstract,
		DOI:             e.DOI,
		Year:            year,
		PrimaryCategory: e.PrimaryCategory,
	}
	if _, err := h.ingester.IngestPDF(ctx, info, bytes.NewReader(data), int64(len(data)), "application/pdf"); err != nil {
		return false, fmt.Errorf("ingest: %w", err)
	}
	return true, nil
}

func filenameForEntry(e sources.Entry) string {
	// arxiv ids may contain a slash (old style, e.g. cond-mat/0211034); flatten it.
	return strings.ReplaceAll(e.SourceID, "/", "_") + ".pdf"
}

type httpPDFFetcher struct {
	httpClient *http.Client
	userAgent  string
	maxBytes   int64
}

func NewHTTPPDFFetcher(timeout time.Duration, maxBytes int64) *httpPDFFetcher {
	return &httpPDFFetcher{
		httpClient: &http.Client{Timeout: timeout},
		userAgent:  "scholarflow/0.1 (+https://github.com/)",
		maxBytes:   maxBytes,
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
	data, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > f.maxBytes {
		return nil, fmt.Errorf("pdf exceeds max size %d bytes", f.maxBytes)
	}
	return data, nil
}
