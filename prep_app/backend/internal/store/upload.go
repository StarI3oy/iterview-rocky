package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"
)

// ImportIssue is one place where an uploaded record diverges from the expected format.
type ImportIssue struct {
	Kind    string `json:"kind"`
	ID      int64  `json:"id,omitempty"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// ImportStats is what a user upload contributed. Cards is the number of
// training cards written (questions, prep items, and concepts).
type ImportStats struct {
	Cards     int
	Questions int
	PrepItems int
	Concepts  int
	Tracker   int
	Tracks    int
	Protocol  int
	Issues    []ImportIssue
}

func (s ImportStats) empty() bool {
	return s.Questions == 0 && s.PrepItems == 0 && s.Concepts == 0 && s.Tracker == 0 && s.Tracks == 0 && s.Protocol == 0
}

// ImportUpload reads a knowledge JSON export, a SQLite dump, or a text dump
// and upserts it. Training progress on existing cards is left in place.
func (s *Store) ImportUpload(data []byte) (ImportStats, error) {
	doc, issues, err := parseUpload(data)
	if err != nil {
		return ImportStats{}, err
	}
	doc, more := validateDoc(doc)
	issues = append(issues, more...)
	stats := ImportStats{
		Questions: len(doc.Questions),
		PrepItems: len(doc.PrepItems),
		Concepts:  len(doc.HardConcepts),
		Tracker:   len(doc.InterviewTracker),
		Tracks:    len(doc.PrepTracks),
		Protocol:  len(doc.PrepProtocol),
		Issues:    issues,
	}
	if stats.Issues == nil {
		stats.Issues = []ImportIssue{}
	}
	if stats.empty() {
		if len(stats.Issues) > 0 {
			return stats, nil
		}
		return ImportStats{}, fmt.Errorf("в файле нет вопросов или карточек")
	}
	n, err := s.importKnowledgeFile(doc)
	if err != nil {
		return ImportStats{}, err
	}
	stats.Cards = n
	return stats, nil
}

func parseUpload(data []byte) (knowledgeFile, []ImportIssue, error) {
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
	if len(data) == 0 {
		return knowledgeFile{}, nil, fmt.Errorf("файл пуст")
	}
	switch data[0] {
	case '{', '[':
		return decodeChecked(data)
	}
	text := string(data)
	if looksLikeSQL(text) {
		doc, err := parseSQLDump(text)
		return doc, nil, err
	}
	doc, err := parseTextDump(text)
	return doc, nil, err
}

func decodeChecked(data []byte) (knowledgeFile, []ImportIssue, error) {
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
	if len(data) == 0 {
		return knowledgeFile{}, nil, fmt.Errorf("файл пуст")
	}
	if data[0] == '[' {
		qs, issues, err := decodeItems[questionSrc](data, "question")
		if err != nil {
			return knowledgeFile{}, nil, fmt.Errorf("json: %w", err)
		}
		return knowledgeFile{Questions: qs}, issues, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return knowledgeFile{}, nil, fmt.Errorf("json: %w", err)
	}
	var doc knowledgeFile
	var issues []ImportIssue
	known := map[string]bool{
		"questions": true, "interview_tracker": true, "prep_tracks": true,
		"prep_items": true, "prep_protocol": true, "hard_concepts": true,
	}
	for key := range raw {
		if !known[key] {
			issues = append(issues, ImportIssue{Kind: "file", Field: key, Message: "неизвестное поле"})
		}
	}
	var err error
	if doc.Questions, err = takeItems(&issues, raw, "questions", "question", decodeItems[questionSrc]); err != nil {
		return knowledgeFile{}, nil, err
	}
	if doc.InterviewTracker, err = takeItems(&issues, raw, "interview_tracker", "tracker", decodeItems[trackerSrc]); err != nil {
		return knowledgeFile{}, nil, err
	}
	if doc.PrepTracks, err = takeItems(&issues, raw, "prep_tracks", "track", decodeItems[trackSrc]); err != nil {
		return knowledgeFile{}, nil, err
	}
	if doc.PrepItems, err = takeItems(&issues, raw, "prep_items", "prep_item", decodeItems[prepSrc]); err != nil {
		return knowledgeFile{}, nil, err
	}
	if doc.PrepProtocol, err = takeItems(&issues, raw, "prep_protocol", "protocol", decodeItems[protocolSrc]); err != nil {
		return knowledgeFile{}, nil, err
	}
	if doc.HardConcepts, err = takeItems(&issues, raw, "hard_concepts", "concept", decodeItems[conceptSrc]); err != nil {
		return knowledgeFile{}, nil, err
	}
	return doc, issues, nil
}

func takeItems[T any](issues *[]ImportIssue, raw map[string]json.RawMessage, key, kind string, decode func(json.RawMessage, string) ([]T, []ImportIssue, error)) ([]T, error) {
	payload, ok := raw[key]
	if !ok {
		return nil, nil
	}
	items, more, err := decode(payload, kind)
	if err != nil {
		*issues = append(*issues, ImportIssue{Kind: "file", Field: key, Message: "ожидался массив"})
		return nil, nil
	}
	*issues = append(*issues, more...)
	return items, nil
}

func decodeItems[T any](raw json.RawMessage, kind string) ([]T, []ImportIssue, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, nil, err
	}
	out := make([]T, 0, len(arr))
	var issues []ImportIssue
	for i, item := range arr {
		var v T
		dec := json.NewDecoder(bytes.NewReader(item))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&v); err != nil {
			issues = append(issues, ImportIssue{
				Kind:    kind,
				Message: fmt.Sprintf("запись %d: формат не совпал (%s)", i+1, jsonHint(err)),
			})
			continue
		}
		out = append(out, v)
	}
	return out, issues, nil
}

func jsonHint(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, "json: "); i >= 0 {
		msg = msg[i+len("json: "):]
	}
	if len(msg) > 140 {
		msg = msg[:140]
	}
	return msg
}

func decodeKnowledge(data []byte) (knowledgeFile, error) {
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
	if len(data) == 0 {
		return knowledgeFile{}, fmt.Errorf("файл пуст")
	}
	if data[0] == '[' {
		var qs []questionSrc
		if err := json.Unmarshal(data, &qs); err != nil {
			return knowledgeFile{}, fmt.Errorf("json: %w", err)
		}
		return knowledgeFile{Questions: qs}, nil
	}
	var doc knowledgeFile
	if err := json.Unmarshal(data, &doc); err != nil {
		return knowledgeFile{}, fmt.Errorf("json: %w", err)
	}
	return doc, nil
}

func looksLikeSQL(s string) bool {
	u := strings.ToUpper(s)
	return strings.Contains(u, "INSERT INTO") || strings.Contains(u, "CREATE TABLE")
}

var uploadDBSeq atomic.Int64

func parseSQLDump(script string) (knowledgeFile, error) {
	dsn := fmt.Sprintf("file:upload%d?mode=memory&cache=shared", uploadDBSeq.Add(1))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return knowledgeFile{}, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	for _, stmt := range splitSQL(script) {
		if !sqlAllowed(stmt) {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return knowledgeFile{}, fmt.Errorf("sql: %w", err)
		}
	}

	var doc knowledgeFile
	if doc.Questions, err = loadQuestions(db); err != nil {
		return knowledgeFile{}, err
	}
	if doc.InterviewTracker, err = loadTracker(db); err != nil {
		return knowledgeFile{}, err
	}
	if doc.PrepTracks, err = loadTracks(db); err != nil {
		return knowledgeFile{}, err
	}
	if doc.PrepItems, err = loadPrepItems(db); err != nil {
		return knowledgeFile{}, err
	}
	if doc.PrepProtocol, err = loadProtocol(db); err != nil {
		return knowledgeFile{}, err
	}
	if doc.HardConcepts, err = loadConcepts(db); err != nil {
		return knowledgeFile{}, err
	}
	return doc, nil
}

func sqlAllowed(stmt string) bool {
	s := strings.ToUpper(strings.TrimSpace(stmt))
	switch {
	case strings.HasPrefix(s, "PRAGMA"),
		strings.HasPrefix(s, "BEGIN"),
		strings.HasPrefix(s, "COMMIT"),
		strings.HasPrefix(s, "ROLLBACK"),
		strings.HasPrefix(s, "END"),
		strings.HasPrefix(s, "CREATE TABLE"),
		strings.HasPrefix(s, "INSERT"),
		strings.HasPrefix(s, "DELETE FROM SQLITE_SEQUENCE"):
		return true
	default:
		return false
	}
}

func splitSQL(script string) []string {
	var out []string
	var b strings.Builder
	inStr := false
	for i := 0; i < len(script); i++ {
		c := script[i]
		if inStr {
			b.WriteByte(c)
			if c == '\'' {
				if i+1 < len(script) && script[i+1] == '\'' {
					b.WriteByte('\'')
					i++
					continue
				}
				inStr = false
			}
			continue
		}
		if c == '\'' {
			inStr = true
			b.WriteByte(c)
			continue
		}
		if c == '-' && i+1 < len(script) && script[i+1] == '-' {
			for i < len(script) && script[i] != '\n' {
				i++
			}
			continue
		}
		if c == ';' {
			if stmt := strings.TrimSpace(b.String()); stmt != "" {
				out = append(out, stmt)
			}
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	if stmt := strings.TrimSpace(b.String()); stmt != "" {
		out = append(out, stmt)
	}
	return out
}

func hasTable(db *sql.DB, name string) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
	return n > 0, err
}

func loadQuestions(db *sql.DB) ([]questionSrc, error) {
	ok, err := hasTable(db, "questions")
	if err != nil || !ok {
		return nil, err
	}
	rows, err := db.Query(`SELECT id,
		CAST(ifnull(topic, '') AS TEXT), CAST(ifnull(category, '') AS TEXT),
		CAST(ifnull(question, '') AS TEXT), CAST(ifnull(answer, '') AS TEXT),
		CAST(ifnull(grade, '') AS TEXT), CAST(ifnull(difficulty, 0) AS INTEGER),
		CAST(ifnull(source, '') AS TEXT), CAST(ifnull(is_answered, 0) AS INTEGER),
		CAST(ifnull(user_notes, '') AS TEXT), CAST(ifnull(created_at, '') AS TEXT)
		FROM questions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []questionSrc
	for rows.Next() {
		var q questionSrc
		if err := rows.Scan(&q.ID, &q.Topic, &q.Category, &q.Question, &q.Answer, &q.Grade, &q.Difficulty, &q.Source, &q.IsAnswered, &q.UserNotes, &q.CreatedAt); err != nil {
			return nil, err
		}
		if strings.TrimSpace(q.Question) == "" {
			continue
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

func loadTracker(db *sql.DB) ([]trackerSrc, error) {
	ok, err := hasTable(db, "interview_tracker")
	if err != nil || !ok {
		return nil, err
	}
	rows, err := db.Query(`SELECT id, CAST(ifnull(topic, '') AS TEXT), CAST(ifnull(question, '') AS TEXT),
		CAST(ifnull(times_asked, 0) AS INTEGER), CAST(ifnull(user_understood, 0) AS INTEGER),
		CAST(ifnull(last_asked, '') AS TEXT)
		FROM interview_tracker ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []trackerSrc
	for rows.Next() {
		var t trackerSrc
		if err := rows.Scan(&t.ID, &t.Topic, &t.Question, &t.TimesAsked, &t.UserUnderstood, &t.LastAsked); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func loadTracks(db *sql.DB) ([]trackSrc, error) {
	ok, err := hasTable(db, "prep_tracks")
	if err != nil || !ok {
		return nil, err
	}
	rows, err := db.Query(`SELECT ifnull(code, ''), ifnull(title, ''), ifnull(why, ''),
		ifnull(how_to_train, ''), ifnull(time_min, 0) FROM prep_tracks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []trackSrc
	for rows.Next() {
		var t trackSrc
		if err := rows.Scan(&t.Code, &t.Title, &t.Why, &t.HowToTrain, &t.TimeMin); err != nil {
			return nil, err
		}
		if t.Code == "" {
			continue
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func loadPrepItems(db *sql.DB) ([]prepSrc, error) {
	ok, err := hasTable(db, "prep_items")
	if err != nil || !ok {
		return nil, err
	}
	cols, err := tableColumns(db, "prep_items")
	if err != nil {
		return nil, err
	}
	// The app's own prep_items table is the card projection, not the export shape.
	if cols["item_type"] || !cols["track"] || !cols["question"] {
		return nil, nil
	}
	rows, err := db.Query(`SELECT id, CAST(ifnull(track, '') AS TEXT), CAST(ifnull(company, '') AS TEXT),
		CAST(ifnull(section, '') AS TEXT), CAST(ifnull(grade, '') AS TEXT),
		CAST(ifnull(difficulty, 0) AS INTEGER), CAST(ifnull(question, '') AS TEXT),
		CAST(ifnull(theory, '') AS TEXT), CAST(ifnull(answer, '') AS TEXT),
		CAST(ifnull(source, '') AS TEXT), CAST(ifnull(source_url, '') AS TEXT),
		CAST(ifnull(slot, '') AS TEXT), CAST(ifnull(status, '') AS TEXT), CAST(ifnull(notes, '') AS TEXT)
		FROM prep_items ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []prepSrc
	for rows.Next() {
		var it prepSrc
		if err := rows.Scan(&it.ID, &it.Track, &it.Company, &it.Section, &it.Grade, &it.Difficulty, &it.Question, &it.Theory, &it.Answer, &it.Source, &it.SourceURL, &it.Slot, &it.Status, &it.Notes); err != nil {
			return nil, err
		}
		if strings.TrimSpace(it.Question) == "" {
			continue
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func loadProtocol(db *sql.DB) ([]protocolSrc, error) {
	ok, err := hasTable(db, "prep_protocol")
	if err != nil || !ok {
		return nil, err
	}
	rows, err := db.Query(`SELECT id, ifnull(body, '') FROM prep_protocol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []protocolSrc
	for rows.Next() {
		var p protocolSrc
		if err := rows.Scan(&p.ID, &p.Body); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func loadConcepts(db *sql.DB) ([]conceptSrc, error) {
	for _, name := range []string{"hard_concepts", "concepts"} {
		ok, err := hasTable(db, name)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		cols, err := tableColumns(db, name)
		if err != nil {
			return nil, err
		}
		if !cols["title"] || !cols["simple_explanation"] {
			continue
		}
		rows, err := db.Query(`SELECT id, CAST(ifnull(topic, '') AS TEXT), CAST(ifnull(title, '') AS TEXT),
			CAST(ifnull(simple_explanation, '') AS TEXT), CAST(ifnull(under_the_hood, '') AS TEXT),
			CAST(ifnull(pitfalls, '') AS TEXT), CAST(ifnull(created_at, '') AS TEXT)
			FROM ` + quoteIdent(name) + ` ORDER BY id`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []conceptSrc
		for rows.Next() {
			var c conceptSrc
			if err := rows.Scan(&c.ID, &c.Topic, &c.Title, &c.SimpleExplanation, &c.UnderTheHood, &c.Pitfalls, &c.CreatedAt); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, c)
		}
		return out, rows.Err()
	}
	return nil, nil
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

var (
	textIDRe    = regexp.MustCompile(`^--- \[ID #(\d+)\]`)
	textGradeRe = regexp.MustCompile(`^Грейд:\s*(.+?)\s*\|\s*Сложность:\s*(\d+)`)
)

func parseTextDump(text string) (knowledgeFile, error) {
	var doc knowledgeFile
	var hint string
	var cur *questionSrc
	var section string
	var buf strings.Builder

	flushSection := func() {
		if cur == nil || section == "" {
			buf.Reset()
			return
		}
		val := strings.TrimSpace(buf.String())
		switch section {
		case "q":
			cur.Question = val
		case "a":
			cur.Answer = val
		case "n":
			cur.UserNotes = val
		}
		buf.Reset()
	}
	flushCard := func() {
		flushSection()
		section = ""
		if cur == nil {
			return
		}
		if strings.TrimSpace(cur.Question) != "" {
			doc.Questions = append(doc.Questions, *cur)
		}
		cur = nil
	}

	for _, line := range strings.Split(text, "\n") {
		raw := strings.TrimRight(line, "\r")
		trim := strings.TrimSpace(raw)
		if strings.HasPrefix(trim, "ТОПИК:") {
			flushCard()
			hint = strings.TrimSpace(strings.TrimPrefix(trim, "ТОПИК:"))
			continue
		}
		if m := textIDRe.FindStringSubmatch(trim); m != nil {
			flushCard()
			id, _ := strconv.ParseInt(m[1], 10, 64)
			cur = &questionSrc{ID: id}
			continue
		}
		if strings.HasPrefix(trim, "####") || strings.HasPrefix(trim, "ТАБЛИЦА:") {
			flushCard()
			hint = ""
			continue
		}
		if cur == nil {
			if strings.HasPrefix(trim, "Запись:") {
				rec, err := parseTrackerRecord(strings.TrimSpace(strings.TrimPrefix(trim, "Запись:")))
				if err != nil {
					return knowledgeFile{}, err
				}
				doc.InterviewTracker = append(doc.InterviewTracker, rec)
			}
			continue
		}
		if strings.HasPrefix(trim, "Тема:") {
			topic, category := splitTopic(strings.TrimSpace(strings.TrimPrefix(trim, "Тема:")), hint)
			cur.Topic, cur.Category = topic, category
			continue
		}
		if m := textGradeRe.FindStringSubmatch(trim); m != nil {
			cur.Grade = strings.TrimSpace(m[1])
			d, _ := strconv.Atoi(m[2])
			cur.Difficulty = d
			continue
		}
		if strings.HasPrefix(trim, "Статус:") {
			status := strings.TrimSpace(strings.TrimPrefix(trim, "Статус:"))
			if strings.Contains(status, "Не отвечен") {
				cur.IsAnswered = 0
			} else if strings.Contains(status, "Отвечен") || strings.Contains(status, "Пройден") {
				cur.IsAnswered = 1
			}
			continue
		}
		if strings.HasPrefix(trim, "Источник:") {
			cur.Source = strings.TrimSpace(strings.TrimPrefix(trim, "Источник:"))
			continue
		}
		if strings.HasPrefix(trim, "Создан:") {
			cur.CreatedAt = strings.TrimSpace(strings.TrimPrefix(trim, "Создан:"))
			continue
		}
		switch trim {
		case "[ВОПРОС]:":
			flushSection()
			section = "q"
			continue
		case "[ЭТАЛОННЫЙ ОТВЕТ]:":
			flushSection()
			section = "a"
			continue
		case "[ЗАМЕТКИ ПОЛЬЗОВАТЕЛЯ]:":
			flushSection()
			section = "n"
			continue
		}
		if section == "" {
			continue
		}
		if trim != "" && strings.Trim(trim, "=#") == "" {
			continue
		}
		buf.WriteString(raw)
		buf.WriteByte('\n')
	}
	flushCard()
	return doc, nil
}

func splitTopic(theme, hint string) (string, string) {
	theme = strings.TrimSpace(theme)
	parts := strings.Split(theme, " / ")
	if len(parts) < 2 {
		return theme, ""
	}
	hint = strings.TrimSpace(hint)
	if hint != "" {
		for i := 1; i < len(parts); i++ {
			prefix := strings.Join(parts[:i], " / ")
			if strings.EqualFold(prefix, hint) {
				return prefix, strings.Join(parts[i:], " / ")
			}
		}
	}
	return parts[0], strings.Join(parts[1:], " / ")
}

func parseTrackerRecord(s string) (trackerSrc, error) {
	p := pyParser{s: strings.TrimSpace(s)}
	m, err := p.parseDict()
	if err != nil {
		return trackerSrc{}, fmt.Errorf("строка трекера: %w", err)
	}
	q, _ := m["question"].(string)
	if strings.TrimSpace(q) == "" {
		return trackerSrc{}, fmt.Errorf("строка трекера без вопроса")
	}
	topic, _ := m["topic"].(string)
	return trackerSrc{
		ID:             pyInt(m["id"]),
		Topic:          topic,
		Question:       q,
		TimesAsked:     int(pyInt(m["times_asked"])),
		UserUnderstood: int(pyInt(m["user_understood"])),
		LastAsked:      pyString(m["last_asked"]),
	}, nil
}

func pyInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	default:
		return 0
	}
}

func pyString(v any) string {
	s, _ := v.(string)
	return s
}

type pyParser struct {
	s string
	i int
}

func (p *pyParser) parseDict() (map[string]any, error) {
	p.skip()
	if p.i >= len(p.s) || p.s[p.i] != '{' {
		return nil, fmt.Errorf("ожидался словарь")
	}
	p.i++
	m := map[string]any{}
	for {
		p.skip()
		if p.i < len(p.s) && p.s[p.i] == '}' {
			p.i++
			return m, nil
		}
		key, err := p.parseString()
		if err != nil {
			return nil, err
		}
		p.skip()
		if p.i >= len(p.s) || p.s[p.i] != ':' {
			return nil, fmt.Errorf("ожидалось двоеточие")
		}
		p.i++
		p.skip()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		m[key] = val
		p.skip()
		if p.i < len(p.s) && p.s[p.i] == ',' {
			p.i++
			continue
		}
		if p.i < len(p.s) && p.s[p.i] == '}' {
			p.i++
			return m, nil
		}
		return nil, fmt.Errorf("ожидалась запятая")
	}
}

func (p *pyParser) parseValue() (any, error) {
	if p.i >= len(p.s) {
		return nil, fmt.Errorf("неожиданный конец")
	}
	if p.s[p.i] == '\'' || p.s[p.i] == '"' {
		return p.parseString()
	}
	rest := p.s[p.i:]
	switch {
	case strings.HasPrefix(rest, "True"), strings.HasPrefix(rest, "true"):
		p.i += 4
		return int64(1), nil
	case strings.HasPrefix(rest, "False"):
		p.i += 5
		return int64(0), nil
	case strings.HasPrefix(rest, "false"):
		p.i += 5
		return int64(0), nil
	}
	start := p.i
	if p.s[p.i] == '-' {
		p.i++
	}
	for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
	}
	if p.i == start || (p.s[start] == '-' && p.i == start+1) {
		return nil, fmt.Errorf("непонятное значение")
	}
	n, err := strconv.ParseInt(p.s[start:p.i], 10, 64)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (p *pyParser) parseString() (string, error) {
	if p.i >= len(p.s) {
		return "", fmt.Errorf("незакрытая строка")
	}
	quote := p.s[p.i]
	if quote != '\'' && quote != '"' {
		return "", fmt.Errorf("ожидалась строка")
	}
	p.i++
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '\\' && p.i+1 < len(p.s) {
			n := p.s[p.i+1]
			switch n {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(n)
			}
			p.i += 2
			continue
		}
		if c == quote {
			p.i++
			return b.String(), nil
		}
		b.WriteByte(c)
		p.i++
	}
	return "", fmt.Errorf("незакрытая строка")
}

func (p *pyParser) skip() {
	for p.i < len(p.s) && unicode.IsSpace(rune(p.s[p.i])) {
		p.i++
	}
}
