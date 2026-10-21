package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFormatPricingKeepsUnits(t *testing.T) {
	raw := []byte(`{
		"prompt": "0.00000015",
		"completion": "0.0000006",
		"request": "0",
		"image": "0",
		"web_search": "0.01",
		"internal_reasoning": "0.0000006",
		"input_cache_read": "0.000000075",
		"input_cache_write": "0.0000001875",
		"input_cache_write_1h": "0.0000003",
		"custom_fee": "0.2",
		"discount": 0,
		"overrides": [{"min_prompt_tokens": 100000, "prompt": "0.0000005", "completion": "0.0000025"}]
	}`)
	got := formatPricing(raw)
	want := []string{
		"Вход: $0.15 за 1M токенов ($0.00000015 за токен)",
		"Выход: $0.6 за 1M токенов ($0.0000006 за токен)",
		"Внутреннее рассуждение: $0.6 за 1M токенов ($0.0000006 за токен)",
		"Запрос: $0 за запрос",
		"Картинка на входе: $0 за изображение",
		"Веб-поиск: $0.01 за поиск",
		"Чтение кэша: $0.075 за 1M токенов ($0.000000075 за токен)",
		"Запись кэша (5 мин): $0.1875 за 1M токенов ($0.0000001875 за токен)",
		"Запись кэша (1 ч): $0.3 за 1M токенов ($0.0000003 за токен)",
		"custom_fee: $0.2 (единица OpenRouter)",
		"Скидка: 0%",
	}
	if strings.Join(got.lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("lines:\n%s", strings.Join(got.lines, "\n"))
	}
	if len(got.overrides) != 1 || got.overrides[0] != "Если входных токенов больше 100000: Вход: $0.5 за 1M токенов ($0.0000005 за токен); Выход: $2.5 за 1M токенов ($0.0000025 за токен)" {
		t.Fatalf("overrides %#v", got.overrides)
	}
}

func TestDescribePolicy(t *testing.T) {
	days := 30
	collects, line := describePolicy(policy{known: true, retains: true, retentionDays: &days})
	if collects == nil || !*collects || line != "Собирает данные: хранит промпты 30 дн., на обучении не использует" {
		t.Fatalf("%v %s", collects, line)
	}
	collects, line = describePolicy(policy{known: true, training: true, retains: true})
	if collects == nil || !*collects || !strings.Contains(line, "может обучать модель на запросах") {
		t.Fatalf("%v %s", collects, line)
	}
	collects, line = describePolicy(policy{known: true, requiresUserIDs: true})
	if collects == nil || *collects || !strings.Contains(line, "просит идентификатор пользователя") {
		t.Fatalf("%v %s", collects, line)
	}
	collects, line = describePolicy(policy{})
	if collects != nil || line != "Политика сбора данных не опубликована" {
		t.Fatalf("%v %s", collects, line)
	}
}

