package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrateLegacyShape(t *testing.T) {
	dir := t.TempDir()
	interview := filepath.Join(dir, "interview_questions.db")
	hard := filepath.Join(dir, "hard_concepts.db")

	raw, err := sql.Open("sqlite", "file:"+interview)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE prep_items (
		id INTEGER PRIMARY KEY,
		question TEXT,
		topic TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO prep_items (question, topic) VALUES ('Что такое GMP?', 'go')`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(interview, hard)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	item, err := s.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Prompt != "Что такое GMP?" {
		t.Fatalf("prompt: %q", item.Prompt)
	}
	if item.Block != BlockGo {
		t.Fatalf("block: %q", item.Block)
	}
	if item.ItemType != TypeFlashcard {
		t.Fatalf("type: %q", item.ItemType)
	}
	n, err := s.CountItems()
	if err != nil || n != 1 {
		t.Fatalf("count %d err %v", n, err)
	}
}

func TestBarAnswerFeedSearch(t *testing.T) {
	s := openTest(t)
	if _, err := s.Items.Exec(`INSERT INTO prep_items
		(item_type, block, title, prompt, options, correct_index, explanation)
		VALUES ('quiz','go','GMP','Сколько потоков?','["один","много","ноль","сто"]', 1, 'по числу P')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Items.Exec(`INSERT INTO prep_items
		(item_type, block, company, title, prompt, answer_skeleton)
		VALUES ('flashcard','Kafka','Авито','Лаг','Почему растёт лаг консьюмера?','отставание offset')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Items.Exec(`INSERT INTO prep_items
		(item_type, block, title, prompt, starter_code, reference_code, language, review_criteria)
		VALUES ('code','algo','Two sum','Найди пару','func twoSum() {}','func twoSum() int { return 0 }','go','O(n)')`); err != nil {
		t.Fatal(err)
	}

	bar, err := s.Bar()
	if err != nil {
		t.Fatal(err)
	}
	if bar.Total != 3 || bar.Done != 0 || bar.Empty {
		t.Fatalf("bar %+v", bar)
	}

	oral, err := s.Next(FeedFilter{Mode: "mobile", Kind: "sprint"})
	if err != nil || oral == nil {
		t.Fatalf("feed %v %v", oral, err)
	}
	if oral.ItemType == TypeCode {
		t.Fatalf("mobile feed returned code: %+v", oral)
	}

	code, err := s.Next(FeedFilter{Mode: "web", Kind: "code"})
	if err != nil || code == nil || code.ItemType != TypeCode {
		t.Fatalf("web feed %+v %v", code, err)
	}
	if code.ReferenceCode != "" && code.ReferenceCode == "should-not-matter" {
		t.Fatal("sanity")
	}

	wrong := 0
	res, err := s.RecordAnswer(AnswerInput{QuestionID: oral.ID, SelectedIndex: &wrong, UserAnswer: "один"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("expected fail")
	}
	hard, err := s.ListHard()
	if err != nil || len(hard) != 1 || hard[0].FailCount != 1 {
		t.Fatalf("hard %+v %v", hard, err)
	}
	_, err = s.RecordAnswer(AnswerInput{QuestionID: oral.ID, SelectedIndex: &wrong, UserAnswer: "снова"})
	if err != nil {
		t.Fatal(err)
	}
	hard, err = s.ListHard()
	if err != nil || hard[0].FailCount != 2 {
		t.Fatalf("fail_count %+v %v", hard, err)
	}

	right := 1
	res, err = s.RecordAnswer(AnswerInput{QuestionID: oral.ID, SelectedIndex: &right})
	if err != nil || !res.Passed {
		t.Fatalf("pass %+v %v", res, err)
	}
	hard, err = s.ListHard()
	if err != nil || len(hard) != 0 {
		t.Fatalf("resolved %+v %v", hard, err)
	}

	if _, err := s.RecordAnswer(AnswerInput{QuestionID: 2, Passed: false, UserAnswer: "не помню"}); err != nil {
		t.Fatal(err)
	}
	hits, hhits, err := s.Search("лаг консьюмера Авито", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].ID != 2 {
		t.Fatalf("hits %+v", hits)
	}
	if len(hhits) == 0 {
		t.Fatal("expected hard hit")
	}

	again, err := s.Next(FeedFilter{Mode: "mobile", Kind: "hard"})
	if err != nil || again == nil || again.ID != 2 {
		t.Fatalf("hard feed %+v %v", again, err)
	}

	if err := s.Enqueue(code.ID); err != nil {
		t.Fatal(err)
	}
	q, err := s.Queue()
	if err != nil || len(q) != 1 || q[0].ID != code.ID {
		t.Fatalf("queue %+v %v", q, err)
	}
	if err := s.MarkReview(*code, true, ""); err != nil {
		t.Fatal(err)
	}
	closed, err := s.Get(code.ID)
	if err != nil || !closed.Closed {
		t.Fatalf("closed %+v %v", closed, err)
	}
	q, err = s.Queue()
	if err != nil || len(q) != 0 {
		t.Fatalf("queue after pass %+v %v", q, err)
	}
	bar, err = s.Bar()
	if err != nil {
		t.Fatal(err)
	}
	if bar.Done != 2 {
		t.Fatalf("done %d", bar.Done)
	}
}

func openTest(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "i.db"), filepath.Join(dir, "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
