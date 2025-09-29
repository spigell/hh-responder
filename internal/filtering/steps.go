package filtering

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/spigell/hh-responder/internal/ai"
	"github.com/spigell/hh-responder/internal/headhunter"
)

const forceFlagSetMsg = "force flag is set"

type withTestFilter struct{}

// NewWithTest creates a filter that removes vacancies requiring tests.
func NewWithTest() Filter {
	return &withTestFilter{}
}

func (f *withTestFilter) Name() string { return "with_test" }

func (f *withTestFilter) Disable(string) {}

func (f *withTestFilter) IsEnabled() bool { return true }

func (f *withTestFilter) Validate(*Config) error { return nil }

func (f *withTestFilter) Apply(_ context.Context, deps Deps, v *headhunter.Vacancies) (*headhunter.Vacancies, Step, error) {
	initial := v.Len()
	excluded := v.ExcludeWithTest()
	if deps.Logger != nil && len(excluded) > 0 {
		deps.Logger.Info("excluding vacancies with tests. It is impossible to apply them",
			zap.Strings("excluded_vacancies", excluded),
			zap.Int("vacancies_left", v.Len()),
		)
	}

	return v, Step{Initial: initial, Dropped: len(excluded), Left: v.Len()}, nil
}

func (f *withTestFilter) Status() Status {
	return Status{Name: f.Name(), Enabled: true}
}

type appliedHistoryFilter struct {
	ignore bool
}

// NewAppliedHistory creates a filter that removes vacancies found in negotiation history.
func NewAppliedHistory(cmd *cobra.Command) Filter {
	ignore := false
	if cmd != nil {
		flag := cmd.Flag("do-not-exclude-applied")
		if flag != nil && strings.EqualFold(flag.Value.String(), "true") {
			ignore = true
		}
	}
	return &appliedHistoryFilter{ignore: ignore}
}

func (f *appliedHistoryFilter) Name() string { return "applied_history" }

func (f *appliedHistoryFilter) Disable(string) {}

func (f *appliedHistoryFilter) IsEnabled() bool { return true }

func (f *appliedHistoryFilter) Validate(*Config) error { return nil }

func (f *appliedHistoryFilter) Apply(ctx context.Context, deps Deps, v *headhunter.Vacancies) (*headhunter.Vacancies, Step, error) {
	initial := v.Len()
	if f.ignore {
		if deps.Logger != nil {
			deps.Logger.Info("ignoring already applied vacancies", zap.String("reason", forceFlagSetMsg))
		}
		return v, Step{Initial: initial, Dropped: 0, Left: v.Len()}, nil
	}

	if deps.HH == nil {
		return v, Step{}, fmt.Errorf("headhunter client is required")
	}

	negotiations, err := deps.HH.GetNegotiations()
	if err != nil {
		return v, Step{}, fmt.Errorf("get my negotiations: %w", err)
	}

	excluded := v.Exclude(headhunter.VacancyIDField, negotiations.VacanciesIDs())
	if deps.Logger != nil && len(excluded) > 0 {
		deps.Logger.Info("excluding vacancies based on my negotiations",
			zap.Strings("excluded_vacancies", excluded),
			zap.Int("vacancies_left", v.Len()),
		)
	}

	return v, Step{Initial: initial, Dropped: len(excluded), Left: v.Len()}, nil
}

func (f *appliedHistoryFilter) Status() Status {
	details := map[string]string{
		"exclude_applied": strconv.FormatBool(!f.ignore),
	}
	reason := ""
	if f.ignore {
		reason = "skip requested via flag"
	}
	return Status{Name: f.Name(), Enabled: true, Reason: reason, Details: details}
}

type employersFilter struct {
	employers []string
}

// NewEmployers creates a filter that removes vacancies by employers configured in the config.
func NewEmployers() Filter {
	return &employersFilter{}
}

func (f *employersFilter) Name() string { return "employers" }

func (f *employersFilter) Disable(string) {}

func (f *employersFilter) IsEnabled() bool { return true }

func (f *employersFilter) Validate(cfg *Config) error {
	f.employers = nil
	if cfg != nil {
		f.employers = append(f.employers, cfg.Employers...)
	}
	return nil
}

