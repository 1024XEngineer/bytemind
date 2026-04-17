package provider

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"bytemind/internal/config"
	"bytemind/internal/llm"
)

type stubHealthChecker struct {
	errors map[ProviderID]error
	calls  map[ProviderID]int
}

func (s stubHealthChecker) Check(_ context.Context, id ProviderID) error {
	if s.calls != nil {
		s.calls[id]++
	}
	if s.errors == nil {
		return nil
	}
	return s.errors[id]
}

type stubRouterClient struct {
	providerID ProviderID
	models     []ModelInfo
	modelsErr  error
	streamErr  error
	streams    []stubRouterStreamResult
	streamReqs []llm.ChatRequest
}

type stubRouterStreamResult struct {
	message     llm.Message
	err         error
	deltas      []string
	events      []Event
	skipAutoEnd bool
}

func (s *stubRouterClient) ProviderID() ProviderID { return s.providerID }
func (s *stubRouterClient) ListModels(context.Context) ([]ModelInfo, error) {
	return s.models, s.modelsErr
}
func (s *stubRouterClient) Stream(_ context.Context, req Request) (<-chan Event, error) {
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	idx := len(s.streamReqs)
	s.streamReqs = append(s.streamReqs, req.ChatRequest)
	result := stubRouterStreamResult{}
	if idx < len(s.streams) {
		result = s.streams[idx]
	}
	ch := make(chan Event, len(result.deltas)+len(result.events)+3)
	go func() {
		defer close(ch)
		ch <- Event{Type: EventStart, ProviderID: s.providerID, ModelID: ModelID(req.Model), TraceID: req.TraceID}
		for _, delta := range result.deltas {
			ch <- Event{Type: EventDelta, ProviderID: s.providerID, ModelID: ModelID(req.Model), TraceID: req.TraceID, Delta: delta}
		}
		for _, event := range result.events {
			ch <- event
		}
		if result.skipAutoEnd {
			return
		}
		if result.err != nil {
			var providerErr *Error
			if errors.As(result.err, &providerErr) {
				ch <- Event{Type: EventError, ProviderID: s.providerID, ModelID: ModelID(req.Model), TraceID: req.TraceID, Error: providerErr}
				return
			}
			ch <- Event{Type: EventError, ProviderID: s.providerID, ModelID: ModelID(req.Model), TraceID: req.TraceID, Error: &Error{Code: ErrCodeUnavailable, Provider: s.providerID, Message: "provider unavailable", Retryable: true, Err: result.err, Detail: result.err.Error()}}
			return
		}
		message := result.message
		message.Normalize()
		ch <- Event{Type: EventResult, ProviderID: s.providerID, ModelID: ModelID(req.Model), TraceID: req.TraceID, Result: &message}
	}()
	return ch, nil
}

type stubRouter struct {
	result RouteResult
	err    error
}

func (s stubRouter) Route(context.Context, ModelID, RouteContext) (RouteResult, error) {
	return s.result, s.err
}

type stubRegistry struct {
	ids     []ProviderID
	listErr error
	clients map[ProviderID]Client
}

func (s stubRegistry) Register(context.Context, Client) error { return nil }
func (s stubRegistry) Get(_ context.Context, id ProviderID) (Client, bool) {
	client, ok := s.clients[id]
	return client, ok
}
func (s stubRegistry) List(context.Context) ([]ProviderID, error) { return s.ids, s.listErr }

func TestRouterRoutesRequestedModelWithFallbacks(t *testing.T) {
	reg, _ := NewRegistryFromProviderConfig(config.ProviderConfig{Type: "openai-compatible", BaseURL: "https://api.openai.com/v1", APIKey: "key", Model: "gpt-5.4"})
	_ = reg.Register(context.Background(), &stubRouterClient{providerID: "backup", models: []ModelInfo{{ProviderID: "backup", ModelID: "gpt-5.4"}}})
	router := NewRouter(reg, nil, RouterConfig{DefaultProvider: ProviderOpenAI, DefaultModel: "gpt-5.4"})
	result, err := router.Route(context.Background(), "gpt-5.4", RouteContext{AllowFallback: true})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Primary.ProviderID != ProviderOpenAI || result.Primary.ModelID != "gpt-5.4" {
		t.Fatalf("unexpected primary %#v", result.Primary)
	}
	if len(result.Fallbacks) != 1 || result.Fallbacks[0].ProviderID != "backup" {
		t.Fatalf("unexpected fallbacks %#v", result.Fallbacks)
	}
}

