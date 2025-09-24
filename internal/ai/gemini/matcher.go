package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spigell/hh-responder/internal/ai"
	"github.com/spigell/hh-responder/internal/headhunter"
	"go.uber.org/zap"
)

type contentGenerator interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
}

type Matcher struct {
	generator contentGenerator
	minScore  float64
	logger    *zap.Logger
}

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

	prompt := buildPrompt(string(resumeJSON), string(vacancyJSON))

	var modelName, baseURL string
	if info, ok := m.generator.(interface{ Model() string }); ok {
		modelName = info.Model()
	}
	if info, ok := m.generator.(interface{ BaseURL() string }); ok {
		baseURL = info.BaseURL()
	}

	if m.logger != nil {
		m.logger.Debug("gemini generate content request",
			zap.String("vacancy_id", vacancy.ID),
			zap.String("resume_id", resume.ID),
			zap.String("model", modelName),
			zap.String("base_url", baseURL),
			zap.Int("prompt_length", utf8.RuneCountInString(prompt)),
			zap.String("prompt_preview", previewText(prompt, 200)),
		)
	}

	raw, err := m.generator.GenerateContent(ctx, prompt)
	if err != nil {
		if m.logger != nil {
			m.logger.Debug("gemini generate content failed",
				zap.String("vacancy_id", vacancy.ID),
				zap.String("resume_id", resume.ID),
				zap.String("model", modelName),
				zap.Error(err),
			)
		}
		return nil, err
	}

	if m.logger != nil {
		m.logger.Debug("gemini generate content response",
			zap.String("vacancy_id", vacancy.ID),
			zap.String("resume_id", resume.ID),
			zap.String("model", modelName),
			zap.Int("response_length", utf8.RuneCountInString(raw)),
			zap.String("response_preview", previewText(raw, 200)),
		)
	}

	assessment, err := parseResponse(raw)
	if err != nil {
		return nil, err
	}

	if m.minScore > 0 && !math.IsNaN(assessment.Score) && assessment.Score < m.minScore {
		if m.logger != nil {
			m.logger.Debug("vacancy rejected by score threshold",
				zap.String("vacancy_id", vacancy.ID),
				zap.Float64("score", assessment.Score),
				zap.Float64("threshold", m.minScore),
			)
		}
		assessment.Fit = false
	}

	assessment.Raw = raw
	return assessment, nil
}

func buildPrompt(resumeJSON, vacancyJSON string) string {
	var sb strings.Builder
	sb.WriteString("You are an assistant that assesses job fit between a candidate resume and a vacancy.\n")
	sb.WriteString("Respond strictly in valid JSON without additional text.\n")
	sb.WriteString("Return an object with keys: fit (boolean), score (number between 0 and 1), reason (short string), message (short cover letter tailored to the vacancy using candidate experience).\n")
	sb.WriteString("If the candidate is not a fit set fit to false, score to 0, and provide a concise reason.\n")
	sb.WriteString("The message must be in the same language as the vacancy description when possible and stay under 1200 characters.\n")
	sb.WriteString("Resume:\n")
	sb.WriteString(resumeJSON)
	sb.WriteString("\nVacancy:\n")
	sb.WriteString(vacancyJSON)
	sb.WriteString("\nJSON Response:")
	return sb.String()
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
