package filtering

import (
	"context"
	"fmt"

	"github.com/spigell/hh-responder/internal/headhunter"
	"go.uber.org/zap"
)

// Filter represents a single filtering step applied to vacancies.
type Filter interface {
	Name() string
	Disable(reason string)
	IsEnabled() bool

	Validate() error
	Apply(ctx context.Context, v *headhunter.Vacancies) (*headhunter.Vacancies, Step, error)
}

type Filtering struct {
	steps  []Filter
	logger *zap.Logger
}

// Step describes the result of executing a filtering step.
type Step struct {
	Initial int
	Dropped int
	Left    int
}

func New(filters []Filter, logger *zap.Logger) *Filtering {
	return &Filtering{
		steps:  filters,
		logger: logger,
	}
}

// DisableByName marks a filter with the provided name as disabled while keeping it in the list.
func (f *Filtering) DisableByName(name, reason string) error {
	var filter Filter
	for _, step := range f.steps {
		if step.Name() == name {
			filter = step
			break
		}
	}

	if filter == nil {
		return fmt.Errorf("filter %s not found", name)
	}

	filter.Disable(reason)

	return nil
}

// Run executes the supplied filters sequentially, returning the resulting vacancies list and AI assessments.
func (f *Filtering) RunFilters(ctx context.Context, vacancies *headhunter.Vacancies) (*headhunter.Vacancies, error) {
	for _, step := range f.steps {
		if !step.IsEnabled() {
			continue
		}
		if err := step.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", step.Name(), err)
		}
	}

	for _, step := range f.steps {
		if !step.IsEnabled() {
			f.logger.Info("filter disabled", zap.String("name", step.Name()))
			continue
		}

		next, info, err := step.Apply(ctx, vacancies)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", step.Name(), err)
		}

		f.logger.Info("filter step",
			zap.String("name", step.Name()),
			zap.Int("initial", info.Initial),
			zap.Int("dropped", info.Dropped),
			zap.Int("left", info.Left),
		)

		vacancies = next
	}

	return vacancies, nil
}