func TestRouterFiltersUnhealthyProviders(t *testing.T) {
	reg, _ := NewRegistry(config.ProviderRuntimeConfig{})
	_ = reg.Register(context.Background(), &stubRouterClient{providerID: "openai", models: []ModelInfo{{ProviderID: "openai", ModelID: "gpt-5.4"}}})
	_ = reg.Register(context.Background(), &stubRouterClient{providerID: "backup", models: []ModelInfo{{ProviderID: "backup", ModelID: "gpt-5.4"}}})
	router := NewRouter(reg, stubHealthChecker{errors: map[ProviderID]error{"openai": errors.New("down")}}, RouterConfig{})
	result, err := router.Route(context.Background(), "gpt-5.4", RouteContext{AllowFallback: true})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Primary.ProviderID != "backup" {
		t.Fatalf("unexpected primary %#v", result.Primary)
	}
}

func TestRouterReturnsUnavailableWithoutCandidates(t *testing.T) {
	reg, _ := NewRegistry(config.ProviderRuntimeConfig{})
	router := NewRouter(reg, nil, RouterConfig{})
	_, err := router.Route(context.Background(), "missing", RouteContext{AllowFallback: true})
	var providerErr *Error
	if !errors.As(err, &providerErr) || providerErr.Code != ErrCodeUnavailable {
		t.Fatalf("unexpected error %#v", err)
	}
}

func TestRoutedClientFallsBackOnRetryableProviderError(t *testing.T) {
	primary := &stubRouterClient{providerID: "openai", models: []ModelInfo{{ProviderID: "openai", ModelID: "gpt-5.4"}}, streams: []stubRouterStreamResult{{message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}}, {err: &Error{Code: ErrCodeRateLimited, Provider: "openai", Message: "rate limited", Retryable: true}, deltas: []string{"bad"}}}}
	fallback := &stubRouterClient{providerID: "backup", models: []ModelInfo{{ProviderID: "backup", ModelID: "gpt-5.4"}}, streams: []stubRouterStreamResult{{message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}, deltas: []string{"o", "k"}}}}
	reg, _ := NewRegistry(config.ProviderRuntimeConfig{})
	_ = reg.Register(context.Background(), primary)
	_ = reg.Register(context.Background(), fallback)
	client := NewRoutedClientWithPolicy(NewRouter(reg, nil, RouterConfig{DefaultProvider: "openai"}), true)
	msg, err := client.CreateMessage(context.Background(), llm.ChatRequest{Model: "gpt-5.4"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if msg.Content != "ok" {
		t.Fatalf("unexpected message %#v", msg)
	}
	var streamed []string
	_, err = client.StreamMessage(context.Background(), llm.ChatRequest{Model: "gpt-5.4"}, func(delta string) { streamed = append(streamed, delta) })
	var providerErr *Error
	if !errors.As(err, &providerErr) || providerErr.Code != ErrCodeRateLimited || !providerErr.Retryable {
		t.Fatalf("expected streamed attempt to stop on retryable primary error after delta, got %#v", err)
	}
	if strings.Join(streamed, "") != "bad" {
		t.Fatalf("expected only primary streamed delta before stopping, got %#v", streamed)
	}
	if len(primary.streamReqs) != 2 || len(fallback.streamReqs) != 0 {
		t.Fatalf("unexpected request counts primary=%d fallback=%d", len(primary.streamReqs), len(fallback.streamReqs))
	}
}

func TestRoutedClientStopsOnNonRetryableProviderError(t *testing.T) {
	primary := &stubRouterClient{providerID: "openai", models: []ModelInfo{{ProviderID: "openai", ModelID: "gpt-5.4"}}, streams: []stubRouterStreamResult{{err: &Error{Code: ErrCodeBadRequest, Provider: "openai", Message: "bad request", Retryable: true}}}}
	fallback := &stubRouterClient{providerID: "backup", models: []ModelInfo{{ProviderID: "backup", ModelID: "gpt-5.4"}}, streams: []stubRouterStreamResult{{message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}}}}
	reg, _ := NewRegistry(config.ProviderRuntimeConfig{})
	_ = reg.Register(context.Background(), primary)
	_ = reg.Register(context.Background(), fallback)
	client := NewRoutedClientWithPolicy(NewRouter(reg, nil, RouterConfig{DefaultProvider: "openai"}), true)
	_, err := client.CreateMessage(context.Background(), llm.ChatRequest{Model: "gpt-5.4"})
	var providerErr *Error
	if !errors.As(err, &providerErr) || providerErr.Code != ErrCodeBadRequest || providerErr.Retryable {
		t.Fatalf("unexpected error %#v", err)
	}
	if len(fallback.streamReqs) != 0 {
		t.Fatalf("expected fallback to be skipped, got %d calls", len(fallback.streamReqs))
	}
}

