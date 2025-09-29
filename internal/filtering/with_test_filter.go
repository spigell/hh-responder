package filtering

import (
	"context"

	"github.com/spigell/hh-responder/internal/headhunter"
	"go.uber.org/zap"
)

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
