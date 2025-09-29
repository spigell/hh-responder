package filtering

import (
	"context"
	"fmt"

	"github.com/spigell/hh-responder/internal/ai"
	"github.com/spigell/hh-responder/internal/headhunter"
	"go.uber.org/zap"
)

// Filter represents a single filtering step applied to vacancies.
type Filter interface {
	Name() string
	Disable(reason string)
	IsEnabled() bool

	Validate(cfg *Config) error
	Apply(ctx context.Context, deps Deps, v *headhunter.Vacancies) (*headhunter.Vacancies, Step, error)
}

// Deps aggregates dependencies shared across all filtering steps.
type Deps struct {
	HH      *headhunter.Client
	Logger  *zap.Logger
	Resume  *headhunter.Resume
	Matcher ai.Matcher
}

// Step describes the result of executing a filtering step.
type Step struct {
	Initial int
	Dropped int
	Left    int
}

// Config contains configuration settings consumed by the filters.
type Config struct {
	Employers []string
	AI        *AIConfig
}

// AIConfig stores AI-related configuration used by the filters.
type AIConfig struct {
	Enabled         bool
	Provider        string
	MinimumFitScore float64
	Gemini          *GeminiConfig
}

// GeminiConfig stores Gemini provider configuration.
type GeminiConfig struct {
	Model        string
	MaxRetries   int
	MaxLogLength int
}

// Status represents runtime information about a filter.
type Status struct {
	Name    string
	Enabled bool
	Reason  string
	Details map[string]string
}

// statusProvider is implemented by filters that can supply detailed status information.
type statusProvider interface {
	Status() Status
}

// DisableByName marks a filter with the provided name as disabled while keeping it in the list.
func DisableByName(steps []Filter, name, reason string) {
	for _, step := range steps {
		if step.Name() == name {
			step.Disable(reason)
		}
	}
}

// Run executes the supplied filters sequentially, returning the resulting vacancies list and AI assessments.
func Run(ctx context.Context, cfg *Config, deps Deps, steps []Filter, v *headhunter.Vacancies) (*headhunter.Vacancies, map[string]*ai.FitAssessment, error) {
	for _, step := range steps {
		if !step.IsEnabled() {
			continue
		}
		if err := step.Validate(cfg); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", step.Name(), err)
		}
	}

	assessments := make(map[string]*ai.FitAssessment)
	for _, step := range steps {
		if !step.IsEnabled() {
			if deps.Logger != nil {
				deps.Logger.Info("filter disabled", zap.String("name", step.Name()))
			}
			continue
		}

		next, info, err := step.Apply(ctx, deps, v)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", step.Name(), err)
		}

		if deps.Logger != nil {
			deps.Logger.Info("filter step",
				zap.String("name", step.Name()),
				zap.Int("initial", info.Initial),
				zap.Int("dropped", info.Dropped),
				zap.Int("left", info.Left),
			)
		}

		v = next

		if collector, ok := step.(interface {
			Assessments() map[string]*ai.FitAssessment
		}); ok {
			for id, assessment := range collector.Assessments() {
				assessments[id] = assessment
			}
		}
	}

	return v, assessments, nil
}

// Describe returns status entries for the provided filters.
func Describe(steps []Filter) []Status {
	statuses := make([]Status, 0, len(steps))
	for _, step := range steps {
		if reporter, ok := step.(statusProvider); ok {
			statuses = append(statuses, reporter.Status())
			continue
		}

		statuses = append(statuses, Status{
			Name:    step.Name(),
			Enabled: step.IsEnabled(),
		})
	}
	return statuses
}
