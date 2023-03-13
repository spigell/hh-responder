package headhunter

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const (
	VacancyIDField         = "ID"
	VacancyEmployerIDField = "EmployerID"
)

type Vacancies struct {
	Items []*Vacancy
}

type Vacancy struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Area struct {
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
		URL  string `json:"url,omitempty"`
	} `json:"area,omitempty"`
	HasTest bool `json:"has_test,omitempty"`
	Salary  struct {
		From     int    `json:"from,omitempty"`
		To       int    `json:"to,omitempty"`
		Currency string `json:"currency,omitempty"`
		Gross    bool   `json:"gross,omitempty"`
	} `json:"salary,omitempty"`
	Experience struct {
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"experience,omitempty"`
	Schedule struct {
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"schedule,omitempty"`
	Employer struct {
		ID           string `json:"id,omitempty"`
		Name         string `json:"name,omitempty"`
		URL          string `json:"url,omitempty"`
		AlternateURL string `json:"alternate_url,omitempty"`
		LogoUrls     struct {
			Original string `json:"original,omitempty"`
		} `json:"logo_urls,omitempty"`
		VacanciesURL string `json:"vacancies_url,omitempty"`
		Trusted      bool   `json:"trusted,omitempty"`
	} `json:"employer,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	AlternateURL string `json:"alternate_url,omitempty"`
	Employment   struct {
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"employment,omitempty"`
	Description string `json:"description,omitempty"`
	KeySkills   []struct {
		Name string `json:"name,omitempty"`
	} `json:"key_skills,omitempty"`
	Archived bool `json:"archived,omitempty"`
	Snipet   struct {
		Requirement    string `json:"requirement,omitempty"`
		Responsibility string `json:"responsibility,omitempty"`
	} `json:"snippet,omitempty"`
	Specializations []struct {
		ID           string `json:"id,omitempty"`
		Name         string `json:"name,omitempty"`
		ProfareaID   string `json:"profarea_id,omitempty"`
		ProfareaName string `json:"profarea_name,omitempty"`
	} `json:"specializations,omitempty"`
	ProfessionalRoles []struct {
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"professional_roles,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

type ExcludedVacancies struct {
	Items []*ExcludedVacancy
}

type ExcludedVacancy struct {
	ID           string
	URL          string
	EmployerName string
	ExcludedAt   time.Time
}

func (v *Vacancies) DumpToTmpFile() (string, error) {
	file, err := os.CreateTemp("", "vacancies_*.json")
	if err != nil {
		return "", err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return file.Name(), nil
}

func (v *Vacancies) ToExcluded() *ExcludedVacancies {
	excluded := &ExcludedVacancies{}
	for _, vacancy := range v.Items {
		excluded.Items = append(excluded.Items, &ExcludedVacancy{
			ID:           vacancy.ID,
			URL:          vacancy.AlternateURL,
			EmployerName: vacancy.Employer.Name,
			ExcludedAt:   time.Now().UTC(),
		})
	}
	return excluded
}

func GetExludedVacanciesFromFile(path string) (*ExcludedVacancies, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if stat.Size() == 0 {
		return &ExcludedVacancies{}, nil
	}

	var excluded ExcludedVacancies
	if err := json.NewDecoder(file).Decode(&excluded); err != nil {
		return nil, err
	}
	return &excluded, nil
}

func (v *ExcludedVacancies) Append(s *ExcludedVacancies) {
	v.Items = append(v.Items, s.Items...)
}

func (v *ExcludedVacancies) VacanciesIDs() []string {
	ids := make([]string, 0)
	for _, vacancy := range v.Items {
		ids = append(ids, vacancy.ID)
	}
	return ids
}

func (v *ExcludedVacancies) ToFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return nil
}

func (va *Vacancy) GetStringField(name string) string {
	switch name {
	case VacancyIDField:
		return va.ID
	case VacancyEmployerIDField:
		return va.Employer.ID

	default:
		return ""
	}
}

// TODO: need create test for this
// Report by employer.
func (v *Vacancies) ReportByEmployer() map[string][]map[string]string {
	report := make(map[string][]map[string]string)
	for _, vacancy := range v.Items {
		key := fmt.Sprintf("%s (%s)", vacancy.Employer.Name, vacancy.Employer.ID)
		report[key] = append(report[key], map[string]string{
			"name":                 vacancy.Name,
			"url":                  vacancy.AlternateURL,
			"area":                 vacancy.Area.Name,
			"salary":               fmt.Sprintf("%d-%d %s", vacancy.Salary.From, vacancy.Salary.To, vacancy.Salary.Currency),
			"brief requirement":    vacancy.Snipet.Requirement,
			"brief responsibility": vacancy.Snipet.Responsibility,
		})
	}
	return report
}

func (v *Vacancies) Len() int {
	return len(v.Items)
}

func (v *Vacancies) FindByID(id string) *Vacancy {
	for _, vacancy := range v.Items {
		if vacancy.ID == id {
			return vacancy
		}
	}
	return nil
}

func (v *Vacancies) ExcludeWithTest() []string {
	var excluded []string
	for idx, vacancy := range v.Items {
		if vacancy.HasTest {
			v.RemoveByIndex(idx)
			excluded = append(excluded, vacancy.ID)
			break
		}
	}
	return excluded
}

// TODO: need create test for this
// Exclude function exclude vacancies from list by id.
func (v *Vacancies) Exclude(name string, targets []string) []string {
	var excluded []string
	for _, target := range targets {
		for idx, vacancy := range v.Items {
			if vacancy.GetStringField(name) == target {
				v.RemoveByIndex(idx)
				excluded = append(excluded, vacancy.ID)
				break
			}
		}
	}
	return excluded
}

// RemoveByIndex remove vacancy from list by index. Do not preserve order.
func (v *Vacancies) RemoveByIndex(idx int) {
	v.Items[idx] = v.Items[len(v.Items)-1]
	v.Items = v.Items[:len(v.Items)-1]
}