func TestSearchCatalog(t *testing.T) {
	var pages []string
	var sawKey bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer secret-key" {
			sawKey = true
		}
		switch {
		case r.URL.Path == "/models":
			pages = append(pages, r.URL.Query().Get("offset"))
			offset := r.URL.Query().Get("offset")
			if offset == "0" {
				writeJSON(w, map[string]any{
					"total_count": 3,
					"data":        []any{haikuModel(), miniModel()},
				})
				return
			}
			writeJSON(w, map[string]any{
				"total_count": 3,
				"data":        []any{goneModel()},
			})
		case r.URL.Path == "/models/anthropic/claude-haiku/endpoints":
			writeJSON(w, map[string]any{"data": map[string]any{"endpoints": []any{anthropicEndpoint(), bedrockEndpoint()}}})
		case r.URL.Path == "/models/openai/gpt-4o-mini/endpoints":
			writeJSON(w, map[string]any{"data": map[string]any{"endpoints": []any{openAIEndpoint()}}})
		case r.URL.Path == "/models/other/gone-model/endpoints":
			http.NotFound(w, r)
		case r.URL.Path == "/providers":
			writeJSON(w, map[string]any{"data": []any{
				map[string]any{
					"name": "Anthropic", "slug": "anthropic",
					"dataPolicy": map[string]any{"training": false, "retainsPrompts": true, "retentionDays": 30},
				},
				map[string]any{
					"name": "Amazon Bedrock", "slug": "amazon-bedrock",
					"dataPolicy": map[string]any{"training": false, "retainsPrompts": false, "requiresUserIDs": true},
				},
				map[string]any{
					"name": "OpenAI", "slug": "openai",
					"dataPolicy": map[string]any{"training": false, "retainsPrompts": false},
				},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := NewDirectory("secret-key")
	dir.APIBase = srv.URL
	dir.ProvidersURL = srv.URL + "/providers"
	dir.PageLimit = 2
	dir.HTTP.Timeout = 2 * timeSecond

	page, err := dir.Search(context.Background(), "haiku")
	if err != nil {
		t.Fatal(err)
	}
	if !sawKey {
		t.Fatal("api key was not sent")
	}
	if len(pages) != 2 || pages[0] != "0" || pages[1] != "2" {
		t.Fatalf("pages %#v", pages)
	}
	if page.MatchCount != 1 || len(page.Results) != 1 || page.LatencyNeedsKey || !page.DataPolicyLoaded {
		t.Fatalf("%+v", page)
	}
	model := page.Results[0]
	if model.ID != "anthropic/claude-haiku" || model.PriceHint != "вход от $0.15 / 1M, выход от $0.6 / 1M" {
		t.Fatalf("%+v", model)
	}
	if len(model.PricingLines) != 0 {
		t.Fatalf("model price duplicated %#v", model.PricingLines)
	}
	if strings.Join(model.BenchmarkLines, "|") != "Интеллект 43.4|Код 11.4" {
		t.Fatalf("benchmark %#v %s", model.BenchmarkLines, model.BenchmarkNote)
	}
	if len(model.Providers) != 2 {
		t.Fatalf("providers %d", len(model.Providers))
	}
	first := model.Providers[0]
	if first.Tag != "anthropic" || first.Availability != "доступен" || first.CollectsData == nil || !*first.CollectsData {
		t.Fatalf("%+v", first)
	}
	if first.DataLine != "Собирает данные: хранит промпты 30 дн., на обучении не использует" {
		t.Fatalf("data %s", first.DataLine)
	}
	if first.LatencyLine != "До первого токена за 30 мин: p50 650 мс, p75 800 мс, p90 1.2 с, p99 2.1 с" {
		t.Fatalf("latency %s", first.LatencyLine)
	}
	if first.UptimeLine != "Доступность: 5 мин 100%, 30 мин 99.5%, сутки нет данных" {
		t.Fatalf("uptime %s", first.UptimeLine)
	}
	if first.ThroughputLine != "Скорость генерации за 30 мин: p50 40.5 ток/с" {
		t.Fatalf("throughput %s", first.ThroughputLine)
	}
	if !containsLine(first.PricingLines, "Веб-поиск: $0.01 за поиск") || !containsLine(first.PricingLines, "Вход: $0.15 за 1M токенов ($0.00000015 за токен)") {
		t.Fatalf("pricing %#v", first.PricingLines)
	}
	second := model.Providers[1]
	if second.Availability != "деградация" || second.CollectsData == nil || *second.CollectsData {
		t.Fatalf("%+v", second)
	}
	if second.LatencyLine != "" {
		t.Fatalf("unexpected latency %s", second.LatencyLine)
	}
	if !strings.Contains(second.DataLine, "просит идентификатор пользователя") {
		t.Fatalf("bedrock %s", second.DataLine)
	}

	gone, err := dir.Search(context.Background(), "gone")
	if err != nil {
		t.Fatal(err)
	}
	if len(gone.Results) != 1 || gone.Results[0].ProvidersError == "" || len(gone.Results[0].PricingLines) == 0 {
		t.Fatalf("%+v", gone.Results)
	}

	name, err := dir.Resolve(context.Background(), "anthropic/claude-haiku", "amazon-bedrock")
	if err != nil || name != "Amazon Bedrock" {
		t.Fatalf("resolve %q %v", name, err)
	}
	if _, err := dir.Resolve(context.Background(), "anthropic/claude-haiku", "missing"); err == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestDirectoryDoesNotLeakKeyOnRedirect(t *testing.T) {
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("key reached %s", r.Host)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer evil.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/steal", http.StatusFound)
	}))
	defer good.Close()
	dir := NewDirectory("secret-key")
	dir.APIBase = good.URL
	dir.HTTP.Timeout = timeSecond
	_, err := dir.Search(context.Background(), "haiku")
	if err == nil {
		t.Fatal("expected redirect error")
	}
	if strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("secret leaked: %v", err)
	}
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

const timeSecond = 1000000000

func haikuModel() map[string]any {
	return map[string]any{
		"id": "anthropic/claude-haiku", "name": "Claude Haiku", "description": "Коротко.",
		"context_length": 200000,
		"benchmarks": map[string]any{"artificial_analysis": map[string]any{
			"intelligence_index": 43.4, "coding_index": 11.4, "agentic_index": nil,
		}},
		"pricing": map[string]any{"prompt": "0.000001", "completion": "0.000005"},
	}
}

func miniModel() map[string]any {
	return map[string]any{
		"id": "openai/gpt-4o-mini", "name": "GPT-4o mini", "context_length": 128000,
		"benchmarks":  map[string]any{"artificial_analysis": map[string]any{"intelligence_index": 6.7}},
		"pricing":     map[string]any{"prompt": "0.00000015", "completion": "0.0000006"},
		"description": "mini",
	}
}

func goneModel() map[string]any {
	return map[string]any{
		"id": "other/gone-model", "name": "Gone Model", "context_length": 8000,
		"pricing": map[string]any{"prompt": "0.000001"},
	}
}

func anthropicEndpoint() map[string]any {
	return map[string]any{
		"provider_name": "Anthropic", "tag": "anthropic", "quantization": "unknown",
		"context_length": 200000, "max_completion_tokens": 8192, "status": 0,
		"uptime_last_5m": 100, "uptime_last_30m": 99.5, "uptime_last_1d": nil,
		"latency_last_30m":    map[string]any{"p50": 650, "p75": 800, "p90": 1200, "p99": 2100},
		"throughput_last_30m": map[string]any{"p50": 40.5},
		"pricing": map[string]any{
			"prompt": "0.00000015", "completion": "0.0000006", "web_search": "0.01", "discount": 0,
		},
	}
}

func bedrockEndpoint() map[string]any {
	return map[string]any{
		"provider_name": "Amazon Bedrock", "tag": "amazon-bedrock", "status": 2,
		"context_length":   200000,
		"latency_last_30m": nil, "throughput_last_30m": nil,
		"uptime_last_5m": nil, "uptime_last_30m": nil, "uptime_last_1d": nil,
		"pricing": map[string]any{"prompt": "0.0000002", "completion": "0.0000008"},
	}
}

func openAIEndpoint() map[string]any {
	return map[string]any{
		"provider_name": "OpenAI", "tag": "openai", "status": 0,
		"pricing": map[string]any{"prompt": "0.00000015", "completion": "0.0000006"},
	}
}
