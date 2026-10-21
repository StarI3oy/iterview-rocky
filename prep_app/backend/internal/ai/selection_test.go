package ai

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectionUsesEnvUntilSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prep_settings.json")
	sel, err := OpenSelection(path, "")
	if err != nil {
		t.Fatal(err)
	}
	cur := sel.Current()
	if cur.Model != defaultModel || cur.Source != "env" || cur.Provider != "" {
		t.Fatalf("%+v", cur)
	}
	cur, err = sel.Save("anthropic/claude-haiku", "anthropic", "Anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if cur.Model != "anthropic/claude-haiku" || cur.Provider != "anthropic" || cur.Source != "settings" {
		t.Fatalf("%+v", cur)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	again, err := OpenSelection(path, "openai/gpt-4o")
	if err != nil {
		t.Fatal(err)
	}
	cur = again.Current()
	if cur.Model != "anthropic/claude-haiku" || cur.EnvModel != "openai/gpt-4o" || cur.ProviderName != "Anthropic" {
		t.Fatalf("%+v", cur)
	}
	cur, err = again.Reset()
	if err != nil {
		t.Fatal(err)
	}
	if cur.Model != "openai/gpt-4o" || cur.Source != "env" || cur.Provider != "" {
		t.Fatalf("%+v", cur)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file remained: %v", err)
	}
}

func TestModelIDAllowsLatestAlias(t *testing.T) {
	if !validModelID("~anthropic/claude-haiku-latest") || !validModelID("openai/gpt-4o-mini:free") {
		t.Fatal("known ids rejected")
	}
	for _, id := range []string{"anthropic/../x", "/x", "has space/model", "a/b/c", "~"} {
		if validModelID(id) {
			t.Fatalf("accepted %s", id)
		}
	}
}

func TestSelectionRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prep_settings.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSelection(path, ""); err == nil {
		t.Fatal("corrupt file accepted")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "{" {
		t.Fatalf("corrupt file changed: %q %v", raw, err)
	}
	sel, err := OpenSelection("", "not a model")
	if err == nil || sel != nil {
		t.Fatal("bad env model accepted")
	}
}
