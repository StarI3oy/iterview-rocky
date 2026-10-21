package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	questionIDBase int64 = 10000
	conceptIDBase  int64 = 20000
)

type knowledgeFile struct {
	Questions        []questionSrc `json:"questions"`
	InterviewTracker []trackerSrc  `json:"interview_tracker"`
	PrepTracks       []trackSrc    `json:"prep_tracks"`
	PrepItems        []prepSrc     `json:"prep_items"`
	PrepProtocol     []protocolSrc `json:"prep_protocol"`
	HardConcepts     []conceptSrc  `json:"hard_concepts"`
}

type questionSrc struct {
	ID         int64  `json:"id"`
	Topic      string `json:"topic"`
	Category   string `json:"category"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
	Grade      string `json:"grade"`
	Difficulty int    `json:"difficulty"`
	Source     string `json:"source"`
	IsAnswered int    `json:"is_answered"`
	UserNotes  string `json:"user_notes"`
	CreatedAt  string `json:"created_at"`
}

type trackerSrc struct {
	ID             int64  `json:"id"`
	Topic          string `json:"topic"`
	Question       string `json:"question"`
	TimesAsked     int    `json:"times_asked"`
	UserUnderstood int    `json:"user_understood"`
	LastAsked      string `json:"last_asked"`
}

type trackSrc struct {
	Code       string `json:"code"`
	Title      string `json:"title"`
	Why        string `json:"why"`
	HowToTrain string `json:"how_to_train"`
	TimeMin    int    `json:"time_min"`
}

type prepSrc struct {
	ID         int64  `json:"id"`
	Track      string `json:"track"`
	Company    string `json:"company"`
	Section    string `json:"section"`
	Grade      string `json:"grade"`
	Difficulty int    `json:"difficulty"`
	Question   string `json:"question"`
	Theory     string `json:"theory"`
	Answer     string `json:"answer"`
	Source     string `json:"source"`
	SourceURL  string `json:"source_url"`
	Slot       string `json:"slot"`
	Status     string `json:"status"`
	Notes      string `json:"notes"`
}

type protocolSrc struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

type conceptSrc struct {
	ID                int64  `json:"id"`
	Topic             string `json:"topic"`
	Title             string `json:"title"`
	SimpleExplanation string `json:"simple_explanation"`
	UnderTheHood      string `json:"under_the_hood"`
	Pitfalls          string `json:"pitfalls"`
	CreatedAt         string `json:"created_at"`
}

type cardRow struct {
	ID             int64
	SourceKey      string
	ItemType       string
	Block          string
	Company        string
	Title          string
	Prompt         string
	Explanation    string
	AnswerSkeleton string
	Checklist      string
	StarterCode    string
	ReferenceCode  string
	ReviewCriteria string
	Language       string
	Track          string
	Section        string
	Grade          string
	Difficulty     int
	Question       string
	Theory         string
	Answer         string
	Source         string
	SourceURL      string
	Slot           string
	Status         string
	Notes          string
}

// ImportFromJSON imports a knowledge export or a bare array of questions.
func (s *Store) ImportFromJSON(data []byte) (int, error) {
	doc, err := decodeKnowledge(data)
	if err != nil {
		return 0, err
	}
	return s.importKnowledgeFile(doc)
}

// ImportKnowledge upserts the exported curriculum. Progress columns stay as they are.
func (s *Store) ImportKnowledge(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return s.ImportFromJSON(raw)
}

func (s *Store) importKnowledgeFile(doc knowledgeFile) (int, error) {
	tx, err := s.Items.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	n := 0
	for _, t := range doc.PrepTracks {
		if _, err := tx.Exec(`INSERT INTO prep_tracks (code, title, why, how_to_train, time_min)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET
				title = excluded.title, why = excluded.why,
				how_to_train = excluded.how_to_train, time_min = excluded.time_min`,
			t.Code, t.Title, t.Why, t.HowToTrain, t.TimeMin); err != nil {
			return 0, err
		}
	}
	for _, p := range doc.PrepProtocol {
		if p.ID == 0 {
			p.ID = 1
		}
		if _, err := tx.Exec(`INSERT INTO prep_protocol (id, body) VALUES (?, ?)
			ON CONFLICT(id) DO UPDATE SET body = excluded.body`, p.ID, p.Body); err != nil {
			return 0, err
		}
	}
	for _, it := range doc.PrepItems {
		if err := upsertCard(tx, projectPrep(it)); err != nil {
			return 0, err
		}
		n++
	}
	for _, q := range doc.Questions {
		if _, err := tx.Exec(`INSERT INTO questions
			(id, topic, category, question, answer, grade, difficulty, source, is_answered, user_notes, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				topic = excluded.topic, category = excluded.category, question = excluded.question,
				answer = excluded.answer, grade = excluded.grade, difficulty = excluded.difficulty,
				source = excluded.source, is_answered = excluded.is_answered,
				user_notes = excluded.user_notes, created_at = excluded.created_at`,
			q.ID, q.Topic, q.Category, q.Question, q.Answer, q.Grade, q.Difficulty, q.Source, q.IsAnswered, q.UserNotes, q.CreatedAt); err != nil {
			return 0, err
		}
		if err := upsertCard(tx, projectQuestion(q)); err != nil {
			return 0, err
		}
		n++
	}
	for _, t := range doc.InterviewTracker {
		if _, err := tx.Exec(`INSERT INTO interview_tracker
			(id, topic, question, times_asked, user_understood, last_asked)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				topic = excluded.topic,
				question = excluded.question,
				times_asked = MAX(interview_tracker.times_asked, excluded.times_asked),
				user_understood = excluded.user_understood,
				last_asked = CASE WHEN interview_tracker.times_asked > excluded.times_asked
					THEN interview_tracker.last_asked ELSE excluded.last_asked END`,
			t.ID, t.Topic, t.Question, t.TimesAsked, t.UserUnderstood, t.LastAsked); err != nil {
			return 0, err
		}
	}
	for _, c := range doc.HardConcepts {
		if err := upsertCard(tx, projectConcept(c)); err != nil {
			return 0, err
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	htx, err := s.Hard.Begin()
	if err != nil {
		return n, err
	}
	defer func() { _ = htx.Rollback() }()
	for _, c := range doc.HardConcepts {
		if _, err := htx.Exec(`INSERT INTO concepts
			(id, topic, title, simple_explanation, under_the_hood, pitfalls, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				topic = excluded.topic, title = excluded.title,
				simple_explanation = excluded.simple_explanation,
				under_the_hood = excluded.under_the_hood,
				pitfalls = excluded.pitfalls, created_at = excluded.created_at`,
			c.ID, c.Topic, c.Title, c.SimpleExplanation, c.UnderTheHood, c.Pitfalls, c.CreatedAt); err != nil {
			return n, err
		}
	}
	if err := htx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

var cardColumns = []string{
	"item_type", "block", "company", "title", "prompt", "explanation", "answer_skeleton", "checklist",
	"starter_code", "reference_code", "review_criteria", "language", "track", "section", "grade", "difficulty",
	"question", "theory", "answer", "source", "source_url", "slot", "notes",
}

func (row cardRow) values() []any {
	return []any{
		row.ItemType, row.Block, row.Company, row.Title, row.Prompt, row.Explanation, row.AnswerSkeleton, row.Checklist,
		row.StarterCode, row.ReferenceCode, row.ReviewCriteria, row.Language, row.Track, row.Section, row.Grade, row.Difficulty,
		row.Question, row.Theory, row.Answer, row.Source, row.SourceURL, row.Slot, row.Notes,
	}
}

// upsertCard matches cards by source_key so foreign rows that share an id are never overwritten.
// A new card keeps its preferred id when it is free, otherwise SQLite assigns one.
// Progress columns (times_asked, last_result, closed, status) are only set on insert.
func upsertCard(tx *sql.Tx, row cardRow) error {
	var id int64
	err := tx.QueryRow(`SELECT id FROM prep_items WHERE source_key = ?`, row.SourceKey).Scan(&id)
	if err == nil {
		sets := make([]string, len(cardColumns))
		for i, c := range cardColumns {
			sets[i] = c + " = ?"
		}
		_, err = tx.Exec(`UPDATE prep_items SET `+strings.Join(sets, ", ")+` WHERE id = ?`, append(row.values(), id)...)
		return err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var taken int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM prep_items WHERE id = ?`, row.ID).Scan(&taken); err != nil {
		return err
	}
	var preferred any
	if taken == 0 {
		preferred = row.ID
	}
	cols := append([]string{"id", "source_key", "status"}, cardColumns...)
	args := append([]any{preferred, row.SourceKey, row.Status}, row.values()...)
	marks := strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ")
	_, err = tx.Exec(`INSERT INTO prep_items (`+strings.Join(cols, ", ")+`) VALUES (`+marks+`)`, args...)
	return err
}

func projectPrep(it prepSrc) cardRow {
	company := strings.TrimSpace(it.Company)
	if strings.EqualFold(company, "any") {
		company = ""
	}
	status := it.Status
	if status == "" {
		status = "new"
	}
	row := cardRow{
		ID:          it.ID,
		SourceKey:   fmt.Sprintf("prep:%d", it.ID),
		ItemType:    TypeFlashcard,
		Block:       blockForPrep(it.Track, it.Section),
		Company:     company,
		Title:       it.Section,
		Prompt:      it.Question,
		Explanation: it.Theory,
		Track:       it.Track,
		Section:     it.Section,
		Grade:       it.Grade,
		Difficulty:  it.Difficulty,
		Question:    it.Question,
		Theory:      it.Theory,
		Answer:      it.Answer,
		Source:      it.Source,
		SourceURL:   it.SourceURL,
		Slot:        it.Slot,
		Status:      status,
		Notes:       it.Notes,
	}
	if it.Track == "patterns" {
		row.ItemType = TypeCode
		row.Language = "go"
		row.StarterCode = "package main\n\n// " + oneLine(it.Question) + "\n// Напиши решение на Go.\n"
		row.ReferenceCode = strings.TrimSpace(it.Answer + "\n\n" + it.Theory)
		row.Explanation = ""
		row.AnswerSkeleton = ""
		return row
	}
	row.AnswerSkeleton = it.Answer
	if strings.TrimSpace(it.Theory) != "" {
		buf, _ := json.Marshal([]string{it.Theory})
		row.Checklist = string(buf)
	}
	return row
}

func projectQuestion(q questionSrc) cardRow {
	var checklist string
	if strings.TrimSpace(q.UserNotes) != "" {
		buf, _ := json.Marshal([]string{q.UserNotes})
		checklist = string(buf)
	}
	title := q.Category
	if title == "" {
		title = q.Topic
	}
	return cardRow{
		ID:             questionIDBase + q.ID,
		SourceKey:      fmt.Sprintf("q:%d", q.ID),
		ItemType:       TypeFlashcard,
		Block:          blockForTopic(q.Topic),
		Company:        q.Topic,
		Title:          title,
		Prompt:         q.Question,
		AnswerSkeleton: q.Answer,
		Checklist:      checklist,
		Track:          "library",
		Section:        q.Category,
		Grade:          q.Grade,
		Difficulty:     q.Difficulty,
		Question:       q.Question,
		Answer:         q.Answer,
		Source:         q.Source,
		Notes:          q.UserNotes,
		Status:         "new",
	}
}

func projectConcept(c conceptSrc) cardRow {
	skeleton := strings.TrimSpace(c.SimpleExplanation + "\n\n" + c.UnderTheHood)
	var checklist string
	if strings.TrimSpace(c.Pitfalls) != "" {
		buf, _ := json.Marshal([]string{c.Pitfalls})
		checklist = string(buf)
	}
	return cardRow{
		ID:             conceptIDBase + c.ID,
		SourceKey:      fmt.Sprintf("c:%d", c.ID),
		ItemType:       TypeFlashcard,
		Block:          "concept",
		Company:        c.Topic,
		Title:          c.Title,
		Prompt:         c.Title,
		AnswerSkeleton: skeleton,
		Checklist:      checklist,
		Track:          "concept",
		Section:        c.Topic,
		Question:       c.Title,
		Theory:         c.UnderTheHood,
		Answer:         skeleton,
		Notes:          c.Pitfalls,
		Status:         "new",
	}
}

func blockForPrep(track, section string) string {
	switch track {
	case "patterns":
		return BlockAlgo
	case "system_design":
		return BlockDesign
	case "legend":
		return "legend"
	}
	return blockForTopic(section)
}

func blockForTopic(topic string) string {
	s := strings.ToLower(topic)
	switch {
	case strings.Contains(s, "kafka"), strings.Contains(s, "sql"), strings.Contains(s, "postgres"), strings.Contains(s, "db"), strings.Contains(s, "money"), strings.Contains(s, "payment"):
		return BlockDB
	case strings.Contains(s, "flutter"), strings.Contains(s, "dart"), strings.Contains(s, "network"), strings.Contains(s, "http"), strings.Contains(s, "tcp"):
		return "networking"
	case strings.Contains(s, "algo"), strings.Contains(s, "leetcode"):
		return BlockAlgo
	case strings.Contains(s, "design"):
		return BlockDesign
	default:
		return BlockGo
	}
}

func oneLine(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}