func TestRouterNoFallbacksWhenDisabled(t *testing.T) {
	reg := stubRegistry{ids: []ProviderID{"openai", "backup"}, clients: map[ProviderID]Client{
		"openai": &stubRouterClient{providerID: "openai", models: []ModelInfo{{ModelID: "gpt-5.4"}}},
		"backup": &stubRouterClient{providerID: "backup", models: []ModelInfo{{ModelID: "gpt-5.4"}}},
	}}
	result, err := NewRouter(reg, nil, RouterConfig{}).Route(context.Background(), "gpt-5.4", RouteContext{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result.Fallbacks) != 0 {
		t.Fatalf("expected no fallbacks, got %#v", result.Fallbacks)
	}
}

func TestRouterCollectCandidatesSkipsInvalidEntries(t *testing.T) {
	reg := stubRegistry{ids: []ProviderID{"missing", "blank", "broken", "dup", "valid"}, clients: map[ProviderID]Client{
		"blank":  &stubRouterClient{providerID: "   ", models: []ModelInfo{{ModelID: "gpt-5.4"}}},
		"broken": &stubRouterClient{providerID: "broken", modelsErr: errors.New("boom")},
		"dup":    &stubRouterClient{providerID: "dup", models: []ModelInfo{{ModelID: "gpt-5.4"}, {ModelID: "gpt-5.4"}, {ModelID: "   "}}},
		"valid":  &stubRouterClient{providerID: "valid", models: []ModelInfo{{ModelID: "gpt-4.1"}}},
	}}
	var buf bytes.Buffer
	prevWriter := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prevWriter)
	candidates, err := (&registryRouter{registry: reg}).collectCandidates(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("unexpected candidates %#v", candidates)
	}
	logged := buf.String()
	if !strings.Contains(logged, "provider=broken") || !strings.Contains(logged, "provider_list_models_failed") {
		t.Fatalf("expected warning log, got %q", logged)
	}
	if strings.Contains(logged, "boom") {
		t.Fatalf("expected warning log to omit raw upstream error, got %q", logged)
	}
}

func TestRouterCollectCandidatesPropagatesListError(t *testing.T) {
	_, err := (&registryRouter{registry: stubRegistry{listErr: errors.New("boom")}}).collectCandidates(context.Background())
	if err == nil {
		t.Fatal("expected list error")
	}
}

