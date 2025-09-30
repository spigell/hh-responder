package filtering

import (
	"context"
	"fmt"

	"github.com/spigell/hh-responder/internal/headhunter"
)

type excludeFileFilter struct {
	path string
}

// NewExcludeFile creates a filter that removes vacancies contained in exclude files.
func NewExcludeFile(path string) Filter {
	return &excludeFileFilter{
		path: path,
	}
}

func (f *excludeFileFilter) Name() string { return "exclude_file" }

func (f *excludeFileFilter) Disable(string) {}

func (f *excludeFileFilter) IsEnabled() bool { return true }

func (f *excludeFileFilter) Validate() error { return nil }

func (f *excludeFileFilter) Apply(_ context.Context, v *headhunter.Vacancies) (*headhunter.Vacancies, Step, error) {
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

	return v, Step{Initial: initial, Dropped: len(removed), Left: v.Len()}, nil
}
