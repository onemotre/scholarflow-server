package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"scholarflow_server/internal/papers"
)

type fakeReader struct {
	job        papers.JobStatus
	jobErr     error
	paper      papers.PaperDetail
	paperErr   error
	gotJobID   uuid.UUID
	gotPaperID uuid.UUID
}

func (f *fakeReader) GetJob(ctx context.Context, jobID uuid.UUID) (papers.JobStatus, error) {
	f.gotJobID = jobID
	return f.job, f.jobErr
}

func (f *fakeReader) GetPaperDetail(ctx context.Context, paperID uuid.UUID) (papers.PaperDetail, error) {
	f.gotPaperID = paperID
	return f.paper, f.paperErr
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
		Status:   "parsed",
		Title:    &title,
		Authors:  []papers.AuthorDTO{{Order: 1, DisplayName: "Ada"}},
		Sections: []papers.SectionDTO{{Order: 1, Text: "Body"}},
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
