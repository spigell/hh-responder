package filtering

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"go.uber.org/zap"

	"github.com/spigell/hh-responder/internal/ai"
	"github.com/spigell/hh-responder/internal/ai/gemini"
	"github.com/spigell/hh-responder/internal/headhunter"
)

type aiFitFilter struct {
	enabled     bool
	reason      string
	config      *AIFitFilterConfig
	assessments map[string]*ai.FitAssessment

	deps *AIFitFilterDeps
}

type AIFitFilterDeps struct {
	Logger      *zap.Logger
	HH          *headhunter.Client
	Matcher     ai.Matcher
	Resume      *headhunter.Resume
	ExcludeFile string
}

type AIFitFilterConfig struct {
	Enabled         bool
	Provider        string
	MinimumFitScore float64
	Gemini          *AIGeminiConfig
}

// GeminiConfig stores Gemini provider configuration.
type AIGeminiConfig struct {
	Model        string
	MaxRetries   int
	MaxLogLength int
}

// NewAIFit creates the AI-based filtering step.
func NewAIFit(cfg *AIFitFilterConfig, deps *AIFitFilterDeps) Filter {
	return &aiFitFilter{
		enabled: cfg.Enabled,
		deps:    deps,
		config:  cfg,
	}
}

func (f *aiFitFilter) Name() string { return "ai_fit" }

func (f *aiFitFilter) Disable(reason string) {
	f.enabled = false
	f.reason = reason
}

func (f *aiFitFilter) WithDeps(client *headhunter.Client, matcher *gemini.Matcher, resume *headhunter.Resume, logger *zap.Logger) {
	f.deps.HH = client
	f.deps.Matcher = matcher
	f.deps.Logger = logger
	f.deps.Resume = resume
}

func (f *aiFitFilter) IsEnabled() bool { return f.enabled }

func (f *aiFitFilter) Validate() error {
	if f.deps == nil {
		return fmt.Errorf("Deps are not initialized. Fitler is not usable.")
	}

	if f.config.Gemini == nil {
		return fmt.Errorf("gemini configuration is required when ai filter is enabled")
	}
	if strings.TrimSpace(f.config.Gemini.Model) == "" {
		return fmt.Errorf("gemini model is required when ai filter is enabled")
	}
	return nil
}

func (f *aiFitFilter) Apply(ctx context.Context, v *headhunter.Vacancies) (*headhunter.Vacancies, Step, error) {
	initial := v.Len()

	resumeDetails, err := f.deps.HH.GetResumeRaw(f.deps.Resume.ID)
	if err != nil {
		return v, Step{}, fmt.Errorf("get resume details: %w", err)
	}

	assessments, err := f.evaluateVacanciesWithMatcher(ctx, resumeDetails, v)
	if err != nil {
		return v, Step{}, err
	}

	f.assessments = make(map[string]*ai.FitAssessment, len(assessments))
	maps.Copy(f.assessments, assessments)

	left := v.Len()
	return v, Step{Initial: initial, Dropped: initial - left, Left: left}, nil
}

func (f *aiFitFilter) evaluateVacanciesWithMatcher(ctx context.Context, resume map[string]any, vacancies *headhunter.Vacancies) (map[string]*ai.FitAssessment, error) {

	initial := vacancies.Len()
	approved := make([]*headhunter.Vacancy, 0, initial)
	assessments := make(map[string]*ai.FitAssessment)

	for _, vacancy := range vacancies.Items {
		detailed := vacancy
		full, err := f.deps.HH.GetVacancy(vacancy.ID)
		if err != nil {
			f.deps.Logger.Warn("fetching detailed vacancy failed. It will be skipped.",
				zap.String("vacancy_id", vacancy.ID),
				zap.Error(err),
			)
			continue
		}

		detailed = full

		assessment, err := f.deps.Matcher.Evaluate(ctx, resume, detailed)
		if err != nil {
			f.deps.Logger.Warn("AI evaluation failed",
				zap.String("vacancy_id", vacancy.ID),
				zap.Error(err),
			)
			detailed.AI = &headhunter.AIAssessment{Error: err.Error()}
			approved = append(approved, detailed)
			continue
		}

		if !assessment.Fit {
			f.deps.Logger.Info("vacancy rejected by AI provider",
				zap.String("vacancy_id", vacancy.ID),
				zap.Float64("ai_score", assessment.Score),
				zap.String("reason", assessment.Reason),
			)

			if err := f.appendToExcludeFile(detailed); err != nil {
				f.deps.Logger.Warn("failed to append vacancy to exclude file",
					zap.String("vacancy_id", vacancy.ID),
					zap.Error(err),
				)
			}
			continue
		}

		f.deps.Logger.Info("vacancy approved by AI",
			zap.String("vacancy_id", vacancy.ID),
			zap.Float64("ai_score", assessment.Score),
		)

		detailed.AI = &headhunter.AIAssessment{
			Fit:     assessment.Fit,
			Score:   assessment.Score,
			Reason:  assessment.Reason,
			Message: assessment.Message,
			Raw:     assessment.Raw,
		}
		approved = append(approved, detailed)
		assessments[detailed.ID] = assessment
	}

	vacancies.Items = approved

	f.deps.Logger.Info("AI filtering completed",
		zap.Int("initial_vacancies", initial),
		zap.Int("approved_vacancies", len(approved)),
	)

	return assessments, nil
}

func (f *aiFitFilter) appendToExcludeFile(vacancy *headhunter.Vacancy) error {
	path := strings.TrimSpace(f.deps.ExcludeFile)
	if path == "" {
		return nil
	}

	excluded, err := headhunter.GetExludedVacanciesFromFile(path)
	if err != nil {
		return fmt.Errorf("load excluded vacancies: %w", err)
	}

	toAppend := (&headhunter.Vacancies{Items: []*headhunter.Vacancy{vacancy}}).ToExcluded()
	excluded.Append(toAppend)

	if err := excluded.ToFile(path); err != nil {
		return fmt.Errorf("write excluded vacancies: %w", err)
	}

	f.deps.Logger.Info("vacancy appended to exclude file",
		zap.String("vacancy_id", vacancy.ID),
		zap.String("exclude_file", path),
	)

	return nil
}
