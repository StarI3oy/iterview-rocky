package store

import (
	"strings"
	"time"
)

var allowedGrades = map[string]bool{
	"Junior": true, "Middle": true, "Middle+": true,
	"Senior": true, "Lead": true, "Easy": true, "Medium": true,
}

var allowedTracks = map[string]bool{
	"dumps": true, "patterns": true, "system_design": true, "legend": true,
}

var allowedSlots = map[string]bool{
	"": true, "even": true, "odd": true, "weekly": true,
}

var allowedStatuses = map[string]bool{
	"": true, "new": true, "done": true, "learning": true,
}

func validateDoc(doc knowledgeFile) (knowledgeFile, []ImportIssue) {
	var issues []ImportIssue
	doc.Questions, issues = filterQuestions(doc.Questions, issues)
	doc.PrepItems, issues = filterPrep(doc.PrepItems, issues)
	doc.HardConcepts, issues = filterConcepts(doc.HardConcepts, issues)
	doc.InterviewTracker, issues = filterTracker(doc.InterviewTracker, issues)
	doc.PrepTracks, issues = filterTracks(doc.PrepTracks, issues)
	doc.PrepProtocol, issues = filterProtocol(doc.PrepProtocol, issues)
	return doc, issues
}

func filterQuestions(items []questionSrc, issues []ImportIssue) ([]questionSrc, []ImportIssue) {
	seen := map[int64]bool{}
	out := make([]questionSrc, 0, len(items))
	for _, q := range items {
		bad := questionIssues(q)
		if q.ID > 0 && seen[q.ID] {
			bad = append(bad, issue("question", q.ID, "id", "такой id уже есть в файле"))
		}
		if len(bad) > 0 {
			issues = append(issues, bad...)
			continue
		}
		seen[q.ID] = true
		out = append(out, q)
	}
	return out, issues
}

func questionIssues(q questionSrc) []ImportIssue {
	var bad []ImportIssue
	if q.ID <= 0 {
		bad = append(bad, issue("question", q.ID, "id", "нужен положительный id"))
	}
	if strings.TrimSpace(q.Topic) == "" {
		bad = append(bad, issue("question", q.ID, "topic", "пустая тема"))
	}
	if strings.TrimSpace(q.Category) == "" {
		bad = append(bad, issue("question", q.ID, "category", "пустая категория"))
	}
	if strings.TrimSpace(q.Question) == "" {
		bad = append(bad, issue("question", q.ID, "question", "пустой вопрос"))
	}
	if strings.TrimSpace(q.Answer) == "" {
		bad = append(bad, issue("question", q.ID, "answer", "пустой ответ"))
	}
	if !allowedGrades[q.Grade] {
		bad = append(bad, issue("question", q.ID, "grade", "грейд не из списка Junior, Middle, Middle+, Senior, Lead, Easy, Medium"))
	}
	if q.Difficulty < 1 || q.Difficulty > 5 {
		bad = append(bad, issue("question", q.ID, "difficulty", "сложность должна быть от 1 до 5"))
	}
	if q.IsAnswered != 0 && q.IsAnswered != 1 {
		bad = append(bad, issue("question", q.ID, "is_answered", "должно быть 0 или 1"))
	}
	if !validTimestamp(q.CreatedAt) {
		bad = append(bad, issue("question", q.ID, "created_at", "дата должна быть YYYY-MM-DD HH:MM:SS"))
	}
	return bad
}

func filterPrep(items []prepSrc, issues []ImportIssue) ([]prepSrc, []ImportIssue) {
	seen := map[int64]bool{}
	out := make([]prepSrc, 0, len(items))
	for _, it := range items {
		var bad []ImportIssue
		if it.ID <= 0 {
			bad = append(bad, issue("prep_item", it.ID, "id", "нужен положительный id"))
		}
		if !allowedTracks[it.Track] {
			bad = append(bad, issue("prep_item", it.ID, "track", "трек должен быть dumps, patterns, system_design или legend"))
		}
		if strings.TrimSpace(it.Question) == "" {
			bad = append(bad, issue("prep_item", it.ID, "question", "пустой вопрос"))
		}
		if strings.TrimSpace(it.Theory) == "" {
			bad = append(bad, issue("prep_item", it.ID, "theory", "пустая теория"))
		}
		if strings.TrimSpace(it.Answer) == "" {
			bad = append(bad, issue("prep_item", it.ID, "answer", "пустой ответ"))
		}
		if it.Grade != "" && !allowedGrades[it.Grade] {
			bad = append(bad, issue("prep_item", it.ID, "grade", "грейд не из списка Junior, Middle, Middle+, Senior, Lead, Easy, Medium"))
		}
		if it.Difficulty != 0 && (it.Difficulty < 1 || it.Difficulty > 5) {
			bad = append(bad, issue("prep_item", it.ID, "difficulty", "сложность должна быть от 1 до 5"))
		}
		if !allowedSlots[it.Slot] {
			bad = append(bad, issue("prep_item", it.ID, "slot", "слот должен быть even, odd или weekly"))
		}
		if !allowedStatuses[it.Status] {
			bad = append(bad, issue("prep_item", it.ID, "status", "статус должен быть new, learning или done"))
		}
		if it.ID > 0 && seen[it.ID] {
			bad = append(bad, issue("prep_item", it.ID, "id", "такой id уже есть в файле"))
		}
		if len(bad) > 0 {
			issues = append(issues, bad...)
			continue
		}
		seen[it.ID] = true
		out = append(out, it)
	}
	return out, issues
}

