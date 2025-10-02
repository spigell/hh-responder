package ai

import (
	"context"

	"github.com/spigell/hh-responder/internal/headhunter"
)

type FitAssessment struct {
	Fit     bool
	Score   float64
	Reason  string
	Message string
	Raw     string
}

type Matcher interface {
	Evaluate(ctx context.Context, resumePayload map[string]any, vacancy *headhunter.Vacancy) (*FitAssessment, error)
}
