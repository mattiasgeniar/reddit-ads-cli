package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// maxListPages bounds how many pages GetList will follow, so a buggy or adversarial
// next_url that loops cannot spin forever.
const maxListPages = 10_000

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
	pageURL := c.base + path
	for pages := 0; pageURL != ""; pages++ {
		if pages >= maxListPages {
			return nil, fmt.Errorf("pagination exceeded %d pages; aborting to avoid a loop", maxListPages)
		}
		resp, err := c.do(http.MethodGet, pageURL, nil)
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

		if next := env.Pagination.NextURL; next != "" {
			// The bearer token is attached to every request, so never follow next_url
			// off to another host: that would hand our access token to it.
			if err := c.sameHostAsBase(next); err != nil {
				return nil, err
			}
		}
		pageURL = env.Pagination.NextURL
	}
	return all, nil
}

// sameHostAsBase rejects a pagination URL whose scheme or host differs from the API
// base, so a poisoned next_url cannot exfiltrate the bearer token to another origin.
func (c *Client) sameHostAsBase(next string) error {
	base, err := url.Parse(c.base)
	if err != nil {
		return err
	}
	n, err := url.Parse(next)
	if err != nil {
		return fmt.Errorf("invalid next_url %q: %w", next, err)
	}
	if n.Scheme != base.Scheme || n.Host != base.Host {
		return fmt.Errorf("refusing to follow next_url to a different host: %q", next)
	}
	return nil
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
