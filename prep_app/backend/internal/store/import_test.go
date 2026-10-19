package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImportKnowledgePreservesProgress(t *testing.T) {
	s := openTest(t)
	path := filepath.Join(t.TempDir(), "knowledge.json")
	if err := os.WriteFile(path, []byte(`{
		"prep_tracks": [{"code":"dumps","title":"Секции компаний","why":"w","how_to_train":"h","time_min":25}],
		"prep_protocol": [{"id":1,"body":"Одна карточка в день."}],
		"prep_items": [{
			"id": 1, "track": "dumps", "company": "Ozon", "section": "Go runtime",
			"grade": "Middle+", "difficulty": 3,
			"question": "Чем горутина отличается от потока ОС?",
			"theory": "GMP", "answer": "Стек около 2 КБ",
			"source": "guide", "source_url": "https://example.test",
			"slot": "even", "status": "new", "notes": ""
		}],
		"questions": [{
			"id": 7, "topic": "Go", "category": "Memory & Runtime",
			"question": "Как устроен slice?", "answer": "header из ptr, len, cap",
			"grade": "Junior", "difficulty": 2, "source": "notes",
			"is_answered": 0, "user_notes": "", "created_at": "2026-08-18 11:46:10"
		}],
		"hard_concepts": [{
			"id": 1, "topic": "MCP", "title": "stdio JSON-RPC",
			"simple_explanation": "stdout только для протокола",
			"under_the_hood": "JSON-RPC 2.0 по строке",
			"pitfalls": "fmt.Println ломает парсер",
			"created_at": "2026-10-07 12:25:59"
		}],
		"interview_tracker": [{
			"id": 1, "topic": "MCP", "question": "Зачем MCP?",
			"times_asked": 1, "user_understood": 1, "last_asked": "2026-10-07 11:35:28"
		}]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := s.ImportKnowledge(path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("imported %d", n)
	}
	item, err := s.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Track != "dumps" || item.ItemType != TypeFlashcard || item.Prompt == "" {
		t.Fatalf("%+v", item)
	}
	if item.AnswerSkeleton != "Стек около 2 КБ" {
		t.Fatalf("skeleton %q", item.AnswerSkeleton)
	}
	q, err := s.Get(questionIDBase + 7)
	if err != nil || q.Track != "library" {
		t.Fatalf("question %+v %v", q, err)
	}
	c, err := s.Get(conceptIDBase + 1)
	if err != nil || c.Track != "concept" || c.Company != "MCP" {
		t.Fatalf("concept %+v %v", c, err)
	}
	if _, err := s.RecordAnswer(AnswerInput{QuestionID: 1, Passed: true, UserAnswer: "чётко"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportKnowledge(path); err != nil {
		t.Fatal(err)
	}
	again, err := s.Get(1)
	if err != nil || again.TimesAsked != 1 || again.Status != "done" {
		t.Fatalf("progress lost %+v %v", again, err)
	}
	bar, err := s.Bar()
	if err != nil {
		t.Fatal(err)
	}
	if bar.Total != 1 || bar.Done != 1 || bar.Blocks[0].ID != "dumps" {
		t.Fatalf("bar %+v", bar)
	}
	if bar.Studied != 1 || bar.Protocol == "" || len(bar.Tracker) != 1 {
		t.Fatalf("meta %+v", bar)
	}
	hits, _, err := s.Search("slice header", 5)
	if err != nil || len(hits) == 0 || hits[0].ID != questionIDBase+7 {
		t.Fatalf("search %+v %v", hits, err)
	}
	var concepts int
	if err := s.Hard.QueryRow(`SELECT COUNT(*) FROM concepts`).Scan(&concepts); err != nil || concepts != 1 {
		t.Fatalf("concepts %d %v", concepts, err)
	}
}

func TestTodayTrack(t *testing.T) {
	sunday := time.Date(2026, 10, 18, 9, 0, 0, 0, time.Local)
	if sunday.Weekday() != time.Sunday {
		t.Fatal(sunday.Weekday())
	}
	if todayTrack(sunday) != "system_design" {
		t.Fatal(todayTrack(sunday))
	}
	even := time.Date(2026, 10, 20, 9, 0, 0, 0, time.Local)
	if todayTrack(even) != "dumps" {
		t.Fatal(todayTrack(even))
	}
	odd := time.Date(2026, 10, 19, 9, 0, 0, 0, time.Local)
	if todayTrack(odd) != "patterns" {
		t.Fatal(todayTrack(odd))
	}
}
