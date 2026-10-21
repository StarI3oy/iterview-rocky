package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var bearerRE = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/-]+=*`)

const defaultModel = "openai/gpt-4o-mini"

type Client struct {
	Key     string
	Model   string
	BaseURL string
	HTTP    *http.Client
	Referer string
}

func New(key, model string) *Client {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if strings.TrimSpace(model) == "" {
		model = defaultModel
	}
	return &Client{
		Key:     key,
		Model:   model,
		BaseURL: "https://openrouter.ai/api/v1/chat/completions",
		Referer: "http://localhost:8080",
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	if c == nil || c.Key == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY is not set")
	}
	body, err := json.Marshal(chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", c.Referer)
	req.Header.Set("X-Title", "prep-app")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("openrouter: %s", c.redact(clip(string(raw), 300)))
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("openrouter: %s", c.redact(parsed.Error.Message))
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("openrouter: http %d", resp.StatusCode)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("openrouter: empty completion")
	}
	return parsed.Choices[0].Message.Content, nil
}

func (c *Client) redact(s string) string {
	if c != nil && c.Key != "" {
		s = strings.ReplaceAll(s, c.Key, "[redacted]")
	}
	return bearerRE.ReplaceAllString(s, "Bearer [redacted]")
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

const ReviewSystemPrompt = `Ты жёсткий Senior Go интервьюер (планка Ozon/Авито, 400k+).
Оцени решение кандидата. Смотри на корректность, гонки, утечки, закрытие каналов, отмену через context, обработку ошибок, сложность по времени и памяти, SQL-инъекции и индексы — если это уместно задаче.
passed=true только если решение можно принять на интервью без принципиальных дефектов.
score — целое от 1 до 5.
Верни ТОЛЬКО JSON без markdown:
{"passed":false,"score":1,"bugs":[],"style_notes":[],"follow_up_question":""}`

const SearchSystemPrompt = `Ты наставник по собеседованиям на планку 400k+ (Go, Postgres/Kafka, алгоритмы, system design).
Отвечай по-русски, сухо и коротко: 6–14 строк.
Опирайся на переданные карточки из базы. Если факта в карточках нет — прямо скажи, что в загруженной базе этого нет, и дай только общую рамку без выдуманных цитат компаний.`

type Review struct {
	Passed           bool     `json:"passed"`
	Score            int      `json:"score"`
	Bugs             []string `json:"bugs"`
	StyleNotes       []string `json:"style_notes"`
	FollowUpQuestion string   `json:"follow_up_question"`
}

func ParseReview(raw string) (Review, error) {
	raw = strings.TrimSpace(raw)
	raw = stripFence(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}
	var r Review
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return Review{}, fmt.Errorf("разбор вердикта: %w", err)
	}
	if r.Score < 1 {
		r.Score = 1
	}
	if r.Score > 5 {
		r.Score = 5
	}
	if r.Bugs == nil {
		r.Bugs = []string{}
	}
	if r.StyleNotes == nil {
		r.StyleNotes = []string{}
	}
	return r, nil
}

func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}
