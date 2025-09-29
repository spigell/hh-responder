package filtering

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/spigell/hh-responder/internal/headhunter"
)

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
