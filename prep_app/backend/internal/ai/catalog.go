package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	modelsTTL         = 10 * time.Minute
	endpointsTTL      = 5 * time.Minute
	providersTTL      = time.Hour
	providersFailTTL  = 2 * time.Minute
	maxCatalogResults = 8
	maxModelPages     = 6
)

// CatalogError is a user-facing catalog failure with an HTTP status.
type CatalogError struct {
	Status int
	Msg    string
}

func (e *CatalogError) Error() string {
	if e == nil {
		return ""
	}
	return e.Msg
}

// CatalogPage is one settings search.
type CatalogPage struct {
	Query            string         `json:"query"`
	MatchCount       int            `json:"match_count"`
	Truncated        bool           `json:"truncated"`
	LatencyNeedsKey  bool           `json:"latency_needs_key"`
	DataPolicyLoaded bool           `json:"data_policy_loaded"`
	Results          []CatalogModel `json:"results"`
}

// CatalogModel is a text model and the providers that serve it.
type CatalogModel struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	ContextLength     int               `json:"context_length"`
	IntelligenceIndex *float64          `json:"intelligence_index"`
	CodingIndex       *float64          `json:"coding_index"`
	AgenticIndex      *float64          `json:"agentic_index"`
	BenchmarkLines    []string          `json:"benchmark_lines"`
	BenchmarkNote     string            `json:"benchmark_note"`
	PriceHint         string            `json:"price_hint"`
	PricingLines      []string          `json:"pricing_lines"`
	OverrideLines     []string          `json:"override_lines"`
	Providers         []CatalogProvider `json:"providers"`
	ProvidersError    string            `json:"providers_error,omitempty"`
}

// CatalogProvider is one hosted endpoint of a model.
type CatalogProvider struct {
	Name                string   `json:"name"`
	Tag                 string   `json:"tag"`
	Quantization        string   `json:"quantization,omitempty"`
	ServiceTier         string   `json:"service_tier,omitempty"`
	ContextLength       int      `json:"context_length"`
	MaxCompletionTokens *int     `json:"max_completion_tokens,omitempty"`
	Availability        string   `json:"availability"`
	UptimeLine          string   `json:"uptime_line"`
	LatencyLine         string   `json:"latency_line"`
	ThroughputLine      string   `json:"throughput_line"`
	CollectsData        *bool    `json:"collects_data"`
	DataLine            string   `json:"data_line"`
	PricingLines        []string `json:"pricing_lines"`
	OverrideLines       []string `json:"override_lines"`
	PromptPerToken      *float64 `json:"-"`
	CompletionPerToken  *float64 `json:"-"`
}

// Directory reads the live OpenRouter catalog. The API key is optional:
// without it latency and throughput come back empty.
type Directory struct {
	Key          string
	APIBase      string
	ProvidersURL string
	Referer      string
	HTTP         *http.Client
	PageLimit    int
	Now          func() time.Time

	mu            sync.Mutex
	models        []listedModel
	modelsAt      time.Time
	providers     map[string]policy
	providersAt   time.Time
	providersOK   bool
	endpointCache map[string]endpointEntry
}

type endpointEntry struct {
	at         time.Time
	providers  []CatalogProvider
	withPolicy bool
}

type listedModel struct {
	ID            string
	Name          string
	Description   string
	ContextLength int
	Intelligence  *float64
	Coding        *float64
	Agentic       *float64
	Pricing       json.RawMessage
}

var mdLink = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)

// NewDirectory talks to the public OpenRouter catalog.
// ProvidersURL points at the catalog that includes training and retention.
// /api/v1/providers does not carry those fields.
func NewDirectory(key string) *Directory {
	return &Directory{
		Key:          strings.TrimSpace(key),
		APIBase:      "https://openrouter.ai/api/v1",
		ProvidersURL: "https://openrouter.ai/api/frontend/v1/all-providers",
		Referer:      "http://localhost:8080",
		HTTP: &http.Client{
			Timeout:       25 * time.Second,
			CheckRedirect: sameHostRedirect,
		},
	}
}

// HasKey reports whether latency can be requested with an API key.
func (d *Directory) HasKey() bool {
	return d != nil && strings.TrimSpace(d.Key) != ""
}

