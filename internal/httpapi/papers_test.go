package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"scholarflow_server/internal/papers"
)

type fakeReader struct {
	job          papers.JobStatus
	jobErr       error
	paper        papers.PaperDetail
	paperErr     error
	summaries    []papers.PaperSummary
	summariesErr error
	gotJobID     uuid.UUID
	gotPaperID   uuid.UUID
}

func (f *fakeReader) GetJob(ctx context.Context, jobID uuid.UUID) (papers.JobStatus, error) {
	f.gotJobID = jobID
	return f.job, f.jobErr
}

func (f *fakeReader) GetPaperDetail(ctx context.Context, paperID uuid.UUID) (papers.PaperDetail, error) {
	f.gotPaperID = paperID
	return f.paper, f.paperErr
}

func (f *fakeReader) ListPapers(ctx context.Context) ([]papers.PaperSummary, error) {
	return f.summaries, f.summariesErr
}

func newTestServer(reader PaperReader) *httptest.Server {
	return httptest.NewServer(NewRouter(Dependencies{ReadHandler: NewReadHandler(reader)}))
}

func TestGetJobReturnsStatus(t *testing.T) {
	jobID := uuid.New()
	reader := &fakeReader{job: papers.JobStatus{JobID: jobID, Status: "parsed"}}
	srv := newTestServer(reader)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/jobs/" + jobID.String())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got papers.JobStatus
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "parsed" {
		t.Fatalf("status = %q", got.Status)
	}
	if reader.gotJobID != jobID {
		t.Fatalf("job id passed = %s, want %s", reader.gotJobID, jobID)
	}
}

func TestGetJobNotFound(t *testing.T) {
	reader := &fakeReader{jobErr: papers.ErrNotFound}
	srv := newTestServer(reader)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/jobs/" + uuid.New().String())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestGetJobInvalidID(t *testing.T) {
	reader := &fakeReader{}
	srv := newTestServer(reader)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/jobs/not-a-uuid")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGetPaperReturnsDetail(t *testing.T) {
	paperID := uuid.New()
	title := "Test Paper"
	reader := &fakeReader{paper: papers.PaperDetail{
		PaperID:  paperID,
		Status:   "completed",
		Title:    &title,
		Authors:  []papers.AuthorDTO{{Order: 1, DisplayName: "Ada"}},
		Sections: []papers.SectionDTO{{Order: 1, Text: "Body"}},
		Card:     json.RawMessage(`{"background":"bg"}`),
		Figures:  []papers.FigureDTO{{Label: "Figure 1", Kind: "figure", Caption: "cap", Order: 1}},
	}}
	srv := newTestServer(reader)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/papers/" + paperID.String())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got papers.PaperDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Title == nil || *got.Title != "Test Paper" {
		t.Fatalf("title = %v", got.Title)
	}
	if len(got.Authors) != 1 {
		t.Fatalf("authors = %d, want 1", len(got.Authors))
	}
	if reader.gotPaperID != paperID {
		t.Fatalf("paper id passed = %s, want %s", reader.gotPaperID, paperID)
	}
	if got.Card == nil || !strings.Contains(string(got.Card), "bg") {
		t.Fatalf("card = %s", string(got.Card))
	}
	if len(got.Figures) != 1 || got.Figures[0].Label != "Figure 1" {
		t.Fatalf("figures = %#v", got.Figures)
	}
}

func TestListPapers(t *testing.T) {
	title := "A Paper"
	h := NewReadHandler(&fakeReader{summaries: []papers.PaperSummary{
		{PaperID: uuid.New(), Title: &title, Status: "completed", UploadedFilename: "a.pdf"},
	}})
	req := httptest.NewRequest(http.MethodGet, "/v1/papers", nil)
	rr := httptest.NewRecorder()
	NewRouter(Dependencies{ReadHandler: h}).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got []papers.PaperSummary
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Status != "completed" {
		t.Fatalf("summaries = %#v", got)
	}
}

func TestListPapersIncludesSourceFields(t *testing.T) {
	title := "Arxiv Paper"
	sourceID := "2301.00001"
	category := "cs.CL"
	h := NewReadHandler(&fakeReader{summaries: []papers.PaperSummary{
		{
			PaperID:          uuid.New(),
			Title:            &title,
			Status:           "completed",
			UploadedFilename: "2301.00001.pdf",
			SourceType:       "arxiv",
			SourceID:         &sourceID,
			PrimaryCategory:  &category,
		},
	}})
	req := httptest.NewRequest(http.MethodGet, "/v1/papers", nil)
	rr := httptest.NewRecorder()
	NewRouter(Dependencies{ReadHandler: h}).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got []papers.PaperSummary
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].SourceType != "arxiv" {
		t.Fatalf("source_type = %q, want arxiv", got[0].SourceType)
	}
	if got[0].SourceID == nil || *got[0].SourceID != "2301.00001" {
		t.Fatalf("source_id = %v, want 2301.00001", got[0].SourceID)
	}
	if got[0].PrimaryCategory == nil || *got[0].PrimaryCategory != "cs.CL" {
		t.Fatalf("primary_category = %v, want cs.CL", got[0].PrimaryCategory)
	}
}

func TestGetPaperNotFound(t *testing.T) {
	reader := &fakeReader{paperErr: papers.ErrNotFound}
	srv := newTestServer(reader)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/papers/" + uuid.New().String())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

type fakeRetrier struct {
	job papers.JobStatus
	err error
}

func (f *fakeRetrier) RetryJob(ctx context.Context, jobID uuid.UUID) (papers.JobStatus, error) {
	return f.job, f.err
}

func TestRetryHandlerAccepted(t *testing.T) {
	h := NewRetryHandler(&fakeRetrier{job: papers.JobStatus{Status: "queued"}})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+uuid.New().String()+"/retry", nil)
	rr := httptest.NewRecorder()
	NewRouter(Dependencies{RetryHandler: h}).ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rr.Code)
	}
}

func TestRetryHandlerNotFound(t *testing.T) {
	h := NewRetryHandler(&fakeRetrier{err: papers.ErrNotFound})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+uuid.New().String()+"/retry", nil)
	rr := httptest.NewRecorder()
	NewRouter(Dependencies{RetryHandler: h}).ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestRetryHandlerConflict(t *testing.T) {
	h := NewRetryHandler(&fakeRetrier{err: papers.ErrNotRetryable})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+uuid.New().String()+"/retry", nil)
	rr := httptest.NewRecorder()
	NewRouter(Dependencies{RetryHandler: h}).ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
}

func TestRetryHandlerBadUUID(t *testing.T) {
	h := NewRetryHandler(&fakeRetrier{})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/not-a-uuid/retry", nil)
	rr := httptest.NewRecorder()
	NewRouter(Dependencies{RetryHandler: h}).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}
