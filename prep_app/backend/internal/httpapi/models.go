package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"prepapp/internal/ai"
)

// ModelDirectory is the live OpenRouter catalog used by settings.
type ModelDirectory interface {
	Search(ctx context.Context, query string) (ai.CatalogPage, error)
	Resolve(ctx context.Context, modelID, provider string) (string, error)
	HasKey() bool
}

func (s *Server) modelSettings(w http.ResponseWriter, r *http.Request) {
	s.writeModelSettings(w, http.StatusOK)
}

func (s *Server) saveModel(w http.ResponseWriter, r *http.Request) {
	if s.Selection == nil || s.Models == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("выбор модели не настроен"))
		return
	}
	var body struct {
		Model    string `json:"model"`
		Provider string `json:"provider"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	name, err := s.Models.Resolve(r.Context(), body.Model, body.Provider)
	if err != nil {
		writeCatalogErr(w, err)
		return
	}
	choice, err := s.Selection.Save(body.Model, body.Provider, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if choice.Provider == "" {
		log.Printf("модель сохранена: %s, поставщик выбирает OpenRouter", choice.Model)
	} else {
		log.Printf("модель сохранена: %s, поставщик %s", choice.Model, choice.Provider)
	}
	s.writeModelSettings(w, http.StatusOK)
}

func (s *Server) resetModel(w http.ResponseWriter, r *http.Request) {
	if s.Selection == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("выбор модели не настроен"))
		return
	}
	choice, err := s.Selection.Reset()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	log.Printf("модель сброшена к %s", choice.Model)
	s.writeModelSettings(w, http.StatusOK)
}

func (s *Server) modelSearch(w http.ResponseWriter, r *http.Request) {
	if s.Models == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("каталог моделей не настроен"))
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(query) < 2 {
		writeErr(w, http.StatusBadRequest, errors.New("введите хотя бы 2 символа"))
		return
	}
	if utf8.RuneCountInString(query) > 80 {
		writeErr(w, http.StatusBadRequest, errors.New("запрос длиннее 80 символов"))
		return
	}
	page, err := s.Models.Search(r.Context(), query)
	if err != nil {
		writeCatalogErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) writeModelSettings(w http.ResponseWriter, status int) {
	if s.Selection == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("выбор модели не настроен"))
		return
	}
	cur := s.Selection.Current()
	needsKey := true
	if s.Models != nil {
		needsKey = !s.Models.HasKey()
	}
	writeJSON(w, status, map[string]any{
		"ai":                s.Completer != nil,
		"model":             cur.Model,
		"provider":          cur.Provider,
		"provider_name":     cur.ProviderName,
		"source":            cur.Source,
		"env_model":         cur.EnvModel,
		"latency_needs_key": needsKey,
	})
}

func writeCatalogErr(w http.ResponseWriter, err error) {
	var ce *ai.CatalogError
	if errors.As(err, &ce) && ce.Status != 0 {
		writeErr(w, ce.Status, ce)
		return
	}
	writeErr(w, http.StatusBadGateway, err)
}
