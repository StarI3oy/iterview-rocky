package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	TypeQuiz      = "quiz"
	TypeFlashcard = "flashcard"
	TypeCode      = "code"

	BlockGo     = "go"
	BlockDB     = "db_kafka"
	BlockAlgo   = "algo"
	BlockDesign = "system_design"
)

// Item is a training card. ReferenceCode is kept server-side and omitted from JSON.
type Item struct {
	ID             int64    `json:"id"`
	ItemType       string   `json:"item_type"`
	Block          string   `json:"block"`
	BlockTitle     string   `json:"block_title"`
	Company        string   `json:"company,omitempty"`
	Title          string   `json:"title"`
	Prompt         string   `json:"prompt"`
	Options        []string `json:"options,omitempty"`
	CorrectIndex   *int     `json:"correct_index,omitempty"`
	Explanation    string   `json:"explanation,omitempty"`
	AnswerSkeleton string   `json:"answer_skeleton,omitempty"`
	Checklist      []string `json:"checklist,omitempty"`
	StarterCode    string   `json:"starter_code,omitempty"`
	ReferenceCode  string   `json:"-"`
	ReviewCriteria string   `json:"review_criteria,omitempty"`
	Language       string   `json:"language,omitempty"`
	TimesAsked     int      `json:"times_asked"`
	TimesPassed    int      `json:"times_passed"`
	LastResult     *int     `json:"last_result,omitempty"`
	Closed         bool     `json:"closed"`
	RequiresCode   bool     `json:"requires_code"`
}

type BlockStat struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Percent int    `json:"percent"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
}

type Bar struct {
	OverallPercent int         `json:"overall_percent"`
	Done           int         `json:"done"`
	Total          int         `json:"total"`
	Empty          bool        `json:"empty"`
	Blocks         []BlockStat `json:"blocks"`
}

type HardConcept struct {
	ID           int64  `json:"id"`
	QuestionID   *int64 `json:"question_id,omitempty"`
	Block        string `json:"block"`
	BlockTitle   string `json:"block_title"`
	Title        string `json:"title"`
	Prompt       string `json:"prompt"`
	Note         string `json:"note,omitempty"`
	FailCount    int    `json:"fail_count"`
	LastFailedAt string `json:"last_failed_at"`
	Resolved     bool   `json:"resolved"`
}

type Store struct {
	Items *sql.DB
	Hard  *sql.DB
}

func Open(interviewPath, hardPath string) (*Store, error) {
	items, err := openSQLite(interviewPath)
	if err != nil {
		return nil, fmt.Errorf("interview db: %w", err)
	}
	hard, err := openSQLite(hardPath)
	if err != nil {
		_ = items.Close()
		return nil, fmt.Errorf("hard concepts db: %w", err)
	}
	s := &Store{Items: items, Hard: hard}
	if err := s.Migrate(); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func openSQLite(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func (s *Store) Close() error {
	var err error
	if s.Items != nil {
		err = s.Items.Close()
	}
	if s.Hard != nil {
		if e := s.Hard.Close(); err == nil {
			err = e
		}
	}
	return err
}

func (s *Store) Migrate() error {
	if _, err := s.Items.Exec(`CREATE TABLE IF NOT EXISTS prep_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		item_type TEXT NOT NULL DEFAULT 'flashcard',
		block TEXT NOT NULL DEFAULT 'go',
		company TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		prompt TEXT NOT NULL DEFAULT '',
		options TEXT NOT NULL DEFAULT '',
		correct_index INTEGER,
		explanation TEXT NOT NULL DEFAULT '',
		answer_skeleton TEXT NOT NULL DEFAULT '',
		checklist TEXT NOT NULL DEFAULT '',
		starter_code TEXT NOT NULL DEFAULT '',
		reference_code TEXT NOT NULL DEFAULT '',
		review_criteria TEXT NOT NULL DEFAULT '',
		language TEXT NOT NULL DEFAULT '',
		times_asked INTEGER NOT NULL DEFAULT 0,
		times_passed INTEGER NOT NULL DEFAULT 0,
		last_result INTEGER,
		closed INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return fmt.Errorf("create prep_items: %w", err)
	}
	// Nullable adds so rows loaded with an older shape stay NULL until backfill.
	adds := []struct{ name, def string }{
		{"item_type", "TEXT"},
		{"block", "TEXT"},
		{"company", "TEXT"},
		{"title", "TEXT"},
		{"prompt", "TEXT"},
		{"options", "TEXT"},
		{"correct_index", "INTEGER"},
		{"explanation", "TEXT"},
		{"answer_skeleton", "TEXT"},
		{"checklist", "TEXT"},
		{"starter_code", "TEXT"},
		{"reference_code", "TEXT"},
		{"review_criteria", "TEXT"},
		{"language", "TEXT"},
		{"times_asked", "INTEGER NOT NULL DEFAULT 0"},
		{"times_passed", "INTEGER NOT NULL DEFAULT 0"},
		{"last_result", "INTEGER"},
		{"closed", "INTEGER NOT NULL DEFAULT 0"},
		{"created_at", "TEXT"},
		{"updated_at", "TEXT"},
	}
	if err := ensureColumns(s.Items, "prep_items", adds); err != nil {
		return err
	}
	if err := backfillItems(s.Items); err != nil {
		return err
	}
	if _, err := s.Items.Exec(`CREATE TABLE IF NOT EXISTS web_queue (
		question_id INTEGER PRIMARY KEY,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return fmt.Errorf("create web_queue: %w", err)
	}
	if _, err := s.Hard.Exec(`CREATE TABLE IF NOT EXISTS hard_concepts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		question_id INTEGER,
		block TEXT,
		title TEXT,
		prompt TEXT,
		note TEXT,
		fail_count INTEGER NOT NULL DEFAULT 1,
		last_failed_at TEXT NOT NULL DEFAULT (datetime('now')),
		resolved INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("create hard_concepts: %w", err)
	}
	hardAdds := []struct{ name, def string }{
		{"question_id", "INTEGER"},
		{"block", "TEXT"},
		{"title", "TEXT"},
		{"prompt", "TEXT"},
		{"note", "TEXT"},
		{"fail_count", "INTEGER NOT NULL DEFAULT 1"},
		{"last_failed_at", "TEXT"},
		{"resolved", "INTEGER NOT NULL DEFAULT 0"},
	}
	return ensureColumns(s.Hard, "hard_concepts", hardAdds)
}

