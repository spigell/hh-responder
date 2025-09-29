package filtering

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"go.uber.org/zap"

	"github.com/spigell/hh-responder/internal/ai"
	"github.com/spigell/hh-responder/internal/headhunter"
)

type aiFitFilter struct {
	disabled    bool
	reason      string
	config      *AIConfig
	assessments map[string]*ai.FitAssessment
}

// NewAIFit creates the AI-based filtering step.
func NewAIFit() Filter {
	return &aiFitFilter{}
}

func (f *aiFitFilter) Name() string { return "ai_fit" }

func (f *aiFitFilter) Disable(reason string) {
	f.disabled = true
	f.reason = reason
}

func (f *aiFitFilter) IsEnabled() bool { return !f.disabled }

func (f *aiFitFilter) Validate(cfg *Config) error {
	f.config = nil
	if cfg != nil {
		f.config = cfg.AI
	}
	if !f.IsEnabled() {
		return nil
	}
	if cfg == nil || cfg.AI == nil {
		return fmt.Errorf("ai configuration is required when ai filter is enabled")
	}
	if cfg.AI.Gemini == nil {
		return fmt.Errorf("gemini configuration is required when ai filter is enabled")
	}
	if strings.TrimSpace(cfg.AI.Gemini.Model) == "" {
		return fmt.Errorf("gemini model is required when ai filter is enabled")
	}
	return nil
}

func (f *aiFitFilter) Apply(ctx context.Context, deps Deps, v *headhunter.Vacancies) (*headhunter.Vacancies, Step, error) {
	initial := v.Len()
	if deps.Matcher == nil {
		if deps.Logger != nil {
			deps.Logger.Info("ai matcher is not configured; skipping ai_fit filter")
		}
		return v, Step{Initial: initial, Dropped: 0, Left: v.Len()}, nil
	}
	if deps.Resume == nil {
		return v, Step{}, fmt.Errorf("resume is required for AI evaluation")
	}
	if deps.HH == nil {
		return v, Step{}, fmt.Errorf("headhunter client is required for AI evaluation")
	}

	resumeDetails, err := deps.HH.GetResumeDetails(deps.Resume.ID)
	if err != nil {
		return v, Step{}, fmt.Errorf("get resume details: %w", err)
	}

	assessments, err := evaluateVacanciesWithMatcher(ctx, deps.Logger, deps.Matcher, resumeDetails, deps.HH, v)
	if err != nil {
		return v, Step{}, err
	}

	f.assessments = make(map[string]*ai.FitAssessment, len(assessments))
	maps.Copy(f.assessments, assessments)

	left := v.Len()
	return v, Step{Initial: initial, Dropped: initial - left, Left: left}, nil
}

func evaluateVacanciesWithMatcher(ctx context.Context, logger *zap.Logger, matcher ai.Matcher, resumeDetails *headhunter.ResumeDetails, hh *headhunter.Client, vacancies *headhunter.Vacancies) (map[string]*ai.FitAssessment, error) {
	if matcher == nil {
		return nil, nil
	}

	initial := vacancies.Len()
	approved := make([]*headhunter.Vacancy, 0, initial)
	assessments := make(map[string]*ai.FitAssessment)

	for _, vacancy := range vacancies.Items {
		detailed := vacancy
		if full, err := hh.GetVacancy(vacancy.ID); err == nil && full != nil {
			detailed = full
		} else if err != nil && logger != nil {
			logger.Debug("fetching detailed vacancy failed",
				zap.String("vacancy_id", vacancy.ID),
				zap.Error(err),
			)
		}

		assessment, err := matcher.Evaluate(ctx, resumeDetails, detailed)
		if err != nil {
			if logger != nil {
				logger.Warn("AI evaluation failed",
					zap.String("vacancy_id", vacancy.ID),
					zap.Error(err),
				)
			}
			detailed.AI = &headhunter.AIAssessment{Error: err.Error()}
			approved = append(approved, detailed)
			continue
		}

		if !assessment.Fit {
			if logger != nil {
				logger.Info("vacancy rejected by AI provider",
					zap.String("vacancy_id", vacancy.ID),
					zap.Float64("ai_score", assessment.Score),
					zap.String("reason", assessment.Reason),
				)
			}
			continue
		}

		if logger != nil {
			logger.Info("vacancy approved by AI",
				zap.String("vacancy_id", vacancy.ID),
				zap.Float64("ai_score", assessment.Score),
			)
		}

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

	if logger != nil {
		logger.Info("AI filtering completed",
			zap.Int("initial_vacancies", initial),
			zap.Int("approved_vacancies", len(approved)),
		)
	}

	return assessments, nil
}
