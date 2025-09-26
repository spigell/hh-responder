package gemini

import (
	"context"
	"strings"
	"testing"

	"github.com/spigell/hh-responder/internal/headhunter"
	"go.uber.org/zap"
)

type stubGenerator struct {
	response    string
	err         error
	instruction string
	message     string
}

func (s *stubGenerator) GenerateContent(ctx context.Context, systemInstruction, message string) (string, error) {
	s.instruction = systemInstruction
	s.message = message
	if s.err != nil {
		return "", s.err
	}
	return s.response, nil
}

func TestMatcherEvaluate(t *testing.T) {
	stub := &stubGenerator{response: `{"fit": true, "score": 0.9, "reason": "Matches skills", "message": "Hello"}`}
	matcher := NewMatcher(stub, zap.NewNop(), 0.5)

	resume := &headhunter.ResumeDetails{ID: "r1", Title: "Engineer", Raw: map[string]any{"skills": []string{"Go"}}}
	vacancy := &headhunter.Vacancy{ID: "v1", Name: "Go Developer"}

	assessment, err := matcher.Evaluate(context.Background(), resume, vacancy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !assessment.Fit {
		t.Fatalf("expected fit to be true")
	}

	if assessment.Score != 0.9 {
		t.Fatalf("expected score 0.9, got %v", assessment.Score)
	}

	if assessment.Message != "Hello" {
		t.Fatalf("unexpected message: %s", assessment.Message)
	}

	if assessment.Reason == "" {
		t.Fatalf("expected reason to be populated")
	}

	if stub.instruction == "" || stub.message == "" {
		t.Fatalf("expected system instruction and message to be sent")
	}

	if !strings.Contains(stub.instruction, "Resume") || !strings.Contains(stub.instruction, "skills") {
		t.Fatalf("expected resume data in system instruction: %s", stub.instruction)
	}

	if !strings.Contains(stub.message, "Vacancy") || !strings.Contains(stub.message, "Go Developer") {
		t.Fatalf("expected vacancy data in chat message: %s", stub.message)
	}
}

func TestMatcherEvaluateAppliesThreshold(t *testing.T) {
	stub := &stubGenerator{response: `{"fit": true, "score": 0.3, "reason": "Too junior", "message": "Hello"}`}
	matcher := NewMatcher(stub, zap.NewNop(), 0.5)

	resume := &headhunter.ResumeDetails{ID: "r1", Title: "Engineer", Raw: map[string]any{"skills": []string{"Go"}}}
	vacancy := &headhunter.Vacancy{ID: "v1", Name: "Go Developer"}

	assessment, err := matcher.Evaluate(context.Background(), resume, vacancy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if assessment.Fit {
		t.Fatalf("expected fit to be false due to threshold")
	}
}

func TestParseResponseHandlesCodeBlock(t *testing.T) {
	raw := "```json\n{\"fit\": true, \"score\": \"0.8\", \"reason\": \"Looks good\", \"message\": \"Hi\"}\n```"
	assessment, err := parseResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !assessment.Fit {
		t.Fatalf("expected fit true")
	}

	if assessment.Score != 0.8 {
		t.Fatalf("expected score 0.8, got %v", assessment.Score)
	}

	if assessment.Message != "Hi" {
		t.Fatalf("unexpected message: %s", assessment.Message)
	}
}