func TestRouterRouteHandlesNilRegistryAndNoHealthyCandidates(t *testing.T) {
	if _, err := (*registryRouter)(nil).Route(context.Background(), "gpt-5.4", RouteContext{}); err == nil {
		t.Fatal("expected nil router error")
	}
	reg := stubRegistry{ids: []ProviderID{"openai"}, clients: map[ProviderID]Client{"openai": &stubRouterClient{providerID: "openai", models: []ModelInfo{{ModelID: "gpt-5.4"}}}}}
	_, err := NewRouter(reg, stubHealthChecker{errors: map[ProviderID]error{"openai": errors.New("down")}}, RouterConfig{}).Route(context.Background(), "gpt-5.4", RouteContext{AllowFallback: true})
	if err == nil {
		t.Fatal("expected unavailable error")
	}
	reg = stubRegistry{ids: []ProviderID{"openai"}, clients: map[ProviderID]Client{"openai": &stubRouterClient{providerID: "openai", modelsErr: context.Canceled}}}
	_, err = NewRouter(reg, nil, RouterConfig{}).Route(context.Background(), "gpt-5.4", RouteContext{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected list models cancellation to propagate, got %v", err)
	}
	reg = stubRegistry{ids: []ProviderID{"openai"}, clients: map[ProviderID]Client{"openai": &stubRouterClient{providerID: "openai", models: []ModelInfo{{ModelID: "gpt-5.4"}}}}}
	_, err = NewRouter(reg, stubHealthChecker{errors: map[ProviderID]error{"openai": context.Canceled}}, RouterConfig{}).Route(context.Background(), "gpt-5.4", RouteContext{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected health check cancellation to propagate, got %v", err)
	}
}

func TestFilterHealthyCandidatesChecksEachProviderOnce(t *testing.T) {
	health := stubHealthChecker{calls: map[ProviderID]int{}, errors: map[ProviderID]error{"backup": errors.New("down")}}
	candidates := []routeCandidate{{ProviderID: "openai", ModelID: "gpt-5.4"}, {ProviderID: "openai", ModelID: "gpt-4.1"}, {ProviderID: "backup", ModelID: "gpt-5.4"}, {ProviderID: "backup", ModelID: "gpt-4.1"}}
	filtered, err := filterHealthyCandidates(context.Background(), health, candidates)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("unexpected filtered candidates %#v", filtered)
	}
	if health.calls["openai"] != 1 || health.calls["backup"] != 1 {
		t.Fatalf("expected one health check per provider, got %#v", health.calls)
	}
}

func TestRouteHelpersAndPolicyBranches(t *testing.T) {
	if normalizeRouteProviderID(" OpenAI ") != "openai" || normalizeRouteModelID(" gpt-5.4 ") != "gpt-5.4" {
		t.Fatal("expected normalization to trim values")
	}
	rc := normalizeRouteContext(RouteContext{Scenario: " chat ", Region: " us ", Tags: nil})
	if rc.Scenario != "chat" || rc.Region != "us" || rc.Tags == nil {
		t.Fatalf("unexpected context %#v", rc)
	}
	if toRouteTarget(routeCandidate{ProviderID: "openai", ModelID: "gpt-5.4"}).ProviderID != "openai" {
		t.Fatal("expected target conversion")
	}
	if got := filterCandidatesByModel([]routeCandidate{{ProviderID: "a", ModelID: "m1"}, {ProviderID: "b", ModelID: "m2"}}, ""); len(got) != 2 {
		t.Fatalf("unexpected candidates %#v", got)
	}
	if got, err := filterHealthyCandidates(context.Background(), nil, []routeCandidate{{ProviderID: "a"}}); err != nil || len(got) != 1 {
		t.Fatalf("unexpected healthy candidates %#v err=%v", got, err)
	}
	ordered := sortRouteCandidates([]routeCandidate{{ProviderID: "anthropic", ModelID: "claude-3"}, {ProviderID: "openai", ModelID: "gpt-5.4"}, {ProviderID: "zeta", ModelID: "z1"}}, "gpt-5.4", RouteContext{PreferLatency: true}, RouterConfig{})
	if ordered[0].ProviderID != "openai" {
		t.Fatalf("unexpected order %#v", ordered)
	}
	ordered = sortRouteCandidates([]routeCandidate{{ProviderID: "openai", ModelID: "gpt-5.4"}, {ProviderID: "anthropic", ModelID: "claude-3"}}, "", RouteContext{PreferLowCost: true}, RouterConfig{})
	if ordered[0].ProviderID != "anthropic" {
		t.Fatalf("unexpected cost order %#v", ordered)
	}
	ordered = sortRouteCandidates([]routeCandidate{{ProviderID: "beta", ModelID: "m2"}, {ProviderID: "alpha", ModelID: "m1"}}, "", RouteContext{}, RouterConfig{})
	if ordered[0].ProviderID != "alpha" {
		t.Fatalf("unexpected lexical order %#v", ordered)
	}
	ordered = sortRouteCandidates([]routeCandidate{{ProviderID: "backup", ModelID: "m1"}, {ProviderID: "openai", ModelID: "m2"}}, "", RouteContext{}, RouterConfig{DefaultProvider: "openai"})
	if ordered[0].ProviderID != "openai" {
		t.Fatalf("unexpected default provider order %#v", ordered)
	}
	ordered = sortRouteCandidates([]routeCandidate{{ProviderID: "backup", ModelID: "m1"}, {ProviderID: "backup", ModelID: "m2"}}, "", RouteContext{}, RouterConfig{DefaultModel: "m2"})
	if ordered[0].ModelID != "m2" {
		t.Fatalf("unexpected default model order %#v", ordered)
	}
	ordered = sortRouteCandidates([]routeCandidate{{ProviderID: "openai", ModelID: "gpt-5.4"}, {ProviderID: "backup", ModelID: "gpt-5.4"}}, "gpt-5.4", RouteContext{Tags: map[string]string{"provider": " backup "}}, RouterConfig{})
	if ordered[0].ProviderID != "backup" {
		t.Fatalf("unexpected tagged provider order %#v", ordered)
	}
	if preferredRouteProvider("claude-3", RouteContext{}) != ProviderAnthropic {
		t.Fatal("expected anthropic preference")
	}
	if preferredRouteProvider("gpt-5.4", RouteContext{}) != ProviderOpenAI {
		t.Fatal("expected openai preference")
	}
	if preferredRouteProvider("", RouteContext{Tags: map[string]string{"provider": " backup "}}) != "backup" {
		t.Fatal("expected tag provider preference")
	}
	if preferredRouteProvider("custom", RouteContext{}) != "" {
		t.Fatal("expected no preferred provider")
	}
	if routeRankLatency("openai") >= routeRankLatency("anthropic") {
		t.Fatal("expected openai latency rank ahead")
	}
	if routeRankCost("anthropic") >= routeRankCost("openai") {
		t.Fatal("expected anthropic cost rank ahead")
	}
	if routeRankLatency("other") != 10 || routeRankCost("other") != 10 {
		t.Fatal("expected default ranks")
	}
	if !isAnthropicModel("claude-3") || isAnthropicModel("gpt-5") {
		t.Fatal("unexpected anthropic model detection")
	}
	if !isOpenAIModel("gpt-5") || !isOpenAIModel("o1-mini") || isOpenAIModel("claude-3") {
		t.Fatal("unexpected openai model detection")
	}
}

func TestUnavailableRouteError(t *testing.T) {
	err := unavailableRouteError("no candidates")
	if err.Code != ErrCodeUnavailable || !err.Retryable || err.Message != "no candidates" {
		t.Fatalf("unexpected error %#v", err)
	}
	if errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("unexpected provider-not-found unwrap %#v", err)
	}
}