func (f *employersFilter) Apply(_ context.Context, deps Deps, v *headhunter.Vacancies) (*headhunter.Vacancies, Step, error) {
	initial := v.Len()
	if len(f.employers) == 0 {
		return v, Step{Initial: initial, Dropped: 0, Left: v.Len()}, nil
	}

	excluded := v.Exclude(headhunter.VacancyEmployerIDField, f.employers)
	if deps.Logger != nil && len(excluded) > 0 {
		deps.Logger.Info("excluding vacancies by employers",
			zap.Strings("excluded_employers", f.employers),
			zap.Strings("excluded_vacancies", excluded),
			zap.Int("vacancies_left", v.Len()),
		)
	}

	return v, Step{Initial: initial, Dropped: len(excluded), Left: v.Len()}, nil
}

func (f *employersFilter) Status() Status {
	details := map[string]string{}
	if len(f.employers) > 0 {
		details["employers"] = strings.Join(f.employers, ",")
	}
	return Status{Name: f.Name(), Enabled: true, Details: details}
}

type excludeFileFilter struct {
	path string
}

// NewExcludeFile creates a filter that removes vacancies contained in exclude files.
func NewExcludeFile() Filter {
	return &excludeFileFilter{}
}

func (f *excludeFileFilter) Name() string { return "exclude_file" }

func (f *excludeFileFilter) Disable(string) {}

func (f *excludeFileFilter) IsEnabled() bool { return true }

func (f *excludeFileFilter) Validate(*Config) error {
	f.path = strings.TrimSpace(viper.GetString("exclude-file"))
	return nil
}

func (f *excludeFileFilter) Apply(_ context.Context, deps Deps, v *headhunter.Vacancies) (*headhunter.Vacancies, Step, error) {
	initial := v.Len()
	if f.path == "" {
		return v, Step{Initial: initial, Dropped: 0, Left: v.Len()}, nil
	}

	excluded, err := headhunter.GetExludedVacanciesFromFile(f.path)
	if err != nil {
		return v, Step{}, fmt.Errorf("getting excluded vacancies from file: %w", err)
	}

	ids := excluded.VacanciesIDs()
	removed := v.Exclude(headhunter.VacancyIDField, ids)
	if deps.Logger != nil && len(removed) > 0 {
		deps.Logger.Info("excluding vacancies based on exclude file",
			zap.String("path", f.path),
			zap.Strings("excluded_vacancies", removed),
			zap.Int("vacancies_left", v.Len()),
		)
	}

	return v, Step{Initial: initial, Dropped: len(removed), Left: v.Len()}, nil
}

func (f *excludeFileFilter) Status() Status {
	details := map[string]string{}
	if f.path != "" {
		details["path"] = f.path
	}
	return Status{Name: f.Name(), Enabled: true, Details: details}
}

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
	for id, assessment := range assessments {
		f.assessments[id] = assessment
	}

	left := v.Len()
	return v, Step{Initial: initial, Dropped: initial - left, Left: left}, nil
}

func (f *aiFitFilter) Assessments() map[string]*ai.FitAssessment {
	if f.assessments == nil {
		return map[string]*ai.FitAssessment{}
	}
	return f.assessments
}

func (f *aiFitFilter) Status() Status {
	details := map[string]string{}
	if f.config != nil {
		details["minimum_fit_score"] = fmt.Sprintf("%.2f", f.config.MinimumFitScore)
		if f.config.Gemini != nil {
			details["model"] = f.config.Gemini.Model
			details["max_retries"] = strconv.Itoa(f.config.Gemini.MaxRetries)
			details["max_log_length"] = strconv.Itoa(f.config.Gemini.MaxLogLength)
		}
	}
	return Status{Name: f.Name(), Enabled: f.IsEnabled(), Reason: f.reason, Details: details}
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

	if initial != len(approved) && logger != nil {
		logger.Info("AI filtering completed",
			zap.Int("initial_vacancies", initial),
			zap.Int("approved_vacancies", len(approved)),
		)
	}

	return assessments, nil
}
