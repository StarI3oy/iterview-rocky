package auth

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPasswordRoundTripAndFileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prep_auth.json")
	g, err := Open(path, FastParams())
	if err != nil {
		t.Fatal(err)
	}
	if g.PasswordSet() {
		t.Fatal("password should be unset")
	}
	if err := g.EnablePassword("127.0.0.1", "short"); err == nil {
		t.Fatal("short password accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("auth file created for a rejected password")
	}
	const password = "correct horse battery"
	if err := g.EnablePassword("127.0.0.1", password); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(password)) {
		t.Fatal("password stored in clear text")
	}
	if err := g.Verify(password); err != nil {
		t.Fatal(err)
	}
	if err := g.Verify("wrong horse battery!"); err != ErrBadPassword {
		t.Fatal(err)
	}
	if err := g.EnablePassword("127.0.0.1", "another strong password"); err != ErrPasswordSet {
		t.Fatal(err)
	}
}

func TestApplyEnvMismatchDoesNotTouchFile(t *testing.T) {
	g := openWithPassword(t, "correct horse battery")
	before, err := os.ReadFile(g.Path())
	if err != nil {
		t.Fatal(err)
	}
	_, err = ApplyEnv(g, "wrong horse battery!!", false)
	if err == nil {
		t.Fatal("expected mismatch")
	}
	after, err := os.ReadFile(g.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("auth file changed on mismatch")
	}
	if err := g.Verify("correct horse battery"); err != nil {
		t.Fatal(err)
	}
}

func TestSessionExpiryAndPasswordChange(t *testing.T) {
	g := openWithPassword(t, "correct horse battery")
	start := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return start }
	token, _, err := g.Login("127.0.0.1", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !g.Authenticate(token) {
		t.Fatal("fresh session rejected")
	}
	g.now = func() time.Time { return start.Add(8 * 24 * time.Hour) }
	if g.Authenticate(token) {
		t.Fatal("expired session accepted")
	}
	g.now = func() time.Time { return start }
	token, _, err = g.Login("127.0.0.1", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.ChangePassword("127.0.0.1", "correct horse battery", "a different passphrase"); err != nil {
		t.Fatal(err)
	}
	if g.Authenticate(token) {
		t.Fatal("session survived password change")
	}
	if err := g.Verify("correct horse battery"); err != ErrBadPassword {
		t.Fatal(err)
	}
	if err := g.Verify("a different passphrase"); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptAuthFileIsLeftAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prep_auth.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, FastParams()); err == nil {
		t.Fatal("corrupt file opened")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "{" {
		t.Fatalf("corrupt file rewritten: %q", raw)
	}
}

func TestRecommendedParamsMeetOWASP(t *testing.T) {
	p := RecommendedParams()
	if p.Memory < 19*1024 || p.Time < 2 || p.Threads < 1 || p.SaltLen < 16 || p.KeyLen < 16 {
		t.Fatalf("%+v", p)
	}
}

func openWithPassword(t *testing.T, password string) *Guard {
	t.Helper()
	g, err := Open(filepath.Join(t.TempDir(), "prep_auth.json"), FastParams())
	if err != nil {
		t.Fatal(err)
	}
	if err := g.SetPassword(password); err != nil {
		t.Fatal(err)
	}
	return g
}
