package api

import "encoding/json"

// The API wraps single objects as {"data": {...}}. dataEnvelope unwraps that.
type dataEnvelope[T any] struct {
	Data T `json:"data"`
}

// Me is the subset of GET /me used by `auth status` to show who a token belongs to.
type Me struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// Business is one entry from GET /me/businesses.
type Business struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AdAccount is the subset of an ad account we render. Money fields are microcurrency.
type AdAccount struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Currency   string `json:"currency"`
	Type       string `json:"type"`
	BusinessID string `json:"business_id"`
	TimeZoneID string `json:"time_zone_id"`
}

// Campaign is the subset of a campaign we render or edit. bid_value, goal_value and
// spend_cap are microcurrency integers and nullable, so they are pointers.
type Campaign struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	ConfiguredStatus  string `json:"configured_status"`
	EffectiveStatus   string `json:"effective_status"`
	Objective         string `json:"objective"`
	SpendCap          *int64 `json:"spend_cap"`
	GoalValue         *int64 `json:"goal_value"`
	GoalType          string `json:"goal_type"`
	BidValue          *int64 `json:"bid_value"`
	IsCBO             bool   `json:"is_campaign_budget_optimization"`
	ConversionPixelID string `json:"conversion_pixel_id"`
	AdAccountID       string `json:"ad_account_id"`
}

// AdGroup is the subset of an ad group we render or edit. The targeting block is kept
// as raw JSON so a read-modify-write of one field never drops sibling fields we do
// not model.
type AdGroup struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	ConfiguredStatus string          `json:"configured_status"`
	EffectiveStatus  string          `json:"effective_status"`
	CampaignID       string          `json:"campaign_id"`
	BidValue         *int64          `json:"bid_value"`
	GoalType         string          `json:"goal_type"`
	GoalValue        *int64          `json:"goal_value"`
	Targeting        json.RawMessage `json:"targeting"`
}

// Ad is the subset of an ad we render or edit.
type Ad struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	ConfiguredStatus    string          `json:"configured_status"`
	EffectiveStatus     string          `json:"effective_status"`
	AdGroupID           string          `json:"ad_group_id"`
	CampaignID          string          `json:"campaign_id"`
	PostID              string          `json:"post_id"`
	ClickURL            string          `json:"click_url"`
	ClickURLQueryParams json.RawMessage `json:"click_url_query_parameters"`
	ProfileID           string          `json:"profile_id"`
}

// Community is one result from the community search/suggest endpoints. id is the
// targeting id used in ad group targeting.communities[].
type Community struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	SubscriberCount int64  `json:"subscriber_count"`
	Description     string `json:"description"`
}
