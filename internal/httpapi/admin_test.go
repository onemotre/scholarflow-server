package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"scholarflow_server/internal/papers"
)

type fakeAdmin struct {
	deleteErr error
	job       papers.JobStatus
	jobErr    error
	gotID     uuid.UUID
}

func (f *fakeAdmin) DeletePaper(ctx context.Context, id uuid.UUID) error {
	f.gotID = id
	return f.deleteErr
}
func (f *fakeAdmin) Reprocess(ctx context.Context, id uuid.UUID) (papers.JobStatus, error) {
	f.gotID = id
	return f.job, f.jobErr
}
func (f *fakeAdmin) RegenerateCard(ctx context.Context, id uuid.UUID) (papers.JobStatus, error) {
	f.gotID = id
	return f.job, f.jobErr
}

func newAdminServer(admin PaperAdmin) *httptest.Server {
	return httptest.NewServer(NewRouter(Dependencies{AdminHandler: NewAdminHandler(admin)}))
}

func doAdminReq(t *testing.T, method, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestDeletePaperReturns204(t *testing.T) {
	admin := &fakeAdmin{}
	srv := newAdminServer(admin)
	defer srv.Close()
	id := uuid.New()
	resp := doAdminReq(t, http.MethodDelete, srv.URL+"/v1/papers/"+id.String())
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if admin.gotID != id {
		t.Fatalf("id passed = %s, want %s", admin.gotID, id)
	}
}

func TestDeletePaperNotFound(t *testing.T) {
	srv := newAdminServer(&fakeAdmin{deleteErr: papers.ErrNotFound})
	defer srv.Close()
	resp := doAdminReq(t, http.MethodDelete, srv.URL+"/v1/papers/"+uuid.New().String())
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestDeletePaperBadID(t *testing.T) {
	srv := newAdminServer(&fakeAdmin{})
	defer srv.Close()
	resp := doAdminReq(t, http.MethodDelete, srv.URL+"/v1/papers/not-a-uuid")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestReprocessReturns202(t *testing.T) {
	srv := newAdminServer(&fakeAdmin{job: papers.JobStatus{Status: "queued"}})
	defer srv.Close()
	resp := doAdminReq(t, http.MethodPost, srv.URL+"/v1/papers/"+uuid.New().String()+"/reprocess")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
}

func TestRegenerateCardConflictWhenUnparsed(t *testing.T) {
	srv := newAdminServer(&fakeAdmin{jobErr: papers.ErrNotRetryable})
	defer srv.Close()
	resp := doAdminReq(t, http.MethodPost, srv.URL+"/v1/papers/"+uuid.New().String()+"/reread")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}
