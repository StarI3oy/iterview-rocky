package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"prepapp/internal/auth"
	"prepapp/internal/store"
)

func TestRemoteAPIClosedUntilPassword(t *testing.T) {
	srv, g := newGuardServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/bar", nil)
	req.RemoteAddr = "203.0.113.8:40000"
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("remote %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/bar", nil)
	req.RemoteAddr = "127.0.0.1:40000"
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("loopback %d %s", rr.Code, rr.Body.String())
	}

	body := `{"password":"correct horse battery","confirm":"correct horse battery"}`
	req = httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(body))
	req.RemoteAddr = "203.0.113.8:40000"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Prep-Client", "1")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("remote setup %d %s", rr.Code, rr.Body.String())
	}
	if g.PasswordSet() {
		t.Fatal("remote setup stored a password")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:40000"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Prep-Client", "1")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("local setup %d %s", rr.Code, rr.Body.String())
	}
}

func TestLoginCookieAndDataUntouched(t *testing.T) {
	srv, _ := newGuardServer(t, "correct horse battery")
	if _, err := srv.Store.Items.Exec(`INSERT INTO prep_items (item_type, block, title, prompt) VALUES ('flashcard','go','Канал','текст')`); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	status, resp := postAuth(t, ts.URL+"/api/auth/login", map[string]any{"password": "nope nope nope"})
	if status != http.StatusUnauthorized {
		t.Fatalf("bad login %d", status)
	}
	resp.Body.Close()

	status, resp = postAuth(t, ts.URL+"/api/auth/login", map[string]any{"password": "correct horse battery"})
	if status != http.StatusOK {
		t.Fatalf("login %d", status)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("login response is cacheable")
	}
	var session *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName {
			session = c
		}
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	token, _ := payload["token"].(string)
	if token == "" || session == nil || !session.HttpOnly || session.SameSite != http.SameSiteLaxMode {
		t.Fatalf("token %q cookie %#v", token, session)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/bar", nil)
	req.RemoteAddr = "203.0.113.9:10"
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("open remote %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/bar", nil)
	req.RemoteAddr = "203.0.113.9:10"
	req.AddCookie(session)
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("cookie without header %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/items/1", nil)
	req.RemoteAddr = "203.0.113.9:10"
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("bearer %d %s", rr.Code, rr.Body.String())
	}

	status, resp = postAuthToken(t, ts.URL+"/api/auth/password", token, map[string]any{
		"current_password": "correct horse battery",
		"password":         "a different passphrase",
		"confirm":          "a different passphrase",
	})
	resp.Body.Close()
	if status != http.StatusOK {
		t.Fatalf("change %d", status)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/items/1", nil)
	req.RemoteAddr = "203.0.113.9:10"
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("old token %d", rr.Code)
	}
	item, err := srv.Store.Get(1)
	if err != nil || item.Title != "Канал" {
		t.Fatalf("card changed %+v %v", item, err)
	}
}

func TestLoginRateLimit(t *testing.T) {
	_, g := newGuardServer(t, "correct horse battery")
	ts := httptest.NewServer((&Server{Guard: g}).Handler())
	defer ts.Close()
	for i := 0; i < 5; i++ {
		status, resp := postAuth(t, ts.URL+"/api/auth/login", map[string]any{"password": "wrong horse battery!"})
		resp.Body.Close()
		if status != http.StatusUnauthorized {
			t.Fatalf("attempt %d -> %d", i, status)
		}
	}
	status, resp := postAuth(t, ts.URL+"/api/auth/login", map[string]any{"password": "correct horse battery"})
	resp.Body.Close()
	if status != http.StatusTooManyRequests {
		t.Fatalf("lockout %d", status)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
}

func TestPublicHealthHidesCounts(t *testing.T) {
	srv, _ := newGuardServer(t, "correct horse battery")
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.RemoteAddr = "203.0.113.8:9"
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["items"]; ok || body["ai"] != nil || body["ok"] != true {
		t.Fatalf("%#v", body)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.RemoteAddr = "127.0.0.1:9"
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["items"]; !ok {
		t.Fatalf("loopback health %#v", body)
	}
}

func newGuardServer(t *testing.T, password string) (*Server, *auth.Guard) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "i.db"), filepath.Join(dir, "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	g, err := auth.Open(filepath.Join(dir, "prep_auth.json"), auth.FastParams())
	if err != nil {
		t.Fatal(err)
	}
	if password != "" {
		if err := g.SetPassword(password); err != nil {
			t.Fatal(err)
		}
	}
	return &Server{Store: st, Guard: g}, g
}

func postAuth(t *testing.T, url string, payload any) (int, *http.Response) {
	t.Helper()
	return postAuthToken(t, url, "", payload)
}

func postAuthToken(t *testing.T, url, token string, payload any) (int, *http.Response) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Prep-Client", "1")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp
}

func TestCORSReflectsOnlyListedOrigin(t *testing.T) {
	srv := &Server{AllowedOrigins: []string{"http://localhost:1234"}}
	req := httptest.NewRequest(http.MethodOptions, "/api/bar", nil)
	req.Header.Set("Origin", "http://evil.example")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("reflected unlisted origin")
	}
	req = httptest.NewRequest(http.MethodOptions, "/api/bar", nil)
	req.Header.Set("Origin", "http://localhost:1234")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatal(rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "http://localhost:1234" {
		t.Fatal(rr.Header().Get("Access-Control-Allow-Origin"))
	}
	if rr.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("credentials")
	}
	_, _ = io.Copy(io.Discard, rr.Body)
}