func ensureColumns(db *sql.DB, table string, cols []struct{ name, def string }) error {
	have, err := tableColumns(db, table)
	if err != nil {
		return err
	}
	for _, c := range cols {
		if have[c.name] {
			continue
		}
		q := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, c.name, c.def)
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
	}
	return nil
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

func backfillItems(db *sql.DB) error {
	cols, err := tableColumns(db, "prep_items")
	if err != nil {
		return err
	}
	if cols["question"] {
		if _, err := db.Exec(`UPDATE prep_items SET prompt = question
			WHERE (prompt IS NULL OR prompt = '') AND question IS NOT NULL AND question != ''`); err != nil {
			return err
		}
	}
	if cols["body"] {
		if _, err := db.Exec(`UPDATE prep_items SET prompt = body
			WHERE (prompt IS NULL OR prompt = '') AND body IS NOT NULL AND body != ''`); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`UPDATE prep_items SET prompt = COALESCE(title, '')
		WHERE prompt IS NULL OR prompt = ''`); err != nil {
		return err
	}
	if cols["topic"] {
		if _, err := db.Exec(`UPDATE prep_items SET block = topic
			WHERE (block IS NULL OR block = '') AND topic IS NOT NULL AND topic != ''`); err != nil {
			return err
		}
	}
	if cols["category"] {
		if _, err := db.Exec(`UPDATE prep_items SET block = category
			WHERE (block IS NULL OR block = '') AND category IS NOT NULL AND category != ''`); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`UPDATE prep_items SET block = 'go' WHERE block IS NULL OR block = ''`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE prep_items SET company = '' WHERE company IS NULL`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE prep_items SET title = '' WHERE title IS NULL`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE prep_items SET
		options = COALESCE(options, ''),
		explanation = COALESCE(explanation, ''),
		answer_skeleton = COALESCE(answer_skeleton, ''),
		checklist = COALESCE(checklist, ''),
		starter_code = COALESCE(starter_code, ''),
		reference_code = COALESCE(reference_code, ''),
		review_criteria = COALESCE(review_criteria, ''),
		language = COALESCE(language, ''),
		times_asked = COALESCE(times_asked, 0),
		times_passed = COALESCE(times_passed, 0),
		closed = COALESCE(closed, 0)`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE prep_items SET item_type = 'code'
		WHERE (item_type IS NULL OR item_type = '')
		AND starter_code IS NOT NULL AND starter_code != ''`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE prep_items SET item_type = 'quiz'
		WHERE (item_type IS NULL OR item_type = '')
		AND options IS NOT NULL AND options != '' AND options != '[]'`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE prep_items SET item_type = 'flashcard'
		WHERE item_type IS NULL OR item_type = ''`); err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE prep_items SET created_at = datetime('now') WHERE created_at IS NULL OR created_at = ''`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE prep_items SET updated_at = datetime('now') WHERE updated_at IS NULL OR updated_at = ''`)
	return err
}

