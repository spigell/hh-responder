package filtering

import (
	"context"

	"github.com/spigell/hh-responder/internal/headhunter"
)

type employersFilter struct {
	employers []string
}

// NewEmployers creates a filter that removes vacancies by employers configured in the config.
func NewExludedEmployers(employers []string) Filter {
	return &employersFilter{
		employers: employers,
	}
}

func (f *employersFilter) Name() string { return "employers" }

func (f *employersFilter) Disable(string) {}

func (f *employersFilter) IsEnabled() bool { return true }

func (f *employersFilter) Validate() error { return nil }

func (f *employersFilter) Apply(_ context.Context, v *headhunter.Vacancies) (*headhunter.Vacancies, Step, error) {
	initial := v.Len()
	if len(f.employers) == 0 {
		return v, Step{Initial: initial, Dropped: 0, Left: v.Len()}, nil
	}

	excluded := v.Exclude(headhunter.VacancyEmployerIDField, f.employers)

	return v, Step{Initial: initial, Dropped: len(excluded), Left: v.Len()}, nil
}
