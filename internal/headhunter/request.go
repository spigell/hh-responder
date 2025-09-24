package headhunter

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

const (
	contentType     = "application/json"
	contentEncoding = "gzip, deflate, br"
)

type ItemResponse struct {
	Items   []Item
	Found   int
	Pages   int
	Page    int
	PerPage int `json:"per_page"`
}

type Item interface{}

// GetItems makes GET request to HeadHunter API and return items from all pages.
func (c *Client) GetItems(url string, q url.Values) ([]Item, error) {
	var items []Item

	req, err := http.NewRequestWithContext(c.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req = c.setHeaders(req)
	// Additional headers. For GET requests only
	req.Header.Set("Content-Type", contentType)
	req.URL.RawQuery = q.Encode()

	resp, err := c.request(req)
	if err != nil {
		return nil, err
	}

	response, err := c.parseItemResponse(resp)
	if err != nil {
		return nil, err
	}

	c.logger.Debug("got response from HH.ru", zap.Int("pages", response.Pages), zap.Int("max items per page", response.PerPage))

	items = append(items, response.Items...)

	for response.Page < (response.Pages - 1) {
		c.logger.Debug("additional request neeeded", zap.String("reason", fmt.Sprintf(
			"current page (%d) < all page count (%d)", response.Page+1, response.Pages),
		))

		resp, err = c.request(addPage(req, response.Page+1))
		if err != nil {
			return nil, err
		}

		response, err = c.parseItemResponse(resp)
		if err != nil {
			return nil, err
		}

		items = append(items, response.Items...)
	}

	return items, nil
}

func (c *Client) parseItemResponse(resp *http.Response) (*ItemResponse, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	var body io.ReadCloser
	var err error
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		body, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer body.Close()
	default:
		body = resp.Body
		defer body.Close()
	}

	var response *ItemResponse
	if err := json.NewDecoder(body).Decode(&response); err != nil {
		return nil, err
	}

	return response, nil
}

func (c *Client) postFormData(url string, data map[string]string) error {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for key, val := range data {
		field, err := w.CreateFormField(key)
		if err != nil {
			return err
		}

		_, err = io.Copy(field, strings.NewReader(val))
		if err != nil {
			return err
		}
	}
	w.Close()

	req, err := http.NewRequestWithContext(c.ctx, http.MethodPost, url, &b)
	if err != nil {
		return err
	}

	req = c.setHeaders(req)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.request(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	return nil
}

func (c *Client) request(req *http.Request) (*http.Response, error) {
	c.logger.Debug("make request", zap.String("url", req.URL.String()))
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *Client) setHeaders(req *http.Request) *http.Request {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept-Encoding", contentEncoding)

	return req
}

func (c *Client) getJSON(url string, q url.Values, target interface{}) error {
	req, err := http.NewRequestWithContext(c.ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	req = c.setHeaders(req)
	req.Header.Set("Content-Type", contentType)
	if q != nil {
		req.URL.RawQuery = q.Encode()
	}

	resp, err := c.request(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	var gzipReader *gzip.Reader
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return err
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	if target == nil {
		return nil
	}

	if err := json.Unmarshal(data, target); err != nil {
		return err
	}

	return nil
}

// addPage adds page parameter to request URL.
func addPage(req *http.Request, page int) *http.Request {
	q := req.URL.Query()
	q.Set("page", strconv.Itoa(page))
	req.URL.RawQuery = q.Encode()

	return req
}
