package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type priced struct {
	lines      []string
	overrides  []string
	prompt     *float64
	completion *float64
}

type policy struct {
	known              bool
	training           bool
	trainingOpenRouter bool
	retains            bool
	safety             bool
	canPublish         bool
	requiresUserIDs    bool
	retentionDays      *int
}

type percentiles struct {
	P50 *float64
	P75 *float64
	P90 *float64
	P99 *float64
}

type priceField struct {
	key      string
	label    string
	unit     string
	perToken bool
}

var priceFields = []priceField{
	{key: "prompt", label: "Вход", perToken: true},
	{key: "completion", label: "Выход", perToken: true},
	{key: "internal_reasoning", label: "Внутреннее рассуждение", perToken: true},
	{key: "request", label: "Запрос", unit: "за запрос"},
	{key: "image", label: "Картинка на входе", unit: "за изображение"},
	{key: "image_output", label: "Картинка на выходе", unit: "за изображение"},
	{key: "image_token", label: "Токен картинки", perToken: true},
	{key: "audio", label: "Аудио на входе", perToken: true},
	{key: "audio_output", label: "Аудио на выходе", perToken: true},
	{key: "input_audio_cache", label: "Кэш аудио", perToken: true},
	{key: "web_search", label: "Веб-поиск", unit: "за поиск"},
	{key: "input_cache_read", label: "Чтение кэша", perToken: true},
	{key: "input_cache_write", label: "Запись кэша (5 мин)", perToken: true},
	{key: "input_cache_write_1h", label: "Запись кэша (1 ч)", perToken: true},
}

var conditionKeys = map[string]struct{}{
	"min_prompt_tokens": {},
	"utc_start":         {},
	"utc_end":           {},
	"utc_days":          {},
	"overrides":         {},
	"discount":          {},
}

var dayName = map[string]string{
	"monday":    "пн",
	"tuesday":   "вт",
	"wednesday": "ср",
	"thursday":  "чт",
	"friday":    "пт",
	"saturday":  "сб",
	"sunday":    "вс",
}

func formatPricing(raw json.RawMessage) priced {
	obj := objectMap(raw)
	if len(obj) == 0 {
		return priced{}
	}
	var out priced
	used := map[string]struct{}{}
	for _, field := range priceFields {
		value, ok := obj[field.key]
		if !ok {
			continue
		}
		used[field.key] = struct{}{}
		line, amount, ok := formatPriceField(field, value)
		if !ok {
			continue
		}
		out.lines = append(out.lines, line)
		if field.key == "prompt" {
			out.prompt = amount
		}
		if field.key == "completion" {
			out.completion = amount
		}
	}
	var unknown []string
	for key := range obj {
		if _, skip := conditionKeys[key]; skip {
			continue
		}
		if _, ok := used[key]; ok {
			continue
		}
		unknown = append(unknown, key)
	}
	sort.Strings(unknown)
	for _, key := range unknown {
		if line, ok := formatUnknownPrice(key, obj[key]); ok {
			out.lines = append(out.lines, line)
		}
	}
	if rawDiscount, ok := obj["discount"]; ok {
		if amount, ok := parseNumber(rawDiscount); ok {
			out.lines = append(out.lines, formatDiscount(amount))
		}
	}
	out.overrides = formatOverrides(obj["overrides"])
	return out
}

func formatPriceField(field priceField, raw json.RawMessage) (string, *float64, bool) {
	amount, ok := parseNumber(raw)
	if !ok {
		return "", nil, false
	}
	if field.perToken {
		return fmt.Sprintf("%s: $%s за 1M токенов ($%s за токен)", field.label, formatMoney(amount*1_000_000), formatMoney(amount)), &amount, true
	}
	return fmt.Sprintf("%s: $%s %s", field.label, formatMoney(amount), field.unit), &amount, true
}

func formatUnknownPrice(key string, raw json.RawMessage) (string, bool) {
	amount, ok := parseNumber(raw)
	if !ok {
		text := unquote(raw)
		if text == "" {
			return "", false
		}
		return key + ": " + text + " (единица OpenRouter)", true
	}
	return fmt.Sprintf("%s: $%s (единица OpenRouter)", key, formatMoney(amount)), true
}

func formatDiscount(fraction float64) string {
	pct := fraction * 100
	s := strconv.FormatFloat(pct, 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-0" {
		s = "0"
	}
	return "Скидка: " + s + "%"
}

func formatOverrides(raw json.RawMessage) []string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	var lines []string
	for _, item := range items {
		obj := objectMap(item)
		if len(obj) == 0 {
			continue
		}
		var bits []string
		for _, field := range priceFields {
			value, ok := obj[field.key]
			if !ok {
				continue
			}
			line, _, ok := formatPriceField(field, value)
			if ok {
				bits = append(bits, line)
			}
		}
		if len(bits) == 0 {
			continue
		}
		lines = append(lines, overridePrefix(obj)+": "+strings.Join(bits, "; "))
	}
	return lines
}

