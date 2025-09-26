package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	_ "embed"

	"github.com/spigell/hh-responder/internal/ai"
	"github.com/spigell/hh-responder/internal/headhunter"
	"go.uber.org/zap"
)

type contentGenerator interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
}

type cachedContentGenerator interface {
	GenerateContentWithCache(ctx context.Context, prompt, cacheName string) (string, error)
}

type resumeCacheEnsurer interface {
	EnsureResumeCache(ctx context.Context, resumeID, displayName, resumePayload string) (string, error)
}

type Matcher struct {
	generator contentGenerator
	minScore  float64
	logger    *zap.Logger
}

//go:embed prompt.md
var promptTemplate string

func NewMatcher(generator contentGenerator, logger *zap.Logger, minScore float64) *Matcher {
	return &Matcher{generator: generator, minScore: minScore, logger: logger}
}

func (m *Matcher) Evaluate(ctx context.Context, resume *headhunter.ResumeDetails, vacancy *headhunter.Vacancy) (*ai.FitAssessment, error) {
	if resume == nil {
		return nil, fmt.Errorf("resume details are required")
	}
	if vacancy == nil {
		return nil, fmt.Errorf("vacancy is required")
	}

	resumePayload := map[string]any{
		"id":      resume.ID,
		"title":   resume.Title,
		"details": resume.Raw,
	}

	resumeJSON, err := json.MarshalIndent(resumePayload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal resume payload: %w", err)
	}

	vacancyJSON, err := json.MarshalIndent(vacancy, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal vacancy payload: %w", err)
	}

	resumeSection := string(resumeJSON)
	var cacheName string
	var cacheCaller cachedContentGenerator
	if cacheCreator, ok := m.generator.(resumeCacheEnsurer); ok {
		if caller, ok := m.generator.(cachedContentGenerator); ok {
			if cachedName, err := cacheCreator.EnsureResumeCache(ctx, resume.ID, resume.Title, resumeSection); err != nil {
				m.logger.Warn("failed to cache resume for gemini", zap.String("resume_id", resume.ID), zap.Error(err))
			} else if trimmed := strings.TrimSpace(cachedName); trimmed != "" {
				cacheName = trimmed
				cacheCaller = caller
				resumeSection = fmt.Sprintf("Candidate resume JSON is stored in cached content resource %q. Use it to evaluate the candidate.", cacheName)
				m.logger.Debug("cached resume for gemini",
					zap.String("resume_id", resume.ID),
					zap.String("cache_name", cacheName),
				)
			}
		}
	}

	prompt := buildPrompt(resumeSection, string(vacancyJSON))

	var modelName, baseURL string
	if info, ok := m.generator.(interface{ Model() string }); ok {
		modelName = info.Model()
	}
	if info, ok := m.generator.(interface{ BaseURL() string }); ok {
		baseURL = info.BaseURL()
	}

	m.logger.Debug("gemini generate content request",
		zap.String("vacancy_id", vacancy.ID),
		zap.String("resume_id", resume.ID),
		zap.String("model", modelName),
		zap.String("base_url", baseURL),
		zap.String("cache_name", cacheName),
		zap.Int("prompt_length", utf8.RuneCountInString(prompt)),
		zap.String("prompt_preview", previewText(prompt, 200)),
	)

	var raw string
	if cacheName != "" && cacheCaller != nil {
		raw, err = cacheCaller.GenerateContentWithCache(ctx, prompt, cacheName)
	} else {
		raw, err = m.generator.GenerateContent(ctx, prompt)
	}
	if err != nil {
		return nil, err
	}

	m.logger.Debug("gemini generate content response",
		zap.String("vacancy_id", vacancy.ID),
		zap.String("resume_id", resume.ID),
		zap.String("model", modelName),
		zap.String("cache_name", cacheName),
		zap.Int("response_length", utf8.RuneCountInString(raw)),
		zap.String("response_preview", previewText(raw, 200)),
	)

	assessment, err := parseResponse(raw)
	if err != nil {
		return nil, err
	}

	if m.minScore > 0 && !math.IsNaN(assessment.Score) && assessment.Score < m.minScore {
		m.logger.Debug("set fit to false by score threshold",
			zap.String("vacancy_id", vacancy.ID),
			zap.Float64("score", assessment.Score),
			zap.Float64("threshold", m.minScore),
		)
		assessment.Fit = false
	}

	assessment.Raw = raw
	return assessment, nil
}

func buildPrompt(resumeSection, vacancyJSON string) string {
	template := promptTemplate
	if strings.TrimSpace(template) == "" {
		template = "Resume context:\n{{RESUME_CONTEXT}}\n\nVacancy:\n{{VACANCY_JSON}}\n\nJSON Response:"
	}
	prompt := strings.ReplaceAll(template, "{{RESUME_CONTEXT}}", resumeSection)
	prompt = strings.ReplaceAll(prompt, "{{RESUME_JSON}}", resumeSection)
	prompt = strings.ReplaceAll(prompt, "{{VACANCY_JSON}}", vacancyJSON)
	return prompt
}

func parseResponse(raw string) (*ai.FitAssessment, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = extractJSON(cleaned)

	var data map[string]any
	if err := json.Unmarshal([]byte(cleaned), &data); err != nil {
		return nil, fmt.Errorf("parse gemini response: %w", err)
	}

	fit := coerceBool(data["fit"])
	score := coerceFloat(data["score"])
	reason := coerceString(data["reason"])
	message := coerceString(data["message"])

	if math.IsNaN(score) {
		score = 0
	}

	return &ai.FitAssessment{
		Fit:     fit,
		Score:   score,
		Reason:  reason,
		Message: message,
	}, nil
}

func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSpace(raw)
		if idx := strings.LastIndex(raw, "```"); idx != -1 {
			raw = raw[:idx]
		}
	}
	raw = strings.Trim(raw, "`")
	return strings.TrimSpace(raw)
}

func coerceBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		lower := strings.ToLower(strings.TrimSpace(val))
		return lower == "true" || lower == "yes"
	case float64:
		return val != 0
	default:
		return false
	}
}

func coerceFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		trimmed := strings.TrimSpace(val)
		if trimmed == "" {
			return math.NaN()
		}
		f, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return math.NaN()
		}
		return f
	default:
		return math.NaN()
	}
}

func coerceString(v any) string {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case fmt.Stringer:
		return strings.TrimSpace(val.String())
	default:
		if v == nil {
			return ""
		}
		bytes, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(bytes)
	}
}

func previewText(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}
