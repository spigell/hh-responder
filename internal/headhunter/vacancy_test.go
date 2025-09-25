package headhunter

import "testing"

func TestReportByEmployerIncludesAIResults(t *testing.T) {
	vacancies := &Vacancies{
		Items: []*Vacancy{
			{
				ID:   "1",
				Name: "Go Developer",
				Employer: struct {
					ID           string `json:"id,omitempty"`
					Name         string `json:"name,omitempty"`
					URL          string `json:"url,omitempty"`
					AlternateURL string `json:"alternate_url,omitempty"`
					LogoUrls     struct {
						Original string `json:"original,omitempty"`
					} `json:"logo_urls,omitempty"`
					VacanciesURL string `json:"vacancies_url,omitempty"`
					Trusted      bool   `json:"trusted,omitempty"`
				}{
					ID:   "emp1",
					Name: "Acme",
				},
				AlternateURL: "https://example.com",
				Area: struct {
					ID   string `json:"id,omitempty"`
					Name string `json:"name,omitempty"`
					URL  string `json:"url,omitempty"`
				}{
					Name: "Moscow",
				},
				Snipet: struct {
					Requirement    string `json:"requirement,omitempty"`
					Responsibility string `json:"responsibility,omitempty"`
				}{
					Requirement:    "Strong Go skills",
					Responsibility: "Build services",
				},
				AI: &AIAssessment{
					Fit:     true,
					Score:   0.91,
					Reason:  "Matches tech stack",
					Message: "Hello",
				},
			},
		},
	}

	report := vacancies.ReportByEmployer()

	entries, ok := report["Acme (emp1)"]
	if !ok {
		t.Fatalf("expected employer key in report")
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry["ai_fit"] != "true" {
		t.Fatalf("expected ai_fit true, got %q", entry["ai_fit"])
	}
	if entry["ai_score"] != "0.91" {
		t.Fatalf("expected ai_score 0.91, got %q", entry["ai_score"])
	}
	if entry["ai_reason"] != "Matches tech stack" {
		t.Fatalf("unexpected ai_reason: %q", entry["ai_reason"])
	}
	if entry["ai_message"] != "Hello" {
		t.Fatalf("unexpected ai_message: %q", entry["ai_message"])
	}
}

func TestReportByEmployerIncludesAIError(t *testing.T) {
	vacancies := &Vacancies{
		Items: []*Vacancy{
			{
				ID:   "2",
				Name: "Python Developer",
				Employer: struct {
					ID           string `json:"id,omitempty"`
					Name         string `json:"name,omitempty"`
					URL          string `json:"url,omitempty"`
					AlternateURL string `json:"alternate_url,omitempty"`
					LogoUrls     struct {
						Original string `json:"original,omitempty"`
					} `json:"logo_urls,omitempty"`
					VacanciesURL string `json:"vacancies_url,omitempty"`
					Trusted      bool   `json:"trusted,omitempty"`
				}{
					ID:   "emp2",
					Name: "Globex",
				},
				AI: &AIAssessment{Error: "quota exceeded"},
			},
		},
	}

	report := vacancies.ReportByEmployer()
	entries := report["Globex (emp2)"]
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry["ai_error"] != "quota exceeded" {
		t.Fatalf("unexpected ai_error: %q", entry["ai_error"])
	}
	if _, ok := entry["ai_fit"]; ok {
		t.Fatalf("did not expect ai_fit for error case")
	}
}