func overridePrefix(obj map[string]json.RawMessage) string {
	var bits []string
	if n := numPtr(obj["min_prompt_tokens"]); n != nil {
		bits = append(bits, "если входных токенов больше "+formatMeasure(*n))
	}
	start := numPtr(obj["utc_start"])
	end := numPtr(obj["utc_end"])
	if start != nil || end != nil {
		from, to := "00:00", "24:00"
		if start != nil {
			from = formatHHMM(*start)
		}
		if end != nil {
			to = formatHHMM(*end)
		}
		bits = append(bits, "время "+from+"–"+to+" UTC")
	}
	if days := dayList(obj["utc_days"]); len(days) > 0 {
		bits = append(bits, "дни "+strings.Join(days, ", "))
	}
	if len(bits) == 0 {
		return "Особое условие"
	}
	return capitalize(strings.Join(bits, ", "))
}

func capitalize(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	upper := strings.ToUpper(string(r[0]))
	return upper + string(r[1:])
}

func dayList(raw json.RawMessage) []string {
	var days []string
	if err := json.Unmarshal(bytes.TrimSpace(raw), &days); err != nil {
		return nil
	}
	var out []string
	for _, day := range days {
		if name, ok := dayName[strings.ToLower(strings.TrimSpace(day))]; ok {
			out = append(out, name)
		}
	}
	return out
}

func formatHHMM(v float64) string {
	n := int(math.Round(v))
	if n < 0 {
		n = 0
	}
	hh := n / 100
	mm := n % 100
	if hh > 23 {
		hh = 23
	}
	if mm > 59 {
		mm = 59
	}
	return fmt.Sprintf("%02d:%02d", hh, mm)
}

func formatMoney(v float64) string {
	if v == 0 {
		return "0"
	}
	abs := math.Abs(v)
	if abs >= 0.01 {
		s := strconv.FormatFloat(v, 'f', 4, 64)
		return strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func formatMeasure(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0"
	}
	if v == math.Trunc(v) && math.Abs(v) < 1e12 {
		return strconv.FormatInt(int64(v), 10)
	}
	s := strconv.FormatFloat(v, 'f', 1, 64)
	return strings.TrimRight(strings.TrimRight(s, "0"), ".")
}

func describePolicy(p policy) (collects *bool, line string) {
	if !p.known {
		return nil, "Политика сбора данных не опубликована"
	}
	collectsData := p.training || p.trainingOpenRouter || p.retains || p.safety || p.canPublish
	collects = &collectsData
	var parts []string
	if p.retains {
		if p.retentionDays != nil && *p.retentionDays > 0 {
			parts = append(parts, fmt.Sprintf("хранит промпты %d дн.", *p.retentionDays))
		} else {
			parts = append(parts, "хранит промпты, срок не указан")
		}
	} else {
		parts = append(parts, "промпты не хранит")
	}
	switch {
	case p.training && p.trainingOpenRouter:
		parts = append(parts, "поставщик и OpenRouter могут обучать модель на запросах")
	case p.training:
		parts = append(parts, "может обучать модель на запросах")
	case p.trainingOpenRouter:
		parts = append(parts, "OpenRouter может обучать модель на запросах")
	default:
		parts = append(parts, "на обучении не использует")
	}
	if p.safety {
		parts = append(parts, "хранит для проверки безопасности")
	}
	if p.canPublish {
		parts = append(parts, "может публиковать промпты")
	}
	if p.requiresUserIDs {
		parts = append(parts, "просит идентификатор пользователя")
	}
	prefix := "Не собирает данные: "
	if collectsData {
		prefix = "Собирает данные: "
	}
	return collects, prefix + strings.Join(parts, ", ")
}

func benchmarkView(intel, coding, agentic *float64) (lines []string, note string) {
	if intel == nil && coding == nil && agentic == nil {
		return []string{}, "Оценки Artificial Analysis для этой модели нет"
	}
	if intel != nil {
		lines = append(lines, "Интеллект "+formatScore(*intel))
	}
	if coding != nil {
		lines = append(lines, "Код "+formatScore(*coding))
	}
	if agentic != nil {
		lines = append(lines, "Агенты "+formatScore(*agentic))
	}
	return lines, "Artificial Analysis через OpenRouter. Выше индекс — сильнее модель."
}

func formatScore(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e6 {
		return strconv.FormatInt(int64(v), 10)
	}
	s := strconv.FormatFloat(v, 'f', 1, 64)
	return strings.TrimRight(strings.TrimRight(s, "0"), ".")
}

func formatLatency(p *percentiles) string {
	parts := percentileParts(p, func(v float64) string { return formatMS(v) })
	if len(parts) == 0 {
		return ""
	}
	return "До первого токена за 30 мин: " + strings.Join(parts, ", ")
}

func formatThroughput(p *percentiles) string {
	parts := percentileParts(p, func(v float64) string { return formatMeasure(v) + " ток/с" })
	if len(parts) == 0 {
		return ""
	}
	return "Скорость генерации за 30 мин: " + strings.Join(parts, ", ")
}

func percentileParts(p *percentiles, format func(float64) string) []string {
	if p == nil {
		return nil
	}
	var parts []string
	add := func(name string, v *float64) {
		if v == nil {
			return
		}
		parts = append(parts, name+" "+format(*v))
	}
	add("p50", p.P50)
	add("p75", p.P75)
	add("p90", p.P90)
	add("p99", p.P99)
	return parts
}

func formatMS(ms float64) string {
	if ms >= 1000 {
		return formatMeasure(ms/1000) + " с"
	}
	if ms >= 100 {
		return strconv.FormatFloat(math.Round(ms), 'f', 0, 64) + " мс"
	}
	return formatMeasure(ms) + " мс"
}

func formatUptime(m5, m30, d1 *float64) string {
	part := func(label string, v *float64) string {
		if v == nil {
			return label + " нет данных"
		}
		return label + " " + formatScore(*v) + "%"
	}
	return "Доступность: " + part("5 мин", m5) + ", " + part("30 мин", m30) + ", " + part("сутки", d1)
}

func availabilityText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "статус не указан"
	}
	value, ok := parseNumber(raw)
	if !ok {
		return "статус не указан"
	}
	if value == 0 {
		return "доступен"
	}
	return "деградация"
}

