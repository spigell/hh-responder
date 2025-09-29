package filtering

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/spigell/hh-responder/internal/headhunter"
)

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
