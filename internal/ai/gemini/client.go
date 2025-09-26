package gemini

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
)

const (
	defaultModel = "gemini-2.5-pro"
)

// Generator wraps the Google GenAI client to provide simple prompt-based interactions.
type Generator struct {
	client    *genai.Client
	modelName string

	cacheMu     sync.RWMutex
	resumeCache map[string]cachedResume
}

type cachedResume struct {
	name string
	hash string
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
	return g.generateContent(ctx, prompt, nil)
}

// GenerateContentWithCache sends the prompt to Gemini and reuses the provided cached content.
func (g *Generator) GenerateContentWithCache(ctx context.Context, prompt, cacheName string) (string, error) {
	cacheName = strings.TrimSpace(cacheName)
	if cacheName == "" {
		return g.generateContent(ctx, prompt, nil)
	}

	cfg := &genai.GenerateContentConfig{CachedContent: cacheName}
	return g.generateContent(ctx, prompt, cfg)
}

// EnsureResumeCache stores the provided resume payload in a Gemini cached content resource.
func (g *Generator) EnsureResumeCache(ctx context.Context, resumeID, displayName, resumePayload string) (string, error) {
	if g == nil || g.client == nil {
		return "", errors.New("gemini generator is not initialized")
	}

	resumeID = strings.TrimSpace(resumeID)
	if resumeID == "" {
		return "", errors.New("resume id is required")
	}

	payload := strings.TrimSpace(resumePayload)
	if payload == "" {
		return "", errors.New("resume payload must not be empty")
	}

	hashBytes := sha256.Sum256([]byte(payload))
	hash := fmt.Sprintf("%x", hashBytes[:])

	g.cacheMu.RLock()
	if existing, ok := g.resumeCache[resumeID]; ok && existing.hash == hash {
		g.cacheMu.RUnlock()
		if strings.TrimSpace(existing.name) != "" {
			return existing.name, nil
		}
	} else {
		g.cacheMu.RUnlock()
	}

	g.cacheMu.Lock()
	defer g.cacheMu.Unlock()

	if g.resumeCache == nil {
		g.resumeCache = make(map[string]cachedResume)
	}

	if existing, ok := g.resumeCache[resumeID]; ok && existing.hash == hash && strings.TrimSpace(existing.name) != "" {
		return existing.name, nil
	}

	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = fmt.Sprintf("resume-%s", resumeID)
	}

	cfg := &genai.CreateCachedContentConfig{
		DisplayName: displayName,
		TTL:         24 * time.Hour,
		Contents: []*genai.Content{{
			Role: genai.RoleUser,
			Parts: []*genai.Part{{
				Text: payload,
			}},
		}},
	}

	cached, err := g.client.Caches.Create(ctx, g.modelName, cfg)
	if err != nil {
		return "", fmt.Errorf("create resume cache: %w", err)
	}

	name := strings.TrimSpace(cached.Name)
	if name == "" {
		return "", errors.New("gemini api returned empty cache name")
	}

	g.resumeCache[resumeID] = cachedResume{name: name, hash: hash}

	return name, nil
}

func (g *Generator) generateContent(ctx context.Context, prompt string, config *genai.GenerateContentConfig) (string, error) {
	if g == nil || g.client == nil {
		return "", errors.New("gemini generator is not initialized")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("prompt must not be empty")
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.modelName, genai.Text(prompt), config)
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
