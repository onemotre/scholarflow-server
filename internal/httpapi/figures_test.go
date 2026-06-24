package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"scholarflow_server/internal/papers"
)

type fakeFigureReader struct {
	key string
	err error
}

func (f *fakeFigureReader) GetFigureImageKey(ctx context.Context, paperID, figureID uuid.UUID) (string, error) {
	return f.key, f.err
}

type fakeObjectStore struct {
	body string
}

func (s *fakeObjectStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(s.body)), nil
}

func TestGetFigureImageReturnsPNG(t *testing.T) {
	h := NewFigureImageHandler(&fakeFigureReader{key: "papers/x/figures/1.png"}, &fakeObjectStore{body: "PNGDATA"})
	router := NewRouter(Dependencies{FigureImageHandler: h})

	url := "/v1/papers/" + uuid.New().String() + "/figures/" + uuid.New().String() + "/image"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type = %q, want image/png", ct)
	}
	if rec.Body.String() != "PNGDATA" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestGetFigureImageNotFound(t *testing.T) {
	h := NewFigureImageHandler(&fakeFigureReader{err: papers.ErrNotFound}, &fakeObjectStore{})
	router := NewRouter(Dependencies{FigureImageHandler: h})

	url := "/v1/papers/" + uuid.New().String() + "/figures/" + uuid.New().String() + "/image"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetFigureImageInvalidID(t *testing.T) {
	h := NewFigureImageHandler(&fakeFigureReader{}, &fakeObjectStore{})
	router := NewRouter(Dependencies{FigureImageHandler: h})

	url := "/v1/papers/" + uuid.New().String() + "/figures/not-a-uuid/image"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
