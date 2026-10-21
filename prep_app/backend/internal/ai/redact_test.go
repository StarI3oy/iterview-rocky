package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompleteRedactsKey(t *testing.T) {
	const key = "sk-test-secret-value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"rejected ` + key + ` and ` + authz + `"}}`))
	}))
	defer srv.Close()

	c := New(key, "openai/gpt-4o-mini")
	c.BaseURL = srv.URL
	_, err := c.Complete(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), key) || strings.Contains(err.Error(), "Bearer "+key) {
		t.Fatalf("secret leaked: %v", err)
	}
}
