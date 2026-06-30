package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireToken(t *testing.T) {
	const secret = "s3cr3t-token"

	// next handler writes 200 + "ok" so we can tell it was reached.
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	cases := []struct {
		name       string
		token      string // configured token
		authHeader string // request Authorization header ("" = omit)
		wantStatus int
	}{
		{"blank token disables auth, no header", "", "", http.StatusOK},
		{"blank token disables auth, junk header", "", "Bearer whatever", http.StatusOK},
		{"token set, missing header", secret, "", http.StatusUnauthorized},
		{"token set, wrong token", secret, "Bearer nope", http.StatusUnauthorized},
		{"token set, wrong scheme", secret, "Basic " + secret, http.StatusUnauthorized},
		{"token set, no Bearer prefix", secret, secret, http.StatusUnauthorized},
		{"token set, correct token", secret, "Bearer " + secret, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := tc.token
			h := RequireToken(func() string { return tok })(next)
			req := httptest.NewRequest(http.MethodPost, "/v1/uploads/papers", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}
