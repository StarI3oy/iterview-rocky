package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

var (
	modelIDRE     = regexp.MustCompile(`^~?[A-Za-z0-9][A-Za-z0-9._+-]{0,80}/[A-Za-z0-9][A-Za-z0-9._+:-]{0,160}$`)
	providerTagRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+/-]{0,120}$`)
)

func validModelID(s string) bool {
	return modelIDRE.MatchString(s) && strings.Count(s, "/") == 1
}

func validProviderTag(s string) bool {
	return providerTagRE.MatchString(s)
}

// Choice is the model used for review and search explanations.
type Choice struct {
	Model        string
	Provider     string
	ProviderName string
	Source       string
	EnvModel     string
}

// Selection stores the model picked in settings. An empty file means OPENROUTER_MODEL.
type Selection struct {
	mu           sync.Mutex
	path         string
	envModel     string
	model        string
	provider     string
	providerName string
	source       string
}

type selectionFile struct {
	Version      int    `json:"version"`
	Model        string `json:"model"`
	Provider     string `json:"provider,omitempty"`
	ProviderName string `json:"provider_name,omitempty"`
}

// OpenSelection reads the settings file. A missing file keeps the env model.
// A corrupt file is returned as an error and left untouched.
func OpenSelection(path, envModel string) (*Selection, error) {
	envModel = strings.TrimSpace(envModel)
	if envModel == "" {
		envModel = defaultModel
	}
	if !validModelID(envModel) {
		return nil, fmt.Errorf("OPENROUTER_MODEL %q не похож на id модели. Базы и ключ не изменены", envModel)
	}
	s := &Selection{
		path:     path,
		envModel: envModel,
		model:    envModel,
		source:   "env",
	}
	if strings.TrimSpace(path) == "" {
		return s, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var file selectionFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("файл выбора модели повреждён (%s), базы и ключ не изменены: %w", path, err)
	}
	if file.Version != 1 || !validModelID(strings.TrimSpace(file.Model)) {
		return nil, fmt.Errorf("файл выбора модели %s имеет неизвестную версию или id, базы и ключ не изменены", path)
	}
	provider := strings.TrimSpace(file.Provider)
	if provider != "" && !validProviderTag(provider) {
		return nil, fmt.Errorf("файл выбора модели %s содержит непонятного поставщика, базы и ключ не изменены", path)
	}
	s.model = strings.TrimSpace(file.Model)
	s.provider = provider
	s.providerName = cleanName(file.ProviderName)
	s.source = "settings"
	_ = os.Chmod(path, 0o600)
	return s, nil
}

// Current is a copy of the active choice.
func (s *Selection) Current() Choice {
	if s == nil {
		return Choice{Model: defaultModel, Source: "env", EnvModel: defaultModel}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentLocked()
}

func (s *Selection) currentLocked() Choice {
	return Choice{
		Model:        s.model,
		Provider:     s.provider,
		ProviderName: s.providerName,
		Source:       s.source,
		EnvModel:     s.envModel,
	}
}

// Save pins a model and, when provider is set, one OpenRouter provider slug.
func (s *Selection) Save(model, provider, providerName string) (Choice, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return Choice{}, errors.New("некуда сохранить выбор модели")
	}
	model = strings.TrimSpace(model)
	provider = strings.TrimSpace(provider)
	if !validModelID(model) {
		return Choice{}, errors.New("некорректный id модели")
	}
	if provider != "" && !validProviderTag(provider) {
		return Choice{}, errors.New("некорректный поставщик")
	}
	file := selectionFile{
		Version:      1,
		Model:        model,
		Provider:     provider,
		ProviderName: cleanName(providerName),
	}
	if err := writeSelection(s.path, file); err != nil {
		return Choice{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = file.Model
	s.provider = file.Provider
	s.providerName = file.ProviderName
	s.source = "settings"
	return s.currentLocked(), nil
}

// Reset removes the saved choice and returns to OPENROUTER_MODEL.
func (s *Selection) Reset() (Choice, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return Choice{}, errors.New("некуда сохранить выбор модели")
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Choice{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = s.envModel
	s.provider = ""
	s.providerName = ""
	s.source = "env"
	return s.currentLocked(), nil
}

func writeSelection(path string, file selectionFile) error {
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".prep_settings-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return os.Chmod(path, 0o600)
}

func cleanName(s string) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s))
	if utf8.RuneCountInString(s) <= 120 {
		return s
	}
	r := []rune(s)
	return string(r[:120])
}
