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

type fakeChatCreator struct {
	mu    sync.Mutex
	calls []chatCallRecord
	queue map[string][]fakeChatResponse
}

type chatCallRecord struct {
	model  string
	config *genai.GenerateContentConfig
	chat   *fakeChat
}

type fakeChatResponse struct {
	resp *genai.GenerateContentResponse
	err  error
}

type fakeChat struct {
	mu       sync.Mutex
	response fakeChatResponse
	messages []string
}

func (f *fakeChat) SendMessage(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, part := range parts {
		f.messages = append(f.messages, part.Text)
	}
	return f.response.resp, f.response.err
}

func newFakeChatCreator() *fakeChatCreator {
	return &fakeChatCreator{queue: make(map[string][]fakeChatResponse)}
}

func (f *fakeChatCreator) enqueue(model string, resp *genai.GenerateContentResponse, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue[model] = append(f.queue[model], fakeChatResponse{resp: resp, err: err})
}

func (f *fakeChatCreator) Create(ctx context.Context, model string, config *genai.GenerateContentConfig, history []*genai.Content) (chatSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	responses := f.queue[model]
	if len(responses) == 0 {
		return nil, errors.New("unexpected call")
	}
	res := responses[0]
	f.queue[model] = responses[1:]
	chat := &fakeChat{response: res}
	f.calls = append(f.calls, chatCallRecord{model: model, config: config, chat: chat})
	return chat, nil
}

func TestGeneratorRetriesOnTemporaryError(t *testing.T) {
	originalSleep := sleep
	sleep = func(time.Duration) {}
	defer func() { sleep = originalSleep }()

	chats := newFakeChatCreator()
	tempErr := genai.APIError{Code: http.StatusInternalServerError, Status: "INTERNAL"}
	chats.enqueue("gemini-pro", nil, tempErr)
	chats.enqueue("gemini-pro", &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{Parts: []*genai.Part{{Text: "retry ok"}}},
		}},
	}, nil)

	g := &Generator{
		chats:      chats,
		model:      "gemini-pro",
		maxRetries: 2,
		logger:     zap.NewNop(),
	}

	output, err := g.GenerateContent(context.Background(), "system", "message")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if output != "retry ok" {
		t.Fatalf("unexpected output: %q", output)
	}

	if len(chats.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(chats.calls))
	}

	for _, call := range chats.calls {
		if call.config == nil || call.config.SystemInstruction == nil {
			t.Fatalf("expected system instruction to be set")
		}
		if got := call.config.SystemInstruction.Parts[0].Text; got != "system" {
			t.Fatalf("unexpected system instruction: %q", got)
		}
		if len(call.chat.messages) != 1 || call.chat.messages[0] != "message" {
			t.Fatalf("unexpected chat message: %+v", call.chat.messages)
		}
	}
}

func TestGeneratorStopsAfterRetriesExhausted(t *testing.T) {
	originalSleep := sleep
	sleep = func(time.Duration) {}
	defer func() { sleep = originalSleep }()

	chats := newFakeChatCreator()
	tempErr := genai.APIError{Code: http.StatusInternalServerError, Status: "INTERNAL"}
	chats.enqueue("gemini-pro", nil, tempErr)
	chats.enqueue("gemini-pro", nil, tempErr)

	g := &Generator{
		chats:      chats,
		model:      "gemini-pro",
		maxRetries: 2,
		logger:     zap.NewNop(),
	}

	_, err := g.GenerateContent(context.Background(), "sys", "msg")
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}

	if len(chats.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(chats.calls))
	}
}

func TestGeneratorDoesNotRetryOnLongQuotaDelay(t *testing.T) {
	chats := newFakeChatCreator()
	quotaErr := genai.APIError{
		Code:    http.StatusTooManyRequests,
		Status:  "RESOURCE_EXHAUSTED",
		Message: "quota exhausted, retry after 60 seconds",
	}
	chats.enqueue("gemini-pro", nil, quotaErr)

	g := &Generator{
		chats:      chats,
		model:      "gemini-pro",
		maxRetries: 3,
		logger:     zap.NewNop(),
	}

	_, err := g.GenerateContent(context.Background(), "sys", "msg")
	if err == nil {
		t.Fatal("expected error when quota delay too long")
	}

	if len(chats.calls) != 1 {
		t.Fatalf("expected single call, got %d", len(chats.calls))
	}
}