func TestNewRoutedClientAndExecuteBranches(t *testing.T) {
	if NewRoutedClient(nil) != nil {
		t.Fatal("expected nil routed client for nil router")
	}
	var client *RoutedClient
	if _, err := client.CreateMessage(context.Background(), llm.ChatRequest{}); err == nil {
		t.Fatal("expected unavailable error")
	}
	client = &RoutedClient{router: stubRouter{err: errors.New("route failed")}}
	if _, err := client.CreateMessage(context.Background(), llm.ChatRequest{}); err == nil {
		t.Fatal("expected route error")
	}
	client = &RoutedClient{router: stubRouter{result: RouteResult{Primary: RouteTarget{ProviderID: "openai", ModelID: "gpt-5.4"}}}}
	if _, err := client.CreateMessage(context.Background(), llm.ChatRequest{Model: "gpt-5.4"}); err == nil {
		t.Fatal("expected unavailable when target client missing")
	}
	primary := &stubRouterClient{providerID: "openai", models: []ModelInfo{{ProviderID: "openai", ModelID: "gpt-5.4"}}, streams: []stubRouterStreamResult{{err: &Error{Code: ErrCodeRateLimited, Provider: "openai", Message: "rate limited", Retryable: true}}}}
	fallback := &stubRouterClient{providerID: "backup", models: []ModelInfo{{ProviderID: "backup", ModelID: "gpt-5.4"}}, streams: []stubRouterStreamResult{{message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}}}}
	reg, _ := NewRegistry(config.ProviderRuntimeConfig{})
	_ = reg.Register(context.Background(), primary)
	_ = reg.Register(context.Background(), fallback)
	client = NewRoutedClient(NewRouter(reg, nil, RouterConfig{DefaultProvider: "openai"})).(*RoutedClient)
	if _, err := client.CreateMessage(context.Background(), llm.ChatRequest{Model: "gpt-5.4"}); err == nil {
		t.Fatal("expected fallback disabled error")
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewRoutedClientWithPolicy(NewRouter(reg, nil, RouterConfig{DefaultProvider: "openai"}), true).CreateMessage(cancelCtx, llm.ChatRequest{Model: "gpt-5.4"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if len(fallback.streamReqs) != 0 {
		t.Fatalf("expected canceled request to skip fallback, got %d fallback calls", len(fallback.streamReqs))
	}
}

func TestExecuteTargetCoversBranches(t *testing.T) {
	client := &stubRouterClient{providerID: "openai", streamErr: errors.New("stream setup failed")}
	if _, err := executeTarget(context.Background(), RouteTarget{ProviderID: "openai", ModelID: "gpt-5.4", Client: client}, Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}}, false, nil); err == nil {
		t.Fatal("expected stream setup error")
	}
	var streamed []string
	tool := llm.ToolCall{ID: "1", Type: "function", Function: llm.ToolFunctionCall{Name: "ls", Arguments: "{}"}}
	client = &stubRouterClient{providerID: "openai", streams: []stubRouterStreamResult{{events: []Event{{Type: EventResult, Result: &llm.Message{Role: llm.RoleAssistant}}}, deltas: []string{"o", "k"}}}}
	msg, err := executeTarget(context.Background(), RouteTarget{ProviderID: "openai", ModelID: "gpt-5.4", Client: client}, Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}}, true, func(delta string) { streamed = append(streamed, delta) })
	if err != nil || msg.Content != "ok" || strings.Join(streamed, "") != "ok" {
		t.Fatalf("expected realtime merged delta content, got msg=%#v deltas=%#v err=%v", msg, streamed, err)
	}
	streamed = nil
	client = &stubRouterClient{providerID: "openai", streams: []stubRouterStreamResult{{events: []Event{{Type: EventToolCall, ToolCall: &tool}, {Type: EventUsage, Usage: &Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}}, {Type: EventResult, Result: &llm.Message{Role: llm.RoleAssistant}}}}}}
	msg, err = executeTarget(context.Background(), RouteTarget{ProviderID: "openai", ModelID: "gpt-5.4", Client: client}, Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}}, true, func(delta string) { streamed = append(streamed, delta) })
	if err != nil || len(msg.ToolCalls) != 1 || msg.Usage == nil || msg.Usage.TotalTokens != 3 || len(streamed) != 0 {
		t.Fatalf("expected merged metadata result, got msg=%#v deltas=%#v err=%v", msg, streamed, err)
	}
	streamed = nil
	client = &stubRouterClient{providerID: "openai", streams: []stubRouterStreamResult{{events: []Event{{Type: EventResult, Result: &llm.Message{Role: llm.RoleAssistant, Content: "ok"}}}, deltas: []string{"o", "k"}}}}
	msg, err = executeTarget(context.Background(), RouteTarget{ProviderID: "openai", ModelID: "gpt-5.4", Client: client}, Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}}, true, func(delta string) { streamed = append(streamed, delta) })
	if err != nil || msg.Content != "ok" || strings.Join(streamed, "") != "ok" {
		t.Fatalf("unexpected success result %#v deltas=%#v err=%v", msg, streamed, err)
	}
	streamed = nil
	client = &stubRouterClient{providerID: "openai", streams: []stubRouterStreamResult{{events: []Event{{Type: EventToolCall, ToolCall: &tool}, {Type: EventUsage, Usage: &Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}}}, deltas: []string{"a", ""}, skipAutoEnd: true}}}
	_, err = executeTarget(context.Background(), RouteTarget{ProviderID: "openai", ModelID: "gpt-5.4", Client: client}, Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}}, true, func(delta string) { streamed = append(streamed, delta) })
	var providerErr *Error
	if !errors.As(err, &providerErr) || providerErr.Code != ErrCodeUnavailable {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if strings.Join(streamed, "") != "a" {
		t.Fatalf("expected immediate streamed delta before failure, got %#v", streamed)
	}
	client = &stubRouterClient{providerID: "openai", streams: []stubRouterStreamResult{{events: []Event{{Type: EventError, Error: &Error{Code: ErrCodeBadRequest, Provider: "", Message: "bad", Retryable: true, Err: errors.New("raw")}}}}}}
	_, err = executeTarget(context.Background(), RouteTarget{ProviderID: "openai", ModelID: "gpt-5.4", Client: client}, Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}}, false, nil)
	if !errors.As(err, &providerErr) || providerErr.Code != ErrCodeBadRequest || providerErr.Retryable || providerErr.Provider != "openai" || providerErr.Detail != "raw" {
		t.Fatalf("expected normalized event error, got %#v", err)
	}
	client = &stubRouterClient{providerID: "openai", streams: []stubRouterStreamResult{{events: []Event{{Type: EventError, Error: &Error{Err: context.Canceled}}}}}}
	_, err = executeTarget(context.Background(), RouteTarget{ProviderID: "openai", ModelID: "gpt-5.4", Client: client}, Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}}, false, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected event error cancellation, got %v", err)
	}
	client = &stubRouterClient{providerID: "openai", streams: []stubRouterStreamResult{{events: []Event{{Type: EventError}}}}}
	_, err = executeTarget(context.Background(), RouteTarget{ProviderID: "openai", ModelID: "gpt-5.4", Client: client}, Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}}, false, nil)
	if !errors.As(err, &providerErr) || providerErr.Code != ErrCodeUnavailable {
		t.Fatalf("expected nil-payload error event to fail, got %v", err)
	}
	client = &stubRouterClient{providerID: "openai", streams: []stubRouterStreamResult{{deltas: []string{"a", "b"}, skipAutoEnd: true}}}
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = executeTarget(cancelCtx, RouteTarget{ProviderID: "openai", ModelID: "gpt-5.4", Client: client}, Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}}, false, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	client = &stubRouterClient{providerID: "openai", streams: []stubRouterStreamResult{{deltas: []string{"a", "b"}, skipAutoEnd: true}}}
	_, err = executeTarget(context.Background(), RouteTarget{ProviderID: "openai", ModelID: "gpt-5.4", Client: client}, Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}}, false, nil)
	if !errors.As(err, &providerErr) || providerErr.Code != ErrCodeUnavailable {
		t.Fatalf("expected delta termination error, got %v", err)
	}
}

func TestNewRouterClient(t *testing.T) {
	client, err := NewRouterClient(config.ProviderRuntimeConfig{DefaultProvider: "openai", DefaultModel: "gpt-5.4", AllowFallback: true, Providers: map[string]config.ProviderConfig{"openai": {Type: "openai-compatible", BaseURL: "https://api.openai.com/v1", APIKey: "key", Model: "gpt-5.4"}}}, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	routed, ok := client.(*RoutedClient)
	if !ok {
		t.Fatalf("expected routed client, got %T", client)
	}
	if !routed.allowFallback {
		t.Fatal("expected routed client fallback to be enabled")
	}
	if _, err := NewRouterClient(config.ProviderRuntimeConfig{Providers: map[string]config.ProviderConfig{"broken": {Type: ""}}}, nil); err == nil {
		t.Fatal("expected registry error")
	}
}
