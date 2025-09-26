package gemini

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/genai"
)

type fakeModels struct {
	mu    sync.Mutex
	calls []callRecord
	queue map[string][]fakeResponse
}

type fakeResponse struct {
	resp *genai.GenerateContentResponse
	err  error
}

type callRecord struct {
	model string
}

func newFakeModels() *fakeModels {
	return &fakeModels{queue: make(map[string][]fakeResponse)}
}

func (f *fakeModels) enqueue(model string, resp *genai.GenerateContentResponse, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue[model] = append(f.queue[model], fakeResponse{resp: resp, err: err})
}

func (f *fakeModels) GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, callRecord{model: model})
	responses := f.queue[model]
	if len(responses) == 0 {
		return nil, errors.New("unexpected call")
	}
	res := responses[0]
	f.queue[model] = responses[1:]
	return res.resp, res.err
}

func TestGeneratorRetriesOnTemporaryError(t *testing.T) {
	originalSleep := sleep
	sleep = func(time.Duration) {}
	defer func() { sleep = originalSleep }()

	models := newFakeModels()
	tempErr := genai.APIError{Code: http.StatusInternalServerError, Status: "INTERNAL"}
	models.enqueue("gemini-pro", nil, tempErr)
	models.enqueue("gemini-pro", &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{Parts: []*genai.Part{{Text: "retry ok"}}},
		}},
	}, nil)

	g := &Generator{
		models:     models,
		model:      "gemini-pro",
		maxRetries: 2,
		logger:     zap.NewNop(),
	}

	output, err := g.GenerateContent(context.Background(), " say hi ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if output != "retry ok" {
		t.Fatalf("unexpected output: %q", output)
	}

	if len(models.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(models.calls))
	}
}

func TestGeneratorStopsAfterRetriesExhausted(t *testing.T) {
	originalSleep := sleep
	sleep = func(time.Duration) {}
	defer func() { sleep = originalSleep }()

	models := newFakeModels()
	tempErr := genai.APIError{Code: http.StatusInternalServerError, Status: "INTERNAL"}
	models.enqueue("gemini-pro", nil, tempErr)
	models.enqueue("gemini-pro", nil, tempErr)

	g := &Generator{
		models:     models,
		model:      "gemini-pro",
		maxRetries: 2,
		logger:     zap.NewNop(),
	}

	_, err := g.GenerateContent(context.Background(), " say hi ")
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}

	if len(models.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(models.calls))
	}
}

func TestGeneratorDoesNotRetryOnLongQuotaDelay(t *testing.T) {
	models := newFakeModels()
	quotaErr := genai.APIError{
		Code:    http.StatusTooManyRequests,
		Status:  "RESOURCE_EXHAUSTED",
		Message: "quota exhausted, retry after 60 seconds",
	}
	models.enqueue("gemini-pro", nil, quotaErr)

	g := &Generator{
		models:     models,
		model:      "gemini-pro",
		maxRetries: 3,
		logger:     zap.NewNop(),
	}

	_, err := g.GenerateContent(context.Background(), " say hi ")
	if err == nil {
		t.Fatal("expected error when quota delay too long")
	}

	if len(models.calls) != 1 {
		t.Fatalf("expected single call, got %d", len(models.calls))
	}
}