func priceHint(prompt, completion *float64) string {
	switch {
	case prompt != nil && completion != nil:
		return fmt.Sprintf("вход от $%s / 1M, выход от $%s / 1M", formatMoney(*prompt*1_000_000), formatMoney(*completion*1_000_000))
	case prompt != nil:
		return fmt.Sprintf("вход от $%s / 1M", formatMoney(*prompt*1_000_000))
	case completion != nil:
		return fmt.Sprintf("выход от $%s / 1M", formatMoney(*completion*1_000_000))
	default:
		return ""
	}
}

func minPrice(current, next *float64) *float64 {
	if next == nil {
		return current
	}
	if current == nil || *next < *current {
		return next
	}
	return current
}

func parsePercentiles(raw json.RawMessage) *percentiles {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	if obj := objectMap(raw); len(obj) > 0 {
		p := &percentiles{
			P50: numPtr(obj["p50"]),
			P75: numPtr(obj["p75"]),
			P90: numPtr(obj["p90"]),
			P99: numPtr(obj["p99"]),
		}
		if p.P50 == nil && p.P75 == nil && p.P90 == nil && p.P99 == nil {
			return nil
		}
		return p
	}
	if n := numPtr(raw); n != nil {
		return &percentiles{P50: n}
	}
	return nil
}

func parsePolicyObject(raw json.RawMessage) policy {
	obj := objectMap(raw)
	if len(obj) == 0 {
		return policy{}
	}
	p := policy{known: true}
	p.training = truth(obj, "training")
	p.trainingOpenRouter = truth(obj, "trainingOpenRouter", "training_openrouter")
	p.retains = truth(obj, "retainsPrompts", "retains_prompts")
	p.safety = truth(obj, "safetyRetention", "safety_retention")
	p.canPublish = truth(obj, "canPublish", "can_publish")
	p.requiresUserIDs = truth(obj, "requiresUserIDs", "requires_user_ids")
	if n := firstNum(obj, "retentionDays", "retention_days"); n != nil {
		days := int(math.Round(*n))
		p.retentionDays = &days
	}
	return p
}

func truth(obj map[string]json.RawMessage, keys ...string) bool {
	for _, key := range keys {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		var b bool
		if err := json.Unmarshal(raw, &b); err == nil {
			return b
		}
	}
	return false
}

func firstNum(obj map[string]json.RawMessage, keys ...string) *float64 {
	for _, key := range keys {
		if n := numPtr(obj[key]); n != nil {
			return n
		}
	}
	return nil
}

func objectMap(raw json.RawMessage) map[string]json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || raw[0] != '{' {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	return obj
}

func numPtr(raw json.RawMessage) *float64 {
	value, ok := parseNumber(raw)
	if !ok {
		return nil
	}
	return &value
}

func parseNumber(raw json.RawMessage) (float64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, false
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number, true
	}
	text := unquote(raw)
	if text == "" {
		return 0, false
	}
	number, err := strconv.ParseFloat(text, 64)
	return number, err == nil
}

func unquote(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

func cleanQuant(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "unknown") || strings.EqualFold(s, "null") {
		return ""
	}
	return s
}
