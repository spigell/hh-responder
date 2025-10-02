package filtering

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/spigell/hh-responder/internal/headhunter"
)

const forceFlagSetMsg = "force flag is set"

type appliedHistoryFilter struct {
	deps   *AppliedHistoryDeps
	ignore bool
}

type AppliedHistoryDeps struct {
	HH     *headhunter.Client
	Logger *zap.Logger
}

type AppliedHistoryConfig struct {
	Ignore bool
}

// NewAppliedHistory creates a filter that removes vacancies found in negotiation history.
func NewAppliedHistory(cfg *AppliedHistoryConfig, deps *AppliedHistoryDeps) Filter {
	ignore := false
	if cfg != nil {
		ignore = cfg.Ignore
	}

	return &appliedHistoryFilter{
		deps:   deps,
		ignore: ignore,
	}
}

func (f *appliedHistoryFilter) Name() string { return "applied_history" }

func (f *appliedHistoryFilter) Disable(string) {}

func (f *appliedHistoryFilter) IsEnabled() bool { return true }

func (f *appliedHistoryFilter) Validate() error {
	if f.deps == nil || f.deps.HH == nil {
		return fmt.Errorf("headhunter client is required")
	}

	if f.deps.Logger == nil {
		return fmt.Errorf("logger is required")
	}

	return nil
}

func (f *appliedHistoryFilter) Apply(_ context.Context, v *headhunter.Vacancies) (*headhunter.Vacancies, Step, error) {
	initial := v.Len()
	if f.ignore {
		f.deps.Logger.Info("ignoring already applied vacancies", zap.String("reason", forceFlagSetMsg))
		return v, Step{Initial: initial, Dropped: 0, Left: v.Len()}, nil
	}

	negotiations, err := f.deps.HH.GetNegotiations()
	if err != nil {
		return v, Step{}, fmt.Errorf("get my negotiations: %w", err)
	}

	excluded := v.Exclude(headhunter.VacancyIDField, negotiations.VacanciesIDs())
	if len(excluded) > 0 {
		f.deps.Logger.Info("excluding vacancies based on my negotiations",
			zap.Strings("excluded_vacancies", excluded),
			zap.Int("vacancies_left", v.Len()),
		)
	}

	return v, Step{Initial: initial, Dropped: len(excluded), Left: v.Len()}, nil
}
