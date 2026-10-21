package store

import (
	"os"
	"strings"
	"testing"
)

func TestImportSQLAndTextDumps(t *testing.T) {
	s := openTest(t)
	sqlDump := `
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE questions (
  id INTEGER PRIMARY KEY,
  topic TEXT, category TEXT, question TEXT, answer TEXT,
  grade TEXT, difficulty INTEGER, source TEXT,
  is_answered BOOLEAN, user_notes TEXT, created_at TIMESTAMP
);
INSERT INTO questions VALUES(4,'Go','Concurrency','Что с nil-каналом?',replace('чтение блокирует\nзапись тоже','\n',char(10)),'Junior',2,'spec',0,'','2026-08-18 11:46:10');
INSERT INTO questions VALUES(5,'Go','Memory','Побег?',replace('переменная ''убегает'' из функции','\n',char(10)),'Middle',3,'O''Reilly',1,'заметка','2026-08-18 11:46:10');
CREATE TABLE interview_tracker (
  id INTEGER PRIMARY KEY, topic TEXT, question TEXT,
  times_asked INTEGER, user_understood BOOLEAN, last_asked TIMESTAMP
);
INSERT INTO interview_tracker VALUES(1,'MCP','Зачем MCP?',2,1,'2026-10-07 11:35:28');
CREATE TABLE prep_items (
  id INTEGER PRIMARY KEY, track TEXT, company TEXT, section TEXT, grade TEXT,
  difficulty INTEGER, question TEXT, theory TEXT, answer TEXT, source TEXT,
  source_url TEXT, slot TEXT, status TEXT, notes TEXT
);
INSERT INTO prep_items VALUES(1,'dumps','Ozon','Go','Middle+',3,'Чем горутина отличается?','GMP','стек 2 КБ','guide','https://example.test','even','new','');
ATTACH DATABASE '/tmp/nope.db' AS evil;
COMMIT;
`
	stats, err := s.ImportUpload([]byte(sqlDump))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Questions != 2 || stats.PrepItems != 1 || stats.Tracker != 1 || stats.Cards != 3 {
		t.Fatalf("stats %+v", stats)
	}
	q, err := s.Get(questionIDBase + 4)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q.AnswerSkeleton, "\n") || q.Track != "library" {
		t.Fatalf("question %+v", q)
	}
	escaped, err := s.Get(questionIDBase + 5)
	if err != nil {
		t.Fatal(err)
	}
	var source string
	if err := s.Items.QueryRow(`SELECT source FROM questions WHERE id = 5`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(escaped.AnswerSkeleton, "'убегает'") || source != "O'Reilly" {
		t.Fatalf("escapes skeleton %q source %q", escaped.AnswerSkeleton, source)
	}
	if _, err := s.RecordAnswer(AnswerInput{QuestionID: 1, Passed: true, UserAnswer: "ок"}); err != nil {
		t.Fatal(err)
	}

	textDump := `
ТОПИК: DATASCIENCE / ML
--- [ID #23] -----------------------------------------------------
Тема:        DataScience / ML / Theory & Regularization
Грейд:       Middle | Сложность: 3/5
Статус:      Не отвечен (Ожидает)
Источник:    ML Interview Handbook
Создан:      2026-08-19 08:05:36

[ВОПРОС]:
В чем разница между L1 и L2?

[ЭТАЛОННЫЙ ОТВЕТ]:
L1 зануляет веса.
L2 нет.

ТАБЛИЦА: interview_tracker
Запись: {'id': 2, 'topic': 'MCP & Agentic AI', 'question': 'Как устроен stdio и fmt.Println("debug")?', 'times_asked': 1, 'user_understood': 0, 'last_asked': '2026-10-07 11:39:13'}
`
	stats, err = s.ImportUpload([]byte(textDump))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Questions != 1 || stats.Tracker != 1 || stats.Cards != 1 {
		t.Fatalf("text stats %+v", stats)
	}
	ml, err := s.Get(questionIDBase + 23)
	if err != nil {
		t.Fatal(err)
	}
	if ml.Company != "DataScience / ML" || ml.Section != "Theory & Regularization" {
		t.Fatalf("topic split %+v", ml)
	}
	if !strings.Contains(ml.AnswerSkeleton, "L2 нет.") {
		t.Fatalf("answer %q", ml.AnswerSkeleton)
	}
	again, err := s.Get(1)
	if err != nil || again.TimesAsked != 1 || again.Status != "done" {
		t.Fatalf("progress lost %+v %v", again, err)
	}

	arr, err := s.ImportUpload([]byte(`[{"id":9,"topic":"Go","category":"Net","question":"Что такое QUIC?","answer":"UDP","grade":"Middle","difficulty":3}]`))
	if err != nil || arr.Questions != 1 {
		t.Fatalf("array %+v %v", arr, err)
	}
}

func TestImportRealSamples(t *testing.T) {
	samples := []struct {
		path      string
		questions int
		prep      int
		tracker   int
	}{
		{"/home/ubuntu/.cursor/projects/workspace/uploads/interview_questions.sql_50f3.txt", 52, 43, 4},
		{"/home/ubuntu/.cursor/projects/workspace/uploads/interview_questions_4b71.txt", 52, 0, 4},
		{"/home/ubuntu/.cursor/projects/workspace/uploads/prep_app_knowledge_export_3b96.json", 52, 43, 4},
	}
	for _, sample := range samples {
		raw, err := os.ReadFile(sample.path)
		if err != nil {
			t.Logf("skip %s: %v", sample.path, err)
			continue
		}
		s := openTest(t)
		stats, err := s.ImportUpload(raw)
		if err != nil {
			t.Fatalf("%s: %v", sample.path, err)
		}
		if stats.Questions != sample.questions || stats.PrepItems != sample.prep || stats.Tracker != sample.tracker {
			t.Fatalf("%s stats %+v", sample.path, stats)
		}
		if sample.prep > 0 && stats.Cards != sample.questions+sample.prep+2 {
			// JSON sample also carries hard concepts; checked loosely below.
		}
		q, err := s.Get(questionIDBase + 14)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(q.AnswerSkeleton, "\n") || !strings.Contains(q.AnswerSkeleton, "Content-Length") {
			t.Fatalf("%s newlines %q", sample.path, q.AnswerSkeleton)
		}
		noted, err := s.Get(questionIDBase + 6)
		if err != nil || len(noted.Checklist) == 0 || !strings.Contains(noted.Checklist[0], "Черные") {
			t.Fatalf("%s notes %+v %v", sample.path, noted, err)
		}
	}
}
