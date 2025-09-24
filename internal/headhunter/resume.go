package headhunter

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
)

type Resumes struct {
	Items []*Resume
}

type Resume struct {
	Title string
	ID    string `json:"id,omitempty"`
}

type ResumeDetails struct {
	ID    string
	Title string
	Raw   map[string]any
}

func (c *Client) getResumes(id string) (*Resumes, error) {
	apiURLMineResumes := fmt.Sprintf("%s/resumes/%s", c.APIURL, id)

	items, err := c.GetItems(apiURLMineResumes, nil)
	if err != nil {
		return nil, err
	}

	var resumes []*Resume
	if err = mapstructure.Decode(items, &resumes); err != nil {
		return nil, err
	}

	return &Resumes{
		Items: resumes,
	}, nil
}

func (r *Resumes) Len() int {
	return len(r.Items)
}

func (r *Resumes) Titles() []string {
	ids := make([]string, 0, len(r.Items))

	for _, v := range r.Items {
		ids = append(ids, v.Title)
	}

	return ids
}

func (r *Resumes) FindByTitle(title string) *Resume {
	for _, resume := range r.Items {
		if resume.Title == title {
			return resume
		}
	}

	return nil
}

func (c *Client) GetResumeDetails(id string) (*ResumeDetails, error) {
	if id == "" {
		return nil, fmt.Errorf("resume id is required")
	}

	apiURL := fmt.Sprintf("%s/resumes/%s", c.APIURL, id)

	var raw map[string]any
	if err := c.getJSON(apiURL, nil, &raw); err != nil {
		return nil, err
	}

	if raw == nil {
		raw = make(map[string]any)
	}

	return &ResumeDetails{
		ID:    valueAsString(raw["id"]),
		Title: valueAsString(raw["title"]),
		Raw:   raw,
	}, nil
}

func valueAsString(v any) string {
	if v == nil {
		return ""
	}

	switch typed := v.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}
