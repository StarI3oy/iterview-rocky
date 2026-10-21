package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"prepapp/internal/ai"
	"prepapp/internal/store"
)

type fakeDir struct {
	page    ai.CatalogPage
	err     error
	calls   int
	name    string
	resolve error
	hasKey  bool
}

func (f *fakeDir) Search(ctx context.Context, query string) (ai.CatalogPage, error) {
	f.calls++
	if f.err != nil {
		return ai.CatalogPage{}, f.err
	}
	page := f.page
	page.Query = query
	return page, nil
}

func (f *fakeDir) Resolve(ctx context.Context, modelID, provider string) (string, error) {
	if f.resolve != nil {
		return "", f.resolve
	}
	return f.name, nil
}

func (f *fakeDir) HasKey() bool { return f.hasKey }

func TestModelSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "i.db"), filepath.Join(dir, "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	sel, err := ai.OpenSelection(filepath.Join(dir, "prep_settings.json"), "openai/gpt-4o-mini")
	if err != nil {
		t.Fatal(err)
	}
	catalog := &fakeDir{
		hasKey: true,
		name:   "Anthropic",
		page: ai.CatalogPage{
			MatchCount: 1,
			Results: []ai.CatalogModel{{
				ID:   "anthropic/claude-haiku",
				Name: "Claude Haiku",
			}},
		},
	}
	upstreamBodies := make(chan string, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		upstreamBodies <- string(raw)
		payload, _ := json.Marshal(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"content": `{"passed":true,"score":4,"bugs":[],"style_notes":[],"follow_up_question":""}`,
			}}},
		})
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()
	client := ai.New("test-key", sel)
	client.BaseURL = upstream.URL

	srv := &Server{Store: st, Completer: client, Models: catalog, Selection: sel}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res := getJSON(t, ts.URL+"/api/settings/model")
	if res["model"] != "openai/gpt-4o-mini" || res["source"] != "env" || res["ai"] != true || res["latency_needs_key"] != false {
		t.Fatalf("%#v", res)
	}
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/models?q=h", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("short query %d", resp.StatusCode)
	}
	req, err = http.NewRequest(http.MethodGet, ts.URL+"/api/models?q=haiku", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search %d", resp.StatusCode)
	}
	var page map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if page["query"] != "haiku" || page["match_count"] != float64(1) {
		t.Fatalf("%#v", page)
	}

	status, body := post(t, ts.URL+"/api/settings/model", map[string]any{
		"model": "anthropic/claude-haiku", "provider": "anthropic",
	})
	if status != http.StatusOK {
		t.Fatalf("save %d %s", status, body)
	}
	var saved map[string]any
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["model"] != "anthropic/claude-haiku" || saved["provider"] != "anthropic" || saved["provider_name"] != "Anthropic" || saved["source"] != "settings" {
		t.Fatalf("%#v", saved)
	}

	if _, err := st.Items.Exec(`INSERT INTO prep_items
		(item_type, block, title, prompt, starter_code, reference_code, language)
		VALUES ('code','go','Канал','Почему?','ch','//','go')`); err != nil {
		t.Fatal(err)
	}
	status, body = post(t, ts.URL+"/api/review", map[string]any{"question_id": 1, "code": "close(ch)"})
	if status != http.StatusOK {
		t.Fatalf("review %d %s", status, body)
	}
	raw := <-upstreamBodies
	if !strings.Contains(raw, `"model":"anthropic/claude-haiku"`) || !strings.Contains(raw, `"order":["anthropic"]`) || !strings.Contains(raw, `"allow_fallbacks":false`) {
		t.Fatalf("upstream %s", raw)
	}

	status, body = post(t, ts.URL+"/api/settings/model/reset", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("reset %d %s", status, body)
	}
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["model"] != "openai/gpt-4o-mini" || saved["source"] != "env" || saved["provider"] != "" {
		t.Fatalf("%#v", saved)
	}
}

func TestModelSearchClosedForRemote(t *testing.T) {
	srv, _ := newGuardServer(t, "correct horse battery")
	catalog := &fakeDir{}
	srv.Models = catalog
	req := httptest.NewRequest(http.MethodGet, "/api/models?q=haiku", nil)
	req.RemoteAddr = "203.0.113.8:40000"
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("remote %d %s", rr.Code, rr.Body.String())
	}
	if catalog.calls != 0 {
		t.Fatal("catalog called without a session")
	}
}
