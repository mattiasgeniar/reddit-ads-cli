package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// listEnvelope is the shape of a paginated list response: a data array plus a
// pagination block whose next_url, when present, is followed verbatim. The docs are
// explicit that clients follow next_url directly rather than rebuilding query params.
type listEnvelope struct {
	Data       []json.RawMessage `json:"data"`
	Pagination struct {
		NextURL     string `json:"next_url"`
		PreviousURL string `json:"previous_url"`
	} `json:"pagination"`
}

// GetList fetches every page of a list endpoint, following pagination.next_url until
// it is empty, and returns the concatenated data elements. path is relative to the
// base URL for the first page; subsequent pages use the absolute next_url as given.
func (c *Client) GetList(path string) ([]json.RawMessage, error) {
	var all []json.RawMessage
	url := c.base + path
	for url != "" {
		resp, err := c.do(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, &APIError{StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(body))}
		}

		var env listEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Data...)
		url = env.Pagination.NextURL
	}
	return all, nil
}

// GetListInto fetches every page and unmarshals the collected elements into out,
// which must be a pointer to a slice.
func (c *Client) GetListInto(path string, out any) error {
	elems, err := c.GetList(path)
	if err != nil {
		return err
	}
	combined, err := json.Marshal(elems)
	if err != nil {
		return err
	}
	return json.Unmarshal(combined, out)
}