// Search filters the cached text-model catalog and attaches provider details.
func (d *Directory) Search(ctx context.Context, query string) (CatalogPage, error) {
	query = strings.TrimSpace(query)
	if d == nil {
		return CatalogPage{}, &CatalogError{Status: http.StatusServiceUnavailable, Msg: "каталог моделей не настроен"}
	}
	models, err := d.cachedModels(ctx)
	if err != nil {
		return CatalogPage{}, err
	}
	ranked := rankModels(models, query)
	page := CatalogPage{
		Query:           query,
		MatchCount:      len(ranked),
		Truncated:       len(ranked) > maxCatalogResults,
		LatencyNeedsKey: !d.HasKey(),
		Results:         []CatalogModel{},
	}
	if page.Truncated {
		ranked = ranked[:maxCatalogResults]
	}
	policies, loaded := d.cachedPolicies(ctx)
	page.DataPolicyLoaded = loaded
	for _, model := range ranked {
		page.Results = append(page.Results, viewModel(model))
	}
	d.attachProviders(ctx, page.Results, policies, loaded)
	for i := range page.Results {
		fillModel(&page.Results[i])
	}
	return page, nil
}

// Resolve checks that the model exists and the provider tag is one of its endpoints.
// An empty provider means OpenRouter may choose. The returned name is for display.
func (d *Directory) Resolve(ctx context.Context, modelID, provider string) (string, error) {
	modelID = strings.TrimSpace(modelID)
	provider = strings.TrimSpace(provider)
	if d == nil {
		return "", &CatalogError{Status: http.StatusServiceUnavailable, Msg: "каталог моделей не настроен"}
	}
	if !validModelID(modelID) {
		return "", &CatalogError{Status: http.StatusBadRequest, Msg: "некорректный id модели"}
	}
	if provider != "" && !validProviderTag(provider) {
		return "", &CatalogError{Status: http.StatusBadRequest, Msg: "некорректный поставщик"}
	}
	policies, loaded := d.cachedPolicies(ctx)
	endpoints, err := d.endpoints(ctx, modelID, policies, loaded)
	if err != nil {
		return "", err
	}
	if provider == "" {
		return "", nil
	}
	for _, endpoint := range endpoints {
		if endpoint.Tag == provider {
			if endpoint.Name != "" {
				return endpoint.Name, nil
			}
			return provider, nil
		}
	}
	return "", &CatalogError{Status: http.StatusBadRequest, Msg: "у этой модели нет такого поставщика"}
}

func (d *Directory) attachProviders(ctx context.Context, models []CatalogModel, policies map[string]policy, withPolicy bool) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i := range models {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				models[i].ProvidersError = "Поставщики не загрузились"
				return
			}
			endpoints, err := d.endpoints(ctx, models[i].ID, policies, withPolicy)
			if err != nil {
				models[i].ProvidersError = providerErrText(err)
				return
			}
			models[i].Providers = endpoints
			var prompt, completion *float64
			hasPrices := false
			for _, endpoint := range endpoints {
				prompt = minPrice(prompt, endpoint.PromptPerToken)
				completion = minPrice(completion, endpoint.CompletionPerToken)
				if len(endpoint.PricingLines) > 0 {
					hasPrices = true
				}
			}
			if !hasPrices {
				return
			}
			models[i].PricingLines = []string{}
			models[i].OverrideLines = []string{}
			if hint := priceHint(prompt, completion); hint != "" {
				models[i].PriceHint = hint
			}
		}(i)
	}
	wg.Wait()
}

func viewModel(model listedModel) CatalogModel {
	lines, note := benchmarkView(model.Intelligence, model.Coding, model.Agentic)
	priced := formatPricing(model.Pricing)
	return CatalogModel{
		ID:                model.ID,
		Name:              model.Name,
		Description:       cleanDescription(model.Description),
		ContextLength:     model.ContextLength,
		IntelligenceIndex: model.Intelligence,
		CodingIndex:       model.Coding,
		AgenticIndex:      model.Agentic,
		BenchmarkLines:    lines,
		BenchmarkNote:     note,
		PriceHint:         priceHint(priced.prompt, priced.completion),
		PricingLines:      nonNil(priced.lines),
		OverrideLines:     nonNil(priced.overrides),
		Providers:         []CatalogProvider{},
	}
}

func fillModel(model *CatalogModel) {
	model.BenchmarkLines = nonNil(model.BenchmarkLines)
	model.PricingLines = nonNil(model.PricingLines)
	model.OverrideLines = nonNil(model.OverrideLines)
	if model.Providers == nil {
		model.Providers = []CatalogProvider{}
	}
	for i := range model.Providers {
		model.Providers[i].PricingLines = nonNil(model.Providers[i].PricingLines)
		model.Providers[i].OverrideLines = nonNil(model.Providers[i].OverrideLines)
	}
}

