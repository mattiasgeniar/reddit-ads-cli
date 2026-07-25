package api

import (
	"encoding/json"
	"errors"
	"net/url"
)

// Me returns the authorizing user (GET /me).
func (c *Client) Me() (*Me, error) {
	var env dataEnvelope[Me]
	if err := c.Get("/me", &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// Businesses lists the businesses the user belongs to (GET /me/businesses).
func (c *Client) Businesses() ([]Business, error) {
	var out []Business
	if err := c.GetListInto("/me/businesses", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AdAccounts lists every ad account across the user's businesses. There is no flat
// "list my ad accounts" endpoint, so it fans out over the businesses.
func (c *Client) AdAccounts() ([]AdAccount, error) {
	businesses, err := c.Businesses()
	if err != nil {
		return nil, err
	}
	var all []AdAccount
	for _, b := range businesses {
		var accts []AdAccount
		if err := c.GetListInto("/businesses/"+b.ID+"/ad_accounts", &accts); err != nil {
			return nil, err
		}
		all = append(all, accts...)
	}
	return all, nil
}

// Path helpers keep the typed list methods and their --json raw counterparts in sync
// so the two can never drift apart.
func CampaignsPath(accountID string) string { return "/ad_accounts/" + accountID + "/campaigns" }
func AdGroupsPath(accountID string) string  { return "/ad_accounts/" + accountID + "/ad_groups" }
func AdsPath(accountID string) string       { return "/ad_accounts/" + accountID + "/ads" }

// AdAccountsRaw returns the raw ad account elements across all businesses, for --json.
func (c *Client) AdAccountsRaw() ([]json.RawMessage, error) {
	businesses, err := c.Businesses()
	if err != nil {
		return nil, err
	}
	var all []json.RawMessage
	for _, b := range businesses {
		elems, err := c.GetList("/businesses/" + b.ID + "/ad_accounts")
		if err != nil {
			return nil, err
		}
		all = append(all, elems...)
	}
	return all, nil
}

// Campaigns lists an account's campaigns.
func (c *Client) Campaigns(accountID string) ([]Campaign, error) {
	var out []Campaign
	err := c.GetListInto(CampaignsPath(accountID), &out)
	return out, err
}

// Campaign fetches one campaign.
func (c *Client) Campaign(id string) (*Campaign, error) {
	var env dataEnvelope[Campaign]
	if err := c.Get("/campaigns/"+id, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// AdGroups lists an account's ad groups.
func (c *Client) AdGroups(accountID string) ([]AdGroup, error) {
	var out []AdGroup
	err := c.GetListInto(AdGroupsPath(accountID), &out)
	return out, err
}

// AdGroup fetches one ad group, including its raw targeting object.
func (c *Client) AdGroup(id string) (*AdGroup, error) {
	var env dataEnvelope[AdGroup]
	if err := c.Get("/ad_groups/"+id, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// Ads lists an account's ads.
func (c *Client) Ads(accountID string) ([]Ad, error) {
	var out []Ad
	err := c.GetListInto(AdsPath(accountID), &out)
	return out, err
}

// Ad fetches one ad.
func (c *Client) Ad(id string) (*Ad, error) {
	var env dataEnvelope[Ad]
	if err := c.Get("/ads/"+id, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// patchData sends {"data": body} to a PATCH endpoint. Reddit wraps writes in a data
// envelope just as it wraps reads.
func (c *Client) patchData(path string, data any) error {
	return c.Patch(path, map[string]any{"data": data}, nil)
}

// PatchCampaign applies a partial update to a campaign.
func (c *Client) PatchCampaign(id string, data map[string]any) error {
	return c.patchData("/campaigns/"+id, data)
}

// PatchAdGroup applies a partial update to an ad group.
func (c *Client) PatchAdGroup(id string, data map[string]any) error {
	return c.patchData("/ad_groups/"+id, data)
}

// PatchAd applies a partial update to an ad.
func (c *Client) PatchAd(id string, data map[string]any) error {
	return c.patchData("/ads/"+id, data)
}

func communitySearchPath(query string) string {
	q := url.Values{}
	q.Set("query", query)
	return "/targeting/communities/search?" + q.Encode()
}

func communitySuggestPath(names []string) string {
	q := url.Values{}
	for _, n := range names {
		q.Add("names", n)
	}
	return "/targeting/communities/suggestions?" + q.Encode()
}

// SearchCommunities resolves a free-text query to targetable communities.
func (c *Client) SearchCommunities(query string) ([]Community, error) {
	var out []Community
	err := c.GetListInto(communitySearchPath(query), &out)
	return out, err
}

// SearchCommunitiesRaw returns the raw community elements for --json.
func (c *Client) SearchCommunitiesRaw(query string) ([]json.RawMessage, error) {
	return c.GetList(communitySearchPath(query))
}

// SuggestCommunities expands from seed community names into similar communities.
func (c *Client) SuggestCommunities(names []string) ([]Community, error) {
	var out []Community
	err := c.GetListInto(communitySuggestPath(names), &out)
	return out, err
}

// SuggestCommunitiesRaw returns the raw community elements for --json.
func (c *Client) SuggestCommunitiesRaw(names []string) ([]json.RawMessage, error) {
	return c.GetList(communitySuggestPath(names))
}

// ReportRequest is the body of POST /ad_accounts/{id}/reports. Dates must be
// YYYY-MM-DDTHH:00:00Z (hourly granularity).
type ReportRequest struct {
	StartsAt   string   `json:"starts_at"`
	EndsAt     string   `json:"ends_at"`
	Fields     []string `json:"fields"`
	Breakdowns []string `json:"breakdowns,omitempty"`
	TimeZoneID string   `json:"time_zone_id,omitempty"`
}

// Report runs a report and returns the raw metric rows. Each row is a flat object
// keyed by the lowercase field name plus any breakdown keys.
func (c *Client) Report(accountID string, req ReportRequest) ([]map[string]json.RawMessage, error) {
	var env struct {
		Data struct {
			Metrics []map[string]json.RawMessage `json:"metrics"`
		} `json:"data"`
	}
	if err := c.Post("/ad_accounts/"+accountID+"/reports", map[string]any{"data": req}, &env); err != nil {
		return nil, err
	}
	return env.Data.Metrics, nil
}

// ReportRaw runs a report and returns the response body verbatim for --json.
func (c *Client) ReportRaw(accountID string, req ReportRequest) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.Post("/ad_accounts/"+accountID+"/reports", map[string]any{"data": req}, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 404
	}
	return false
}
