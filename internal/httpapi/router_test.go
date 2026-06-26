package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// doReq issues one request against h and returns the status code.
func doReq(t *testing.T, h http.Handler, method, path, auth string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestRouterAuthSplit(t *testing.T) {
	// Token set: healthz open, write blocked without token.
	h := NewRouter(Dependencies{WriteAPIToken: "tok"})

	if code := doReq(t, h, http.MethodGet, "/healthz", ""); code != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", code)
	}
	// No handlers wired, but the auth middleware runs before the (absent)
	// route, so an unauthenticated write must be 401, not 404.
	if code := doReq(t, h, http.MethodPost, "/v1/uploads/papers", ""); code != http.StatusUnauthorized {
		t.Fatalf("unauth upload = %d, want 401", code)
	}
	if code := doReq(t, h, http.MethodPost, "/v1/jobs/abc/retry", ""); code != http.StatusUnauthorized {
		t.Fatalf("unauth retry = %d, want 401", code)
	}
	if code := doReq(t, h, http.MethodPost, "/v1/harvest/arxiv", ""); code != http.StatusUnauthorized {
		t.Fatalf("unauth harvest = %d, want 401", code)
	}
}
