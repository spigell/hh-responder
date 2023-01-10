package headhunter

import (
	"fmt"
	"net/url"

	"github.com/mitchellh/mapstructure"
)

const (
	apiNegotiataionPath       = "/negotiations"
	allStatusesExceptArchived = "non_archived"
)

type Negotations []*Negotiation

type Negotiation struct {
	ID        string
	CreatedAt string `json:"created_at" mapstructure:"created_at"`
	URL       string
	Vacancy   *Vacancy
}

type NegotiationResponse struct {
	Items []*Negotiation
}

func (c *Client) GetNegotiations() (*Negotations, error) {
	apiURLMineNegotations := fmt.Sprintf("%s%s", c.APIURL, apiNegotiataionPath)

	q := url.Values{}
	// We never need our archived negotiations
	q.Add("status", allStatusesExceptArchived)
	// Set per_page max as possible. It should be faster.
	q.Add("per_page", perPage)

	items, err := c.GetItems(apiURLMineNegotations, q)
	if err != nil {
		return nil, err
	}

	var negotations Negotations
	if err = mapstructure.Decode(items, &negotations); err != nil {
		return nil, err
	}

	return &negotations, nil
}

func (n *Negotations) VacanciesIDs() []string {
	ids := make([]string, 0, len(*n))

	for _, v := range *n {
		ids = append(ids, v.Vacancy.ID)
	}

	return ids
}

func (c *Client) postNegotiation(resume, vacancy, message string) error {
	apiURLMineNegotations := fmt.Sprintf("%s%s", c.APIURL, apiNegotiataionPath)

	data := map[string]string{
		"resume_id":  resume,
		"vacancy_id": vacancy,
		"message":    message,
	}

	err := c.postFormData(apiURLMineNegotations, data)
	if err != nil {
		return err
	}

	return nil
}