func filterConcepts(items []conceptSrc, issues []ImportIssue) ([]conceptSrc, []ImportIssue) {
	seen := map[int64]bool{}
	out := make([]conceptSrc, 0, len(items))
	for _, c := range items {
		var bad []ImportIssue
		if c.ID <= 0 {
			bad = append(bad, issue("concept", c.ID, "id", "нужен положительный id"))
		}
		if strings.TrimSpace(c.Topic) == "" {
			bad = append(bad, issue("concept", c.ID, "topic", "пустая тема"))
		}
		if strings.TrimSpace(c.Title) == "" {
			bad = append(bad, issue("concept", c.ID, "title", "пустой заголовок"))
		}
		if strings.TrimSpace(c.SimpleExplanation) == "" {
			bad = append(bad, issue("concept", c.ID, "simple_explanation", "пустое простое объяснение"))
		}
		if !validTimestamp(c.CreatedAt) {
			bad = append(bad, issue("concept", c.ID, "created_at", "дата должна быть YYYY-MM-DD HH:MM:SS"))
		}
		if c.ID > 0 && seen[c.ID] {
			bad = append(bad, issue("concept", c.ID, "id", "такой id уже есть в файле"))
		}
		if len(bad) > 0 {
			issues = append(issues, bad...)
			continue
		}
		seen[c.ID] = true
		out = append(out, c)
	}
	return out, issues
}

func filterTracker(items []trackerSrc, issues []ImportIssue) ([]trackerSrc, []ImportIssue) {
	seen := map[int64]bool{}
	out := make([]trackerSrc, 0, len(items))
	for _, t := range items {
		var bad []ImportIssue
		if t.ID <= 0 {
			bad = append(bad, issue("tracker", t.ID, "id", "нужен положительный id"))
		}
		if strings.TrimSpace(t.Topic) == "" {
			bad = append(bad, issue("tracker", t.ID, "topic", "пустая тема"))
		}
		if strings.TrimSpace(t.Question) == "" {
			bad = append(bad, issue("tracker", t.ID, "question", "пустой вопрос"))
		}
		if t.TimesAsked < 0 {
			bad = append(bad, issue("tracker", t.ID, "times_asked", "не может быть отрицательным"))
		}
		if t.UserUnderstood != 0 && t.UserUnderstood != 1 {
			bad = append(bad, issue("tracker", t.ID, "user_understood", "должно быть 0 или 1"))
		}
		if !validTimestamp(t.LastAsked) {
			bad = append(bad, issue("tracker", t.ID, "last_asked", "дата должна быть YYYY-MM-DD HH:MM:SS"))
		}
		if t.ID > 0 && seen[t.ID] {
			bad = append(bad, issue("tracker", t.ID, "id", "такой id уже есть в файле"))
		}
		if len(bad) > 0 {
			issues = append(issues, bad...)
			continue
		}
		seen[t.ID] = true
		out = append(out, t)
	}
	return out, issues
}

func filterTracks(items []trackSrc, issues []ImportIssue) ([]trackSrc, []ImportIssue) {
	seen := map[string]bool{}
	out := make([]trackSrc, 0, len(items))
	for _, t := range items {
		var bad []ImportIssue
		if !allowedTracks[t.Code] {
			bad = append(bad, issue("track", 0, "code", "код должен быть dumps, patterns, system_design или legend"))
		}
		if strings.TrimSpace(t.Title) == "" {
			bad = append(bad, issue("track", 0, "title", "пустой заголовок"))
		}
		if strings.TrimSpace(t.Why) == "" {
			bad = append(bad, issue("track", 0, "why", "пустое поле why"))
		}
		if strings.TrimSpace(t.HowToTrain) == "" {
			bad = append(bad, issue("track", 0, "how_to_train", "пустое поле how_to_train"))
		}
		if t.TimeMin <= 0 {
			bad = append(bad, issue("track", 0, "time_min", "время должно быть больше нуля"))
		}
		if t.Code != "" && seen[t.Code] {
			bad = append(bad, issue("track", 0, "code", "такой код уже есть в файле"))
		}
		if len(bad) > 0 {
			issues = append(issues, bad...)
			continue
		}
		seen[t.Code] = true
		out = append(out, t)
	}
	return out, issues
}

func filterProtocol(items []protocolSrc, issues []ImportIssue) ([]protocolSrc, []ImportIssue) {
	out := make([]protocolSrc, 0, len(items))
	seen := map[int64]bool{}
	for _, p := range items {
		if strings.TrimSpace(p.Body) == "" {
			issues = append(issues, issue("protocol", p.ID, "body", "пустой текст протокола"))
			continue
		}
		id := p.ID
		if id == 0 {
			id = 1
		}
		if seen[id] {
			issues = append(issues, issue("protocol", id, "id", "такой id уже есть в файле"))
			continue
		}
		seen[id] = true
		out = append(out, p)
	}
	return out, issues
}

func issue(kind string, id int64, field, message string) ImportIssue {
	return ImportIssue{Kind: kind, ID: id, Field: field, Message: message}
}

func validTimestamp(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if _, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
