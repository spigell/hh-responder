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
	mu        sync.Mutex
	responses []fakeChatResponse
	messages  []string
}

func (f *fakeChat) SendMessage(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, part := range parts {
		f.messages = append(f.messages, part.Text)
	}
	if len(f.responses) == 0 {
		return nil, errors.New("unexpected send call")
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response.resp, response.err
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
	chat := &fakeChat{responses: append([]fakeChatResponse(nil), responses...)}
	delete(f.queue, model)
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

	if len(chats.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(chats.calls))
	}

	call := chats.calls[0]
	if call.config == nil || call.config.SystemInstruction == nil {
		t.Fatalf("expected system instruction to be set")
	}
	if got := call.config.SystemInstruction.Parts[0].Text; got != "system" {
		t.Fatalf("unexpected system instruction: %q", got)
	}
	if len(call.chat.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(call.chat.messages))
	}
	for i, msg := range call.chat.messages {
		if msg != "message" {
			t.Fatalf("unexpected message %d: %q", i, msg)
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

	if len(chats.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(chats.calls))
	}
	if got := len(chats.calls[0].chat.messages); got != 2 {
		t.Fatalf("expected 2 messages, got %d", got)
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
	if got := len(chats.calls[0].chat.messages); got != 1 {
		t.Fatalf("expected single message attempt, got %d", got)
	}
}

func TestGeneratorReusesChatAcrossCalls(t *testing.T) {
	chats := newFakeChatCreator()
	chats.enqueue("gemini-pro", &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{Parts: []*genai.Part{{Text: "first"}}},
		}},
	}, nil)
	chats.enqueue("gemini-pro", &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{Parts: []*genai.Part{{Text: "second"}}},
		}},
	}, nil)

	g := &Generator{
		chats:      chats,
		model:      "gemini-pro",
		maxRetries: 2,
		logger:     zap.NewNop(),
	}

	first, err := g.GenerateContent(context.Background(), "system", "vacancy-1")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if first != "first" {
		t.Fatalf("unexpected first response: %q", first)
	}

	second, err := g.GenerateContent(context.Background(), "system", "vacancy-2")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if second != "second" {
		t.Fatalf("unexpected second response: %q", second)
	}

	if len(chats.calls) != 1 {
		t.Fatalf("expected a single chat creation, got %d", len(chats.calls))
	}

	messages := chats.calls[0].chat.messages
	if len(messages) != 2 {
		t.Fatalf("expected two messages, got %d", len(messages))
	}
	if messages[0] != "vacancy-1" || messages[1] != "vacancy-2" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
}

func TestGeneratorRejectsSystemInstructionChange(t *testing.T) {
	chats := newFakeChatCreator()
	chats.enqueue("gemini-pro", &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{Parts: []*genai.Part{{Text: "ok"}}},
		}},
	}, nil)

	g := &Generator{
		chats:      chats,
		model:      "gemini-pro",
		maxRetries: 2,
		logger:     zap.NewNop(),
	}

	if _, err := g.GenerateContent(context.Background(), "system-a", "message-1"); err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	_, err := g.GenerateContent(context.Background(), "system-b", "message-2")
	if err == nil {
		t.Fatal("expected error when system instruction changes")
	}

	if len(chats.calls) != 1 {
		t.Fatalf("expected a single chat creation, got %d", len(chats.calls))
	}
	if len(chats.calls[0].chat.messages) != 1 {
		t.Fatalf("expected one message sent, got %d", len(chats.calls[0].chat.messages))
	}
}
