package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"prepapp/internal/ai"
	"prepapp/internal/auth"
	"prepapp/internal/store"
)

type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

type Server struct {
	Store          *store.Store
	Completer      Completer
	WebDir         string
	Guard          *auth.Guard
	AllowedOrigins []string
	TrustProxy     bool
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/auth/status", s.authStatus)
	mux.HandleFunc("POST /api/auth/login", s.authLogin)
	mux.HandleFunc("POST /api/auth/setup", s.authSetup)
	mux.HandleFunc("POST /api/auth/logout", s.authLogout)
	mux.HandleFunc("POST /api/auth/password", s.authChangePassword)
	mux.HandleFunc("GET /api/bar", s.bar)
	mux.HandleFunc("GET /api/feed", s.feed)
	mux.HandleFunc("GET /api/items/{id}", s.item)
	mux.HandleFunc("GET /api/hard", s.hard)
	mux.HandleFunc("GET /api/queue", s.queue)
	mux.HandleFunc("POST /api/answer", s.answer)
	mux.HandleFunc("POST /api/review", s.review)
	mux.HandleFunc("POST /api/search", s.search)
	mux.HandleFunc("POST /api/queue", s.enqueue)
	if s.WebDir != "" {
		mux.Handle("/", spaHandler(s.WebDir))
	}
	var h http.Handler = mux
	h = protect(h, s.Guard)
	h = withCORS(h, s.AllowedOrigins)
	h = logRequests(h)
	return withSecurity(h, s.TrustProxy)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	n, err := s.Store.CountItems()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"items":      n,
		"ai":         s.Completer != nil,
		"data_ready": n > 0,
	})
}

func (s *Server) bar(w http.ResponseWriter, r *http.Request) {
	bar, err := s.Store.Bar()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, bar)
}

func (s *Server) feed(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "mobile"
	}
	if mode != "mobile" && mode != "web" {
		writeErr(w, http.StatusBadRequest, errors.New("mode должен быть mobile или web"))
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = r.URL.Query().Get("filter")
	}
	var skip int64
	if raw := r.URL.Query().Get("skip_id"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, errors.New("skip_id должен быть числом"))
			return
		}
		skip = n
	}
	item, err := s.Store.Next(store.FeedFilter{Mode: mode, Kind: kind, SkipID: skip})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (s *Server) item(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("некорректный id"))
		return
	}
	item, err := s.Store.Get(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, errors.New("карточка не найдена"))
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) hard(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListHard()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) queue(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Queue()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

type answerBody struct {
	QuestionID    int64  `json:"question_id"`
	Passed        bool   `json:"passed"`
	UserAnswer    string `json:"user_answer"`
	SelectedIndex *int   `json:"selected_index"`
}

func (s *Server) answer(w http.ResponseWriter, r *http.Request) {
	var body answerBody
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.QuestionID == 0 {
		writeErr(w, http.StatusBadRequest, errors.New("нужен question_id"))
		return
	}
	res, err := s.Store.RecordAnswer(store.AnswerInput{
		QuestionID:    body.QuestionID,
		Passed:        body.Passed,
		UserAnswer:    body.UserAnswer,
		SelectedIndex: body.SelectedIndex,
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, errors.New("карточка не найдена"))
		return
	}
	if errors.Is(err, store.ErrCodeNeedsReview) {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type reviewBody struct {
	QuestionID int64  `json:"question_id"`
	Code       string `json:"code"`
}

func (s *Server) review(w http.ResponseWriter, r *http.Request) {
	var body reviewBody
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.QuestionID == 0 {
		writeErr(w, http.StatusBadRequest, errors.New("нужен question_id"))
		return
	}
	if strings.TrimSpace(body.Code) == "" {
		writeErr(w, http.StatusBadRequest, errors.New("нужен code"))
		return
	}
	item, err := s.Store.Get(body.QuestionID)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, errors.New("карточка не найдена"))
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if s.Completer == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("OPENROUTER_API_KEY не задан"))
		return
	}
	user := buildReviewPrompt(*item, body.Code)
	raw, err := s.Completer.Complete(r.Context(), ai.ReviewSystemPrompt, user)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	verdict, err := ai.ParseReview(raw)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	note := strings.Join(verdict.Bugs, "; ")
	if err := s.Store.MarkReview(*item, verdict.Passed, note); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, verdict)
}

func buildReviewPrompt(item store.Item, code string) string {
	var b strings.Builder
	b.WriteString("Задача:\n")
	b.WriteString(item.Prompt)
	b.WriteString("\n\nКритерии:\n")
	if item.ReviewCriteria != "" {
		b.WriteString(item.ReviewCriteria)
		b.WriteString("\n")
	}
	if len(item.Checklist) > 0 {
		b.WriteString(strings.Join(item.Checklist, "\n"))
		b.WriteString("\n")
	}
	if strings.TrimSpace(item.ReferenceCode) != "" {
		b.WriteString("\nЭталон (только для тебя, кандидату его не показывали):\n")
		b.WriteString(item.ReferenceCode)
		b.WriteString("\n")
	}
	b.WriteString("\nКод кандидата (")
	b.WriteString(item.Language)
	b.WriteString("):\n")
	b.WriteString(code)
	return b.String()
}

type searchBody struct {
	Query string `json:"query"`
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	var body searchBody
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	q := strings.TrimSpace(body.Query)
	if q == "" {
		writeErr(w, http.StatusBadRequest, errors.New("нужен query"))
		return
	}
	items, hard, err := s.Store.Search(q, 8)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	explanation := ""
	if s.Completer != nil {
		prompt := buildSearchPrompt(q, items, hard)
		text, err := s.Completer.Complete(r.Context(), ai.SearchSystemPrompt, prompt)
		if err != nil {
			explanation = ""
			log.Printf("search ai: %v", err)
		} else {
			explanation = strings.TrimSpace(text)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"query":       q,
		"matches":     items,
		"hard":        hard,
		"explanation": explanation,
	})
}

func buildSearchPrompt(query string, items, hard []store.SearchHit) string {
	var b strings.Builder
	b.WriteString("Запрос: ")
	b.WriteString(query)
	b.WriteString("\n\nКарточки:\n")
	if len(items) == 0 && len(hard) == 0 {
		b.WriteString("совпадений нет\n")
		return b.String()
	}
	for _, h := range items {
		b.WriteString("- [")
		b.WriteString(h.BlockTitle)
		b.WriteString("] ")
		b.WriteString(h.Title)
		b.WriteString(": ")
		b.WriteString(h.Snippet)
		b.WriteString("\n")
	}
	for _, h := range hard {
		b.WriteString("- [hard] ")
		b.WriteString(h.Title)
		b.WriteString(": ")
		b.WriteString(h.Snippet)
		b.WriteString("\n")
	}
	return b.String()
}

func (s *Server) enqueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		QuestionID int64 `json:"question_id"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.QuestionID == 0 {
		writeErr(w, http.StatusBadRequest, errors.New("нужен question_id"))
		return
	}
	if err := s.Store.Enqueue(body.QuestionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, errors.New("карточка не найдена"))
			return
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func decode(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		return errors.New("некорректный JSON")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)
		log.Printf("%s %s %d", r.Method, r.URL.RequestURI(), ww.status)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
