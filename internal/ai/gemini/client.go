package gemini

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

const (
	defaultModel    = "gemini-2.5-pro"
)

// Generator wraps the Google GenAI client to provide simple prompt-based interactions.
type Generator struct {
	client    *genai.Client
	modelName string
}

// NewGenerator creates a new Generator configured for the Gemini API backend.
func NewGenerator(ctx context.Context, apiKey, model string) (*Generator, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("gemini api key is required")
	}

	cfg := &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	}

	client, err := genai.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create genai client: %w", err)
	}

	if model = strings.TrimSpace(model); model == "" {
		model = defaultModel
	}

	return &Generator{client: client, modelName: model}, nil
}

// GenerateContent sends the prompt to Gemini and returns the first textual response.
func (g *Generator) GenerateContent(ctx context.Context, prompt string) (string, error) {
	if g == nil || g.client == nil {
		return "", errors.New("gemini generator is not initialized")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("prompt must not be empty")
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.modelName, genai.Text(prompt), nil)
	if err != nil {
		return "", fmt.Errorf("generate content: %w", err)
	}

	var builder strings.Builder
	for _, candidate := range resp.Candidates {
		if candidate == nil || candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			if part == nil {
				continue
			}
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(text)
		}
	}

	output := strings.TrimSpace(builder.String())
	if output == "" {
		return "", errors.New("gemini api returned empty response")
	}

	return output, nil
}

func (g *Generator) Model() string {
	if g == nil {
		return ""
	}
	return g.modelName
}