func BlockTitle(block string) string {
	switch NormalizeBlock(block) {
	case BlockGo:
		return "Go/Concurrency"
	case BlockDB:
		return "DB/Kafka"
	case BlockAlgo:
		return "Алгоритмы"
	case BlockDesign:
		return "System Design"
	default:
		if strings.TrimSpace(block) == "" {
			return "Go/Concurrency"
		}
		return block
	}
}

func NormalizeBlock(block string) string {
	v := strings.ToLower(strings.TrimSpace(block))
	v = strings.ReplaceAll(v, " ", "_")
	v = strings.ReplaceAll(v, "-", "_")
	switch v {
	case "go", "golang", "concurrency", "go_concurrency", "go/concurrency":
		return BlockGo
	case "db", "postgres", "postgresql", "kafka", "db_kafka", "db/kafka", "database", "databases":
		return BlockDB
	case "algo", "algorithms", "leetcode", "algorithm":
		return BlockAlgo
	case "system_design", "systemdesign", "sysdesign", "sd", "design":
		return BlockDesign
	default:
		return v
	}
}

func (s *Store) CountItems() (int, error) {
	var n int
	err := s.Items.QueryRow(`SELECT COUNT(*) FROM prep_items`).Scan(&n)
	return n, err
}

func (s *Store) Bar() (Bar, error) {
	rows, err := s.Items.Query(`SELECT COALESCE(block,''), COALESCE(item_type,''), last_result, COALESCE(closed,0) FROM prep_items`)
	if err != nil {
		return Bar{}, err
	}
	defer rows.Close()
	type acc struct{ done, total int }
	by := map[string]*acc{
		BlockGo:     {},
		BlockDB:     {},
		BlockAlgo:   {},
		BlockDesign: {},
	}
	var done, total int
	for rows.Next() {
		var block, itemType string
		var last sql.NullInt64
		var closed int
		if err := rows.Scan(&block, &itemType, &last, &closed); err != nil {
			return Bar{}, err
		}
		total++
		mastered := false
		if itemType == TypeCode {
			mastered = closed == 1
		} else if last.Valid && last.Int64 == 1 {
			mastered = true
		}
		if mastered {
			done++
		}
		key := NormalizeBlock(block)
		bucket, ok := by[key]
		if !ok {
			continue
		}
		bucket.total++
		if mastered {
			bucket.done++
		}
	}
	if err := rows.Err(); err != nil {
		return Bar{}, err
	}
	order := []string{BlockGo, BlockDB, BlockAlgo, BlockDesign}
	bar := Bar{
		OverallPercent: percent(done, total),
		Done:           done,
		Total:          total,
		Empty:          total == 0,
		Blocks:         make([]BlockStat, 0, 4),
	}
	for _, id := range order {
		a := by[id]
		bar.Blocks = append(bar.Blocks, BlockStat{
			ID:      id,
			Title:   BlockTitle(id),
			Percent: percent(a.done, a.total),
			Done:    a.done,
			Total:   a.total,
		})
	}
	return bar, nil
}

func percent(done, total int) int {
	if total == 0 {
		return 0
	}
	return int(float64(done) * 100 / float64(total))
}

type FeedFilter struct {
	Mode   string // mobile | web
	Kind   string // sprint | code | hard | or explicit item type
	SkipID int64
}

