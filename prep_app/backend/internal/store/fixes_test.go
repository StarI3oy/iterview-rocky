package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func seedCurriculum(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.Items.Exec(`INSERT INTO prep_tracks (code, title) VALUES
		('dumps','Секции'),('patterns','Паттерны'),('system_design','SD')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Items.Exec(`INSERT INTO prep_items (id, item_type, track, title, prompt) VALUES
		(1,'flashcard','dumps','a','a?'),
		(2,'flashcard','dumps','b','b?'),
		(3,'code','patterns','c','c?'),
		(4,'flashcard','system_design','d','d?')`); err != nil {
		t.Fatal(err)
	}
}

func at(day string) func() time.Time {
	return func() time.Time {
		v, _ := time.Parse("2006-01-02", day)
		return v
	}
}

func TestRecordAnswerRejectsCode(t *testing.T) {
	s := openTest(t)
	seedCurriculum(t, s)
	_, err := s.RecordAnswer(AnswerInput{QuestionID: 3, Passed: true})
	if !errors.Is(err, ErrCodeNeedsReview) {
		t.Fatalf("err %v", err)
	}
	it, err := s.Get(3)
	if err != nil {
		t.Fatal(err)
	}
	if it.TimesAsked != 0 {
		t.Fatalf("code item touched: %+v", it)
	}
	if isMastered(TypeCode, sql.NullInt64{Int64: 1, Valid: true}, 0, "done") {
		t.Fatal("code with status=done but not closed must not be mastered")
	}
}

func TestSprintSkipsMastered(t *testing.T) {
	s := openTest(t)
	seedCurriculum(t, s)
	s.Now = at("2026-10-20") // even day: dumps

	for _, want := range []int64{1, 2} {
		it, err := s.Next(FeedFilter{Mode: "mobile", Kind: "sprint"})
		if err != nil || it == nil || it.ID != want {
			t.Fatalf("want %d, got %+v %v", want, it, err)
		}
		if _, err := s.RecordAnswer(AnswerInput{QuestionID: want, Passed: true}); err != nil {
			t.Fatal(err)
		}
	}
	it, err := s.Next(FeedFilter{Mode: "mobile", Kind: "sprint"})
	if err != nil || it == nil || it.ID != 4 {
		t.Fatalf("closed track must fall back to unfinished oral card, got %+v %v", it, err)
	}
}

func TestMobileSprintOnCodeDay(t *testing.T) {
	s := openTest(t)
	seedCurriculum(t, s)
	s.Now = at("2026-10-21") // odd day: patterns (code only)

	it, err := s.Next(FeedFilter{Mode: "mobile", Kind: "sprint"})
	if err != nil || it == nil || it.ItemType == TypeCode {
		t.Fatalf("mobile sprint must give an oral card, got %+v %v", it, err)
	}
	it, err = s.Next(FeedFilter{Mode: "web", Kind: "sprint"})
	if err != nil || it == nil || it.ID != 3 {
		t.Fatalf("web sprint must give the code task, got %+v %v", it, err)
	}
	if err := s.MarkReview(*it, true, ""); err != nil {
		t.Fatal(err)
	}
	it, err = s.Next(FeedFilter{Mode: "web", Kind: "sprint"})
	if err != nil || it == nil || it.ID == 3 {
		t.Fatalf("closed code task returned again: %+v %v", it, err)
	}
}

const collisionExport = `{
	"prep_tracks": [{"code":"dumps","title":"Секции","why":"","how_to_train":"","time_min":25}],
	"prep_items": [{"id": 1, "track": "dumps", "question": "Импортированный?", "answer": "да", "status": "new"}],
	"questions": [{"id": 7, "topic": "Go", "question": "Как устроен slice?", "answer": "ptr,len,cap"}]
}`

func TestImportDoesNotOverwriteForeignRows(t *testing.T) {
	s := openTest(t)
	if _, err := s.Items.Exec(`INSERT INTO prep_items (id, item_type, title, prompt, times_asked, last_result)
		VALUES (1,'flashcard','свой','Моя карточка',5,1), (10007,'flashcard','свой2','Ещё моя',0,NULL)`); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "k.json")
	if err := os.WriteFile(path, []byte(collisionExport), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := s.ImportKnowledge(path); err != nil {
			t.Fatal(err)
		}
	}
	own, err := s.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if own.Prompt != "Моя карточка" || own.TimesAsked != 5 || own.Track != "" {
		t.Fatalf("foreign row overwritten: %+v", own)
	}
	own2, err := s.Get(10007)
	if err != nil || own2.Prompt != "Ещё моя" {
		t.Fatalf("foreign row overwritten: %+v %v", own2, err)
	}
	var n int
	if err := s.Items.QueryRow(`SELECT COUNT(*) FROM prep_items`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("rows %d, want 4 (2 own + 2 imported, no duplicates)", n)
	}
	var id int64
	if err := s.Items.QueryRow(`SELECT id FROM prep_items WHERE source_key = 'prep:1'`).Scan(&id); err != nil || id == 1 {
		t.Fatalf("imported card id %d %v", id, err)
	}
}

func TestLegacyImportedRowsGetSourceKey(t *testing.T) {
	dir := t.TempDir()
	items, hard := filepath.Join(dir, "i.db"), filepath.Join(dir, "h.db")
	s, err := Open(items, hard)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "k.json")
	if err := os.WriteFile(path, []byte(collisionExport), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportKnowledge(path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Items.Exec(`DROP INDEX prep_items_source_key`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Items.Exec(`UPDATE prep_items SET source_key = NULL, times_asked = 3`); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	s, err = Open(items, hard)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.ImportKnowledge(path); err != nil {
		t.Fatal(err)
	}
	var n, asked int
	if err := s.Items.QueryRow(`SELECT COUNT(*), MIN(times_asked) FROM prep_items`).Scan(&n, &asked); err != nil {
		t.Fatal(err)
	}
	if n != 2 || asked != 3 {
		t.Fatalf("rows %d, min times_asked %d", n, asked)
	}
}
