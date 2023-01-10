package headhunter

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"

	"github.com/mitchellh/mapstructure"
)

const (
	SearchPath = "/vacancies"
)

type SearchParams struct {
	Text string `yaml:"text"`
	// hhparam is custom tag for reflect. Please see below.
	Areas       []int    `hhparam:"area"`
	Clusters    bool     `yaml:"clusters"`
	OrderBy     string   `yaml:"order_by"`
	Employer    uint     `yaml:"employer_id" mapstructure:"employer_id"`
	SearchField string   `yaml:"search_field" mapstructure:"search_field"`
	Schedules   []string `hhparam:"schedule"`
	PerPage     string   `yaml:"per_page" mapstructure:"per_page"`
	Experience  string   `yaml:"experience"`
	Period      uint     `yaml:"period"`
}

func (c *Client) search(params *SearchParams) (*Vacancies, error) {
	var vacancies []*Vacancy

	// Set per_page max as possible. It should be faster.
	if params.PerPage == "" {
		params.PerPage = perPage
	}

	q := buildParams(params)
	apiURLSearch := fmt.Sprintf("%s%s", c.APIURL, SearchPath)

	items, err := c.GetItems(apiURLSearch, q)
	if err != nil {
		return nil, err
	}

	cfg := &mapstructure.DecoderConfig{
		Metadata: nil,
		Result:   &vacancies,
		TagName:  "json",
	}
	decoder, _ := mapstructure.NewDecoder(cfg)
	decoder.Decode(items)

	return &Vacancies{
		Items: vacancies,
	}, nil
}

func buildParams(params *SearchParams) url.Values {
	q := url.Values{}
	fields := reflect.VisibleFields(reflect.TypeOf(*params))
	// TODO: need create test for this
	for _, field := range fields {
		// Our custom tag is using here.
		key := field.Tag.Get("hhparam")
		if key == "" {
			// Failover to default tag if our tag do not exist.
			key = field.Tag.Get("yaml")
		}
		kind := field.Type.Kind()
		switch kind {
		case reflect.Slice:

			s := reflect.ValueOf(params).Elem().Field(field.Index[0]).Interface()
			switch v := s.(type) {
			case []int:
				for _, value := range v {
					q.Add(key, strconv.Itoa(value))
				}

			case []string:
				for _, value := range v {
					q.Add(key, value)
				}
			}

		default:
			value := fmt.Sprintf("%v", reflect.ValueOf(params).Elem().Field(field.Index[0]).Interface())
			if value != "" && value != "0" {
				q.Set(key, value)
			}
		}
	}

	return q
}