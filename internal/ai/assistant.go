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
	Evaluate(ctx context.Context, resume *headhunter.ResumeDetails, vacancy *headhunter.Vacancy) (*FitAssessment, error)
}
