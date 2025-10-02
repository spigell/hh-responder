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

func (c *Client) GetResumeRaw(id string) (map[string]any, error) {
	apiURL := fmt.Sprintf("%s/resumes/%s", c.APIURL, id)

	var raw map[string]any
	if err := c.getJSON(apiURL, nil, &raw); err != nil {
		return nil, err
	}

	if raw == nil {
		return nil, fmt.Errorf("resume is empty")
	}

	return raw, nil
}
