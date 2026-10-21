package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestCompleteUsesSavedProvider(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("body: %v", err)
		}
		bodies = append(bodies, body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	sel, err := OpenSelection(filepath.Join(t.TempDir(), "prep_settings.json"), "openai/gpt-4o-mini")
	if err != nil {
		t.Fatal(err)
	}
	client := New("test-key", sel)
	client.BaseURL = srv.URL
	if _, err := client.Complete(context.Background(), "sys", "user"); err != nil {
		t.Fatal(err)
	}
	if _, err := sel.Save("anthropic/claude-haiku", "amazon-bedrock", "Amazon Bedrock"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(context.Background(), "sys", "user"); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 {
		t.Fatalf("calls %d", len(bodies))
	}
	if bodies[0]["model"] != "openai/gpt-4o-mini" {
		t.Fatalf("env model %#v", bodies[0]["model"])
	}
	if _, ok := bodies[0]["provider"]; ok {
		t.Fatalf("provider pinned without a choice: %#v", bodies[0]["provider"])
	}
	if bodies[1]["model"] != "anthropic/claude-haiku" {
		t.Fatalf("saved model %#v", bodies[1]["model"])
	}
	provider, _ := bodies[1]["provider"].(map[string]any)
	if provider["allow_fallbacks"] != false {
		t.Fatalf("fallbacks %#v", provider)
	}
	order, _ := provider["order"].([]any)
	if len(order) != 1 || order[0] != "amazon-bedrock" {
		t.Fatalf("order %#v", provider["order"])
	}
}
