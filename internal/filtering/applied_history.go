package filtering

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/spigell/hh-responder/internal/headhunter"
)

const forceFlagSetMsg = "force flag is set"

type appliedHistoryFilter struct {
	ignore bool
}

// NewAppliedHistory creates a filter that removes vacancies found in negotiation history.
func NewAppliedHistory(cmd *cobra.Command) Filter {
	ignore := false
	if cmd != nil {
		flag := cmd.Flag("do-not-exclude-applied")
		if flag != nil && strings.EqualFold(flag.Value.String(), "true") {
			ignore = true
		}
	}
	return &appliedHistoryFilter{ignore: ignore}
}

func (f *appliedHistoryFilter) Name() string { return "applied_history" }

func (f *appliedHistoryFilter) Disable(string) {}

func (f *appliedHistoryFilter) IsEnabled() bool { return true }

func (f *appliedHistoryFilter) Validate(*Config) error { return nil }

func (f *appliedHistoryFilter) Apply(ctx context.Context, deps Deps, v *headhunter.Vacancies) (*headhunter.Vacancies, Step, error) {
	initial := v.Len()
	if f.ignore {
		if deps.Logger != nil {
			deps.Logger.Info("ignoring already applied vacancies", zap.String("reason", forceFlagSetMsg))
		}
		return v, Step{Initial: initial, Dropped: 0, Left: v.Len()}, nil
	}

	if deps.HH == nil {
		return v, Step{}, fmt.Errorf("headhunter client is required")
	}

	negotiations, err := deps.HH.GetNegotiations()
	if err != nil {
		return v, Step{}, fmt.Errorf("get my negotiations: %w", err)
	}

	excluded := v.Exclude(headhunter.VacancyIDField, negotiations.VacanciesIDs())
	if deps.Logger != nil && len(excluded) > 0 {
		deps.Logger.Info("excluding vacancies based on my negotiations",
			zap.Strings("excluded_vacancies", excluded),
			zap.Int("vacancies_left", v.Len()),
		)
	}

	return v, Step{Initial: initial, Dropped: len(excluded), Left: v.Len()}, nil
}

func (f *appliedHistoryFilter) Status() Status {
	details := map[string]string{
		"exclude_applied": strconv.FormatBool(!f.ignore),
	}
	reason := ""
	if f.ignore {
		reason = "skip requested via flag"
	}
	return Status{Name: f.Name(), Enabled: true, Reason: reason, Details: details}
}