func (s *Store) Next(f FeedFilter) (*Item, error) {
	if f.Kind == "hard" {
		return s.nextHard(f.SkipID)
	}
	where := []string{"1=1"}
	args := []any{}
	switch {
	case f.Mode == "web" || f.Kind == TypeCode || f.Kind == "code":
		where = append(where, "item_type = ?", "closed = 0")
		args = append(args, TypeCode)
	default:
		where = append(where, "item_type IN (?, ?)")
		args = append(args, TypeQuiz, TypeFlashcard)
	}
	if f.SkipID > 0 {
		where = append(where, "id != ?")
		args = append(args, f.SkipID)
	}
	q := `SELECT id FROM prep_items WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY
			CASE WHEN last_result = 0 THEN 0 WHEN last_result IS NULL THEN 1 ELSE 2 END,
			times_asked ASC,
			id ASC
		LIMIT 1`
	var id int64
	err := s.Items.QueryRow(q, args...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.Get(id)
}

func (s *Store) nextHard(skipID int64) (*Item, error) {
	rows, err := s.Hard.Query(`SELECT question_id FROM hard_concepts
		WHERE resolved = 0 AND question_id IS NOT NULL ORDER BY fail_count DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id sql.NullInt64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if !id.Valid || id.Int64 == skipID {
			continue
		}
		item, err := s.Get(id.Int64)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		return item, nil
	}
	return nil, rows.Err()
}

func (s *Store) Get(id int64) (*Item, error) {
	row := s.Items.QueryRow(`SELECT id,
		COALESCE(item_type,''), COALESCE(block,''), COALESCE(company,''), COALESCE(title,''), COALESCE(prompt,''),
		COALESCE(options,''), correct_index, COALESCE(explanation,''), COALESCE(answer_skeleton,''),
		COALESCE(checklist,''), COALESCE(starter_code,''), COALESCE(reference_code,''),
		COALESCE(review_criteria,''), COALESCE(language,''), COALESCE(times_asked,0), COALESCE(times_passed,0),
		last_result, COALESCE(closed,0)
		FROM prep_items WHERE id = ?`, id)
	item, err := scanItem(row)
	if err != nil {
		return nil, err
	}
	return item, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanItem(row scanner) (*Item, error) {
	var it Item
	var options, checklist string
	var correct, last sql.NullInt64
	var closed int
	err := row.Scan(
		&it.ID, &it.ItemType, &it.Block, &it.Company, &it.Title, &it.Prompt, &options,
		&correct, &it.Explanation, &it.AnswerSkeleton, &checklist, &it.StarterCode, &it.ReferenceCode,
		&it.ReviewCriteria, &it.Language, &it.TimesAsked, &it.TimesPassed, &last, &closed,
	)
	if err != nil {
		return nil, err
	}
	it.ItemType = strings.TrimSpace(it.ItemType)
	if it.ItemType == "" {
		it.ItemType = TypeFlashcard
	}
	it.BlockTitle = BlockTitle(it.Block)
	it.Block = NormalizeBlock(it.Block)
	it.Options = decodeList(options)
	it.Checklist = decodeList(checklist)
	if correct.Valid {
		v := int(correct.Int64)
		it.CorrectIndex = &v
	}
	if last.Valid {
		v := int(last.Int64)
		it.LastResult = &v
	}
	it.Closed = closed == 1
	it.RequiresCode = it.ItemType == TypeCode
	if it.Language == "" && it.ItemType == TypeCode {
		it.Language = "go"
	}
	return &it, nil
}

func decodeList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err == nil {
		return list
	}
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.TrimPrefix(p, "-"))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

type AnswerInput struct {
	QuestionID    int64
	Passed        bool
	UserAnswer    string
	SelectedIndex *int
}

type AnswerResult struct {
	Passed       bool   `json:"passed"`
	CorrectIndex *int   `json:"correct_index,omitempty"`
	Explanation  string `json:"explanation,omitempty"`
	TimesAsked   int    `json:"times_asked"`
}

func (s *Store) RecordAnswer(in AnswerInput) (AnswerResult, error) {
	item, err := s.Get(in.QuestionID)
	if err != nil {
		return AnswerResult{}, err
	}
	passed := in.Passed
	if item.ItemType == TypeQuiz && in.SelectedIndex != nil && item.CorrectIndex != nil {
		passed = *in.SelectedIndex == *item.CorrectIndex
	}
	times := item.TimesAsked + 1
	passedN := item.TimesPassed
	last := 0
	if passed {
		passedN++
		last = 1
	}
	_, err = s.Items.Exec(`UPDATE prep_items
		SET times_asked = ?, times_passed = ?, last_result = ?, updated_at = ?
		WHERE id = ?`, times, passedN, last, time.Now().UTC().Format(time.RFC3339), item.ID)
	if err != nil {
		return AnswerResult{}, err
	}
	if passed {
		if err := s.ResolveHard(item.ID); err != nil {
			return AnswerResult{}, err
		}
	} else {
		if err := s.UpsertHard(*item, in.UserAnswer); err != nil {
			return AnswerResult{}, err
		}
	}
	return AnswerResult{
		Passed:       passed,
		CorrectIndex: item.CorrectIndex,
		Explanation:  item.Explanation,
		TimesAsked:   times,
	}, nil
}

func (s *Store) UpsertHard(item Item, note string) error {
	res, err := s.Hard.Exec(`UPDATE hard_concepts
		SET block = ?, title = ?, prompt = ?, note = ?, fail_count = fail_count + 1,
			last_failed_at = datetime('now'), resolved = 0
		WHERE question_id = ?`,
		item.Block, item.Title, item.Prompt, note, item.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err = s.Hard.Exec(`INSERT INTO hard_concepts
		(question_id, block, title, prompt, note, fail_count, last_failed_at, resolved)
		VALUES (?, ?, ?, ?, ?, 1, datetime('now'), 0)`,
		item.ID, item.Block, item.Title, item.Prompt, note)
	return err
}

func (s *Store) ResolveHard(questionID int64) error {
	_, err := s.Hard.Exec(`UPDATE hard_concepts SET resolved = 1 WHERE question_id = ?`, questionID)
	return err
}

func (s *Store) MarkReview(item Item, passed bool, note string) error {
	times := item.TimesAsked + 1
	if passed {
		_, err := s.Items.Exec(`UPDATE prep_items
			SET times_asked = ?, times_passed = times_passed + 1, last_result = 1, closed = 1, updated_at = ?
			WHERE id = ?`, times, time.Now().UTC().Format(time.RFC3339), item.ID)
		if err != nil {
			return err
		}
		if _, err := s.Items.Exec(`DELETE FROM web_queue WHERE question_id = ?`, item.ID); err != nil {
			return err
		}
		return s.ResolveHard(item.ID)
	}
	_, err := s.Items.Exec(`UPDATE prep_items
		SET times_asked = ?, last_result = 0, updated_at = ?
		WHERE id = ?`, times, time.Now().UTC().Format(time.RFC3339), item.ID)
	if err != nil {
		return err
	}
	return s.UpsertHard(item, note)
}

func (s *Store) Enqueue(questionID int64) error {
	item, err := s.Get(questionID)
	if err != nil {
		return err
	}
	if item.ItemType != TypeCode {
		return fmt.Errorf("в очередь Web попадают только кодовые задачи")
	}
	_, err = s.Items.Exec(`INSERT INTO web_queue (question_id, created_at) VALUES (?, datetime('now'))
		ON CONFLICT(question_id) DO UPDATE SET created_at = datetime('now')`, questionID)
	return err
}

func (s *Store) Queue() ([]Item, error) {
	rows, err := s.Items.Query(`SELECT question_id FROM web_queue ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(ids))
	for _, id := range ids {
		item, err := s.Get(id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		out = append(out, *item)
	}
	return out, nil
}

type SearchHit struct {
	Kind       string `json:"kind"` // item | hard
	ID         int64  `json:"id"`
	QuestionID *int64 `json:"question_id,omitempty"`
	ItemType   string `json:"item_type,omitempty"`
	Block      string `json:"block"`
	BlockTitle string `json:"block_title"`
	Company    string `json:"company,omitempty"`
	Title      string `json:"title"`
	Snippet    string `json:"snippet"`
	Score      int    `json:"score"`
}

func (s *Store) Search(query string, limit int) (items []SearchHit, hard []SearchHit, err error) {
	if limit <= 0 {
		limit = 8
	}
	words := queryWords(query)
	rows, err := s.Items.Query(`SELECT id, COALESCE(item_type,''), COALESCE(block,''), COALESCE(company,''),
		COALESCE(title,''), COALESCE(prompt,''), COALESCE(explanation,''), COALESCE(answer_skeleton,'')
		FROM prep_items`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var itemHits []SearchHit
	for rows.Next() {
		var id int64
		var itemType, block, company, title, prompt, explanation, skeleton string
		if err := rows.Scan(&id, &itemType, &block, &company, &title, &prompt, &explanation, &skeleton); err != nil {
			return nil, nil, err
		}
		hay := strings.Join([]string{title, prompt, explanation, skeleton, company, block}, "\n")
		score := scoreHay(hay, words)
		if score == 0 {
			continue
		}
		itemHits = append(itemHits, SearchHit{
			Kind:       "item",
			ID:         id,
			ItemType:   itemType,
			Block:      NormalizeBlock(block),
			BlockTitle: BlockTitle(block),
			Company:    company,
			Title:      firstNonEmpty(title, clip(prompt, 80)),
			Snippet:    clip(prompt, 220),
			Score:      score,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	hrows, err := s.Hard.Query(`SELECT id, question_id, COALESCE(block,''), COALESCE(title,''), COALESCE(prompt,''), COALESCE(note,'')
		FROM hard_concepts WHERE COALESCE(resolved,0) = 0`)
	if err != nil {
		return nil, nil, err
	}
	defer hrows.Close()
	var hardHits []SearchHit
	for hrows.Next() {
		var id int64
		var qid sql.NullInt64
		var block, title, prompt, note string
		if err := hrows.Scan(&id, &qid, &block, &title, &prompt, &note); err != nil {
			return nil, nil, err
		}
		hay := strings.Join([]string{title, prompt, note, block}, "\n")
		score := scoreHay(hay, words)
		if score == 0 {
			continue
		}
		hit := SearchHit{
			Kind:       "hard",
			ID:         id,
			Block:      NormalizeBlock(block),
			BlockTitle: BlockTitle(block),
			Title:      firstNonEmpty(title, clip(prompt, 80)),
			Snippet:    clip(firstNonEmpty(note, prompt), 220),
			Score:      score,
		}
		if qid.Valid {
			v := qid.Int64
			hit.QuestionID = &v
		}
		hardHits = append(hardHits, hit)
	}
	if err := hrows.Err(); err != nil {
		return nil, nil, err
	}
	sortHits(itemHits)
	sortHits(hardHits)
	return take(itemHits, limit), take(hardHits, limit), nil
}

func (s *Store) ListHard() ([]HardConcept, error) {
	rows, err := s.Hard.Query(`SELECT id, question_id, COALESCE(block,''), COALESCE(title,''), COALESCE(prompt,''),
		COALESCE(note,''), COALESCE(fail_count,1), COALESCE(last_failed_at,''), COALESCE(resolved,0)
		FROM hard_concepts WHERE COALESCE(resolved,0) = 0
		ORDER BY fail_count DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HardConcept
	for rows.Next() {
		var h HardConcept
		var qid sql.NullInt64
		var resolved int
		if err := rows.Scan(&h.ID, &qid, &h.Block, &h.Title, &h.Prompt, &h.Note, &h.FailCount, &h.LastFailedAt, &resolved); err != nil {
			return nil, err
		}
		if qid.Valid {
			v := qid.Int64
			h.QuestionID = &v
		}
		h.BlockTitle = BlockTitle(h.Block)
		h.Block = NormalizeBlock(h.Block)
		h.Resolved = resolved == 1
		out = append(out, h)
	}
	if out == nil {
		out = []HardConcept{}
	}
	return out, rows.Err()
}

func queryWords(q string) []string {
	fields := strings.Fields(strings.ToLower(q))
	var out []string
	for _, f := range fields {
		f = strings.Trim(f, `"'.,:;!?()[]«»`)
		if len([]rune(f)) < 2 {
			continue
		}
		out = append(out, f)
	}
	return out
}

func scoreHay(hay string, words []string) int {
	if len(words) == 0 {
		return 0
	}
	h := strings.ToLower(hay)
	score := 0
	for _, w := range words {
		if strings.Contains(h, w) {
			score++
		}
	}
	return score
}

func sortHits(hits []SearchHit) {
	for i := 1; i < len(hits); i++ {
		j := i
		for j > 0 && (hits[j].Score > hits[j-1].Score || (hits[j].Score == hits[j-1].Score && hits[j].ID < hits[j-1].ID)) {
			hits[j], hits[j-1] = hits[j-1], hits[j]
			j--
		}
	}
}

func take(hits []SearchHit, n int) []SearchHit {
	if hits == nil {
		return []SearchHit{}
	}
	if len(hits) > n {
		return hits[:n]
	}
	return hits
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
