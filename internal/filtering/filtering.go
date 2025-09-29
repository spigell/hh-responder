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

// DisableByName marks a filter with the provided name as disabled while keeping it in the list.
func DisableByName(steps []Filter, name, reason string) {
	for _, step := range steps {
		if step.Name() == name {
			step.Disable(reason)
		}
	}
}

// Run executes the supplied filters sequentially, returning the resulting vacancies list and AI assessments.
func Run(ctx context.Context, cfg *Config, deps Deps, steps []Filter, vacancies *headhunter.Vacancies) (*headhunter.Vacancies, error) {
	for _, step := range steps {
		if !step.IsEnabled() {
			continue
		}
		if err := step.Validate(cfg); err != nil {
			return nil, fmt.Errorf("%s: %w", step.Name(), err)
		}
	}

	for _, step := range steps {
		if !step.IsEnabled() {
			deps.Logger.Info("filter disabled", zap.String("name", step.Name()))
			continue
		}

		next, info, err := step.Apply(ctx, deps, vacancies)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", step.Name(), err)
		}

		deps.Logger.Info("filter step",
			zap.String("name", step.Name()),
			zap.Int("initial", info.Initial),
			zap.Int("dropped", info.Dropped),
			zap.Int("left", info.Left),
		)

		vacancies = next

	}

	return vacancies, nil
}
