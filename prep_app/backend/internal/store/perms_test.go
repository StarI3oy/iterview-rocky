package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseFilesArePrivate(t *testing.T) {
	dir := t.TempDir()
	interview := filepath.Join(dir, "i.db")
	hard := filepath.Join(dir, "h.db")
	st, err := Open(interview, hard)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, p := range []string{interview, hard} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode %o", p, info.Mode().Perm())
		}
	}
	for _, p := range []string{interview + "-wal", hard + "-wal", interview + "-shm", hard + "-shm"} {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode %o", p, info.Mode().Perm())
		}
	}
}
