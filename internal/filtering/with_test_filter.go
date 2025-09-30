package filtering

import (
	"context"

	"github.com/spigell/hh-responder/internal/headhunter"
)

type withTestFilter struct{}

// NewWithTest creates a filter that removes vacancies requiring tests.
func NewWithTest() Filter {
	return &withTestFilter{}
}

func (f *withTestFilter) Name() string { return "with_test" }

func (f *withTestFilter) Disable(string) {}

func (f *withTestFilter) IsEnabled() bool { return true }

func (f *withTestFilter) Validate() error { return nil }

func (f *withTestFilter) Apply(_ context.Context, v *headhunter.Vacancies) (*headhunter.Vacancies, Step, error) {
	initial := v.Len()
	excluded := v.ExcludeWithTest()

	return v, Step{Initial: initial, Dropped: len(excluded), Left: v.Len()}, nil
}
