package headhunter

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const (
	apiURL      = "https://api.hh.ru"
	mineResumID = "mine"
	userAgent   = "spigell/hh-responder (spigelly@gmail.com)"
	// Max value for search per page.
	perPage = "100"
)

type Client struct {
	// ctx used only for http requests right now
	ctx        context.Context
	token      string
	logger     *zap.Logger
	HTTPClient *http.Client
	UserAgent  string
	APIURL     string
}

func New(ctx context.Context, logger *zap.Logger, token string) *Client {
	return &Client{
		ctx:    ctx,
		token:  token,
		APIURL: apiURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:    logger,
		UserAgent: userAgent,
	}
}

func (c *Client) Search(params *SearchParams) (*Vacancies, error) {
	return c.search(params)
}

func (c *Client) GetMineResumes() (*Resumes, error) {
	return c.getResumes(mineResumID)
}

func (c *Client) Apply(resume *Resume, vacancies *Vacancies, message string) error {
	for _, v := range vacancies.Items {
		if err := c.postNegotiation(resume.ID, v.ID, message); err != nil {
			return err
		}
	}

	return nil
}