func nonNil(lines []string) []string {
	if lines == nil {
		return []string{}
	}
	return lines
}

func (d *Directory) cachedModels(ctx context.Context) ([]listedModel, error) {
	d.mu.Lock()
	if d.models != nil && d.now().Sub(d.modelsAt) < modelsTTL {
		out := d.models
		d.mu.Unlock()
		return out, nil
	}
	d.mu.Unlock()
	models, err := d.fetchAllModels(ctx)
	if err != nil {
		d.mu.Lock()
		stale := d.models
		d.mu.Unlock()
		if stale != nil {
			return stale, nil
		}
		return nil, err
	}
	d.mu.Lock()
	d.models = models
	d.modelsAt = d.now()
	d.mu.Unlock()
	return models, nil
}

func (d *Directory) cachedPolicies(ctx context.Context) (map[string]policy, bool) {
	d.mu.Lock()
	age := d.now().Sub(d.providersAt)
	if d.providersOK && d.providers != nil && age < providersTTL {
		out := d.providers
		d.mu.Unlock()
		return out, true
	}
	if !d.providersOK && !d.providersAt.IsZero() && age < providersFailTTL {
		d.mu.Unlock()
		return map[string]policy{}, false
	}
	d.mu.Unlock()
	policies, err := d.fetchPolicies(ctx)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.providersAt = d.now()
	if err != nil {
		log.Printf("политика данных OpenRouter: %s", redact(d.Key, clip(err.Error(), 180)))
		d.providersOK = false
		if d.providers != nil {
			return d.providers, true
		}
		return map[string]policy{}, false
	}
	d.providers = policies
	d.providersOK = true
	return policies, true
}

func (d *Directory) endpoints(ctx context.Context, id string, policies map[string]policy, withPolicy bool) ([]CatalogProvider, error) {
	d.mu.Lock()
	if d.endpointCache != nil {
		if entry, ok := d.endpointCache[id]; ok && d.now().Sub(entry.at) < endpointsTTL && (entry.withPolicy || !withPolicy) {
			out := entry.providers
			d.mu.Unlock()
			return out, nil
		}
	}
	d.mu.Unlock()
	list, err := d.fetchEndpoints(ctx, id, policies)
	if err != nil {
		return nil, err
	}
	if withPolicy || policiesKnown(list) {
		d.mu.Lock()
		if d.endpointCache == nil {
			d.endpointCache = map[string]endpointEntry{}
		}
		d.endpointCache[id] = endpointEntry{at: d.now(), providers: list, withPolicy: withPolicy || policiesKnown(list)}
		d.mu.Unlock()
	}
	return list, nil
}

func providerErrText(err error) string {
	var ce *CatalogError
	if errors.As(err, &ce) && ce.Status == http.StatusNotFound {
		return "OpenRouter не отдал поставщиков этой модели"
	}
	return "Поставщики не загрузились"
}

func policiesKnown(list []CatalogProvider) bool {
	if len(list) == 0 {
		return false
	}
	for _, provider := range list {
		if provider.CollectsData == nil {
			return false
		}
	}
	return true
}

func (d *Directory) fetchAllModels(ctx context.Context) ([]listedModel, error) {
	var all []listedModel
	seen := map[string]struct{}{}
	total := -1
	limit := d.pageLimit()
	for page := 0; page < maxModelPages; page++ {
		batch, n, err := d.fetchModelPage(ctx, page*limit)
		if err != nil {
			return nil, err
		}
		if n >= 0 {
			total = n
		}
		if len(batch) == 0 {
			break
		}
		fresh := 0
		for _, model := range batch {
			if model.ID == "" {
				continue
			}
			if _, ok := seen[model.ID]; ok {
				continue
			}
			seen[model.ID] = struct{}{}
			all = append(all, model)
			fresh++
		}
		if fresh == 0 || (total >= 0 && len(all) >= total) || len(batch) < limit {
			break
		}
	}
	return all, nil
}

func (d *Directory) fetchModelPage(ctx context.Context, offset int) ([]listedModel, int, error) {
	limit := d.pageLimit()
	endpoint := d.apiBase() + "/models?output_modalities=text&limit=" + fmt.Sprint(limit) + "&offset=" + fmt.Sprint(offset)
	body, status, err := d.get(ctx, endpoint, 8<<20)
	if err != nil {
		return nil, 0, &CatalogError{Status: http.StatusBadGateway, Msg: "OpenRouter недоступен: " + redact(d.Key, clip(err.Error(), 180))}
	}
	if status >= 300 {
		return nil, 0, httpErr(d.Key, status, body)
	}
	var payload struct {
		Data       []json.RawMessage `json:"data"`
		TotalCount int               `json:"total_count"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, 0, &CatalogError{Status: http.StatusBadGateway, Msg: "OpenRouter вернул не JSON"}
	}
	out := make([]listedModel, 0, len(payload.Data))
	for _, raw := range payload.Data {
		model, ok := parseListedModel(raw)
		if ok {
			out = append(out, model)
		}
	}
	total := payload.TotalCount
	if total == 0 {
		total = -1
	}
	return out, total, nil
}

func parseListedModel(raw json.RawMessage) (listedModel, bool) {
	var wire struct {
		ID            string          `json:"id"`
		Name          string          `json:"name"`
		Description   string          `json:"description"`
		ContextLength int             `json:"context_length"`
		Pricing       json.RawMessage `json:"pricing"`
		Benchmarks    *struct {
			ArtificialAnalysis *struct {
				Intelligence *float64 `json:"intelligence_index"`
				Coding       *float64 `json:"coding_index"`
				Agentic      *float64 `json:"agentic_index"`
			} `json:"artificial_analysis"`
		} `json:"benchmarks"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil || strings.TrimSpace(wire.ID) == "" {
		return listedModel{}, false
	}
	model := listedModel{
		ID:            strings.TrimSpace(wire.ID),
		Name:          strings.TrimSpace(wire.Name),
		Description:   wire.Description,
		ContextLength: wire.ContextLength,
		Pricing:       wire.Pricing,
	}
	if model.Name == "" {
		model.Name = model.ID
	}
	if wire.Benchmarks != nil && wire.Benchmarks.ArtificialAnalysis != nil {
		aa := wire.Benchmarks.ArtificialAnalysis
		model.Intelligence = aa.Intelligence
		model.Coding = aa.Coding
		model.Agentic = aa.Agentic
	}
	return model, true
}

func (d *Directory) fetchEndpoints(ctx context.Context, id string, policies map[string]policy) ([]CatalogProvider, error) {
	endpoint, err := endpointURL(d.apiBase(), id)
	if err != nil {
		return nil, &CatalogError{Status: http.StatusBadRequest, Msg: "некорректный id модели"}
	}
	body, status, err := d.get(ctx, endpoint, 2<<20)
	if err != nil {
		return nil, &CatalogError{Status: http.StatusBadGateway, Msg: "OpenRouter недоступен: " + redact(d.Key, clip(err.Error(), 180))}
	}
	if status == http.StatusNotFound {
		return nil, &CatalogError{Status: http.StatusNotFound, Msg: "модель не найдена в каталоге OpenRouter"}
	}
	if status >= 300 {
		return nil, httpErr(d.Key, status, body)
	}
	var payload struct {
		Data struct {
			Endpoints []json.RawMessage `json:"endpoints"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, &CatalogError{Status: http.StatusBadGateway, Msg: "OpenRouter вернул не JSON"}
	}
	out := make([]CatalogProvider, 0, len(payload.Data.Endpoints))
	for _, raw := range payload.Data.Endpoints {
		if provider, ok := parseEndpoint(raw, policies); ok {
			out = append(out, provider)
		}
	}
	return out, nil
}

func parseEndpoint(raw json.RawMessage, policies map[string]policy) (CatalogProvider, bool) {
	var wire struct {
		Name                string          `json:"name"`
		ProviderName        string          `json:"provider_name"`
		Tag                 string          `json:"tag"`
		Quantization        string          `json:"quantization"`
		ServiceTier         *string         `json:"service_tier"`
		ContextLength       int             `json:"context_length"`
		MaxCompletionTokens *int            `json:"max_completion_tokens"`
		Status              json.RawMessage `json:"status"`
		Uptime5             json.RawMessage `json:"uptime_last_5m"`
		Uptime30            json.RawMessage `json:"uptime_last_30m"`
		Uptime1d            json.RawMessage `json:"uptime_last_1d"`
		Latency             json.RawMessage `json:"latency_last_30m"`
		Throughput          json.RawMessage `json:"throughput_last_30m"`
		Pricing             json.RawMessage `json:"pricing"`
		DataPolicy          json.RawMessage `json:"data_policy"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return CatalogProvider{}, false
	}
	var camel struct {
		DataPolicy json.RawMessage `json:"dataPolicy"`
	}
	_ = json.Unmarshal(raw, &camel)
	name := strings.TrimSpace(wire.ProviderName)
	if name == "" {
		name = strings.TrimSpace(wire.Name)
	}
	tag := strings.TrimSpace(wire.Tag)
	pol := findPolicy(policies, tag, name)
	inline := parsePolicyObject(wire.DataPolicy)
	if !inline.known {
		inline = parsePolicyObject(camel.DataPolicy)
	}
	if inline.known {
		pol = inline
	}
	collects, dataLine := describePolicy(pol)
	priced := formatPricing(wire.Pricing)
	tier := ""
	if wire.ServiceTier != nil {
		tier = strings.TrimSpace(*wire.ServiceTier)
	}
	return CatalogProvider{
		Name:                name,
		Tag:                 tag,
		Quantization:        cleanQuant(wire.Quantization),
		ServiceTier:         tier,
		ContextLength:       wire.ContextLength,
		MaxCompletionTokens: wire.MaxCompletionTokens,
		Availability:        availabilityText(wire.Status),
		UptimeLine:          formatUptime(numPtr(wire.Uptime5), numPtr(wire.Uptime30), numPtr(wire.Uptime1d)),
		LatencyLine:         formatLatency(parsePercentiles(wire.Latency)),
		ThroughputLine:      formatThroughput(parsePercentiles(wire.Throughput)),
		CollectsData:        collects,
		DataLine:            dataLine,
		PricingLines:        nonNil(priced.lines),
		OverrideLines:       nonNil(priced.overrides),
		PromptPerToken:      priced.prompt,
		CompletionPerToken:  priced.completion,
	}, name != "" || tag != ""
}

func (d *Directory) fetchPolicies(ctx context.Context) (map[string]policy, error) {
	endpoint := d.providersURL()
	body, status, err := d.get(ctx, endpoint, 2<<20)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, httpErr(d.Key, status, body)
	}
	return parseProviderCatalog(body)
}

func parseProviderCatalog(body []byte) (map[string]policy, error) {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, errors.New("каталог поставщиков не JSON")
	}
	items, err := providerItems(envelope.Data)
	if err != nil {
		return nil, err
	}
	out := map[string]policy{}
	add := func(key string, pol policy) {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" || !pol.known {
			return
		}
		if _, ok := out[key]; ok {
			return
		}
		out[key] = pol
	}
	for _, raw := range items {
		var doc struct {
			Name        string          `json:"name"`
			DisplayName string          `json:"displayName"`
			DisplayAlt  string          `json:"display_name"`
			Slug        string          `json:"slug"`
			DataPolicy  json.RawMessage `json:"dataPolicy"`
			DataPolicy2 json.RawMessage `json:"data_policy"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			continue
		}
		pol := parsePolicyObject(doc.DataPolicy)
		if !pol.known {
			pol = parsePolicyObject(doc.DataPolicy2)
		}
		add(doc.Slug, pol)
		add(doc.Name, pol)
		add(doc.DisplayName, pol)
		add(doc.DisplayAlt, pol)
	}
	if len(out) == 0 {
		return nil, errors.New("каталог поставщиков пуст")
	}
	return out, nil
}

func providerItems(raw json.RawMessage) ([]json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, errors.New("каталог поставщиков пуст")
	}
	if raw[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, errors.New("каталог поставщиков не JSON")
		}
		return items, nil
	}
	var obj struct {
		Providers []json.RawMessage `json:"providers"`
		Data      []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errors.New("каталог поставщиков не JSON")
	}
	if len(obj.Providers) > 0 {
		return obj.Providers, nil
	}
	return obj.Data, nil
}

func findPolicy(policies map[string]policy, tag, name string) policy {
	if len(policies) == 0 {
		return policy{}
	}
	if pol, ok := policies[strings.ToLower(tag)]; ok {
		return pol
	}
	if i := strings.IndexByte(tag, '/'); i > 0 {
		if pol, ok := policies[strings.ToLower(tag[:i])]; ok {
			return pol
		}
	}
	if pol, ok := policies[strings.ToLower(name)]; ok {
		return pol
	}
	return policy{}
}

func rankModels(models []listedModel, query string) []listedModel {
	query = strings.ToLower(strings.TrimSpace(query))
	type item struct {
		model    listedModel
		score    int
		intel    float64
		hasIntel bool
		idx      int
	}
	var items []item
	for i, model := range models {
		score := matchScore(model, query)
		if score == 0 {
			continue
		}
		next := item{model: model, score: score, idx: i}
		if model.Intelligence != nil {
			next.intel = *model.Intelligence
			next.hasIntel = true
		}
		items = append(items, next)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		if items[i].hasIntel != items[j].hasIntel {
			return items[i].hasIntel
		}
		if items[i].intel != items[j].intel {
			return items[i].intel > items[j].intel
		}
		return items[i].idx < items[j].idx
	})
	out := make([]listedModel, len(items))
	for i := range items {
		out[i] = items[i].model
	}
	return out
}

func matchScore(model listedModel, query string) int {
	if query == "" {
		return 0
	}
	id := strings.ToLower(model.ID)
	name := strings.ToLower(model.Name)
	desc := strings.ToLower(model.Description)
	if id == query || name == query {
		return 1000
	}
	parts := strings.Fields(query)
	for _, part := range parts {
		if !strings.Contains(id, part) && !strings.Contains(name, part) && !strings.Contains(desc, part) {
			return 0
		}
	}
	switch {
	case strings.HasPrefix(id, query) || strings.HasPrefix(name, query):
		return 850
	case strings.Contains(id, query) || strings.Contains(name, query):
		return 700
	}
	idname := id + " " + name
	for _, part := range parts {
		if !strings.Contains(idname, part) {
			return 400
		}
	}
	return 650
}

func (d *Directory) get(ctx context.Context, endpoint string, limit int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	if d.Key != "" {
		req.Header.Set("Authorization", "Bearer "+d.Key)
	}
	req.Header.Set("HTTP-Referer", d.referer())
	req.Header.Set("X-Title", "prep-app")
	resp, err := d.client().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func (d *Directory) client() *http.Client {
	if d != nil && d.HTTP != nil {
		return d.HTTP
	}
	return &http.Client{Timeout: 25 * time.Second, CheckRedirect: sameHostRedirect}
}

func (d *Directory) apiBase() string {
	if d == nil || strings.TrimSpace(d.APIBase) == "" {
		return "https://openrouter.ai/api/v1"
	}
	return strings.TrimRight(d.APIBase, "/")
}

func (d *Directory) providersURL() string {
	if d == nil || strings.TrimSpace(d.ProvidersURL) == "" {
		return "https://openrouter.ai/api/frontend/v1/all-providers"
	}
	return d.ProvidersURL
}

func (d *Directory) referer() string {
	if d == nil || strings.TrimSpace(d.Referer) == "" {
		return "http://localhost:8080"
	}
	return d.Referer
}

func (d *Directory) pageLimit() int {
	if d == nil || d.PageLimit <= 0 || d.PageLimit > 1000 {
		return 1000
	}
	return d.PageLimit
}

func (d *Directory) now() time.Time {
	if d != nil && d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func endpointURL(base, id string) (string, error) {
	author, slug, ok := strings.Cut(id, "/")
	if !ok || author == "" || slug == "" || strings.Contains(slug, "/") {
		return "", errors.New("некорректный id модели")
	}
	return base + "/models/" + url.PathEscape(author) + "/" + url.PathEscape(slug) + "/endpoints", nil
}

func sameHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 {
		return errors.New("слишком много перенаправлений")
	}
	if len(via) == 0 || req.URL.Host != via[0].URL.Host {
		return errors.New("перенаправление с OpenRouter отклонено")
	}
	return nil
}

func httpErr(key string, status int, body []byte) error {
	msg := fmt.Sprintf("OpenRouter ответил HTTP %d", status)
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && strings.TrimSpace(parsed.Error.Message) != "" {
		msg += ": " + redact(key, clip(parsed.Error.Message, 180))
	}
	code := http.StatusBadGateway
	if status == http.StatusTooManyRequests {
		code = http.StatusTooManyRequests
	}
	return &CatalogError{Status: code, Msg: msg}
}

func cleanDescription(s string) string {
	s = mdLink.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.Join(strings.Fields(s), " ")
	return clipRunes(s, 320)
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
