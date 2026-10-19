package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"prepapp/internal/store"
)

type fakeAI struct {
	reply string
	err   error
}

func (f fakeAI) Complete(ctx context.Context, system, user string) (string, error) {
	return f.reply, f.err
}

func TestAPI(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "i.db"), filepath.Join(dir, "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	srv := &Server{Store: st}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res := getJSON(t, ts.URL+"/api/health")
	if res["data_ready"] != false || res["ai"] != false {
		t.Fatalf("health %#v", res)
	}
	res = getJSON(t, ts.URL+"/api/bar")
	if res["empty"] != true || res["overall_percent"] != float64(0) {
		t.Fatalf("bar %#v", res)
	}
	res = getJSON(t, ts.URL+"/api/feed?mode=mobile")
	if res["item"] != nil {
		t.Fatalf("feed %#v", res)
	}

	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/api/answer", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("cors %d", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing cors")
	}

	status, body := post(t, ts.URL+"/api/answer", map[string]any{"question_id": 9, "passed": true})
	if status != http.StatusNotFound {
		t.Fatalf("answer %d %s", status, body)
	}
	status, body = post(t, ts.URL+"/api/search", map[string]any{"query": "канал"})
	if status != http.StatusOK {
		t.Fatalf("search %d %s", status, body)
	}

	if _, err := st.Items.Exec(`INSERT INTO prep_items
		(item_type, block, title, prompt, starter_code, reference_code, language)
		VALUES ('code','go','Канал','Почему паникует закрытый канал?','ch := make(chan int)','// close once','go')`); err != nil {
		t.Fatal(err)
	}
	status, body = post(t, ts.URL+"/api/review", map[string]any{"question_id": 1, "code": "package main"})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("review without key %d %s", status, body)
	}
	srv.Completer = fakeAI{reply: `{"passed":true,"score":5,"bugs":[],"style_notes":["ясно"],"follow_up_question":"кто закрывает?"}`}
	status, body = post(t, ts.URL+"/api/review", map[string]any{"question_id": 1, "code": "close(ch)"})
	if status != http.StatusOK {
		t.Fatalf("review %d %s", status, body)
	}
	var verdict map[string]any
	if err := json.Unmarshal(body, &verdict); err != nil {
		t.Fatal(err)
	}
	if verdict["passed"] != true {
		t.Fatalf("%s", body)
	}
	item, err := st.Get(1)
	if err != nil || !item.Closed {
		t.Fatalf("not closed %+v %v", item, err)
	}

	status, body = post(t, ts.URL+"/api/search", map[string]any{"query": "закрытый канал"})
	if status != http.StatusOK {
		t.Fatalf("search2 %d %s", status, body)
	}
	var found map[string]any
	if err := json.Unmarshal(body, &found); err != nil {
		t.Fatal(err)
	}
	matches, _ := found["matches"].([]any)
	if len(matches) == 0 {
		t.Fatalf("no matches %s", body)
	}
	if found["explanation"] != "кто закрывает?" && found["explanation"] == "" {
		t.Fatalf("explanation %#v", found["explanation"])
	}
}

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s -> %d %#v", url, resp.StatusCode, m)
	}
	return m
}

func post(t *testing.T, url string, payload any) (int, []byte) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.Bytes()
}
