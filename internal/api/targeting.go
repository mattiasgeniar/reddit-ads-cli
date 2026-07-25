package api

import (
	"encoding/json"
	"sort"
)

// Sending an ad group's `targeting` object replaces it wholesale, so every edit is a
// read-modify-write: fetch the current targeting, change exactly one field, send it
// back whole. To guarantee we never drop a sibling field we do not model (gender,
// view_modes, custom audiences, ...), the object is decoded into a map of raw
// messages and only the one edited key is touched. Every other field round-trips
// byte-for-byte.

// TargetingListField is a targeting key whose value is an array of string IDs.
type TargetingListField string

const (
	FieldCommunities         TargetingListField = "communities"
	FieldExcludedCommunities TargetingListField = "excluded_communities"
)

// AddToTargetingList adds ids to the given string-array field, de-duplicating, and
// returns the full targeting object with only that field changed. A nil or empty
// targeting input yields an object containing just the edited field.
func AddToTargetingList(targeting json.RawMessage, field TargetingListField, ids ...string) (json.RawMessage, error) {
	obj, current, err := decodeTargetingList(targeting, field)
	if err != nil {
		return nil, err
	}

	set := make(map[string]bool, len(current))
	for _, v := range current {
		set[v] = true
	}
	for _, id := range ids {
		set[id] = true
	}
	return encodeTargetingList(obj, field, set)
}

// RemoveFromTargetingList removes ids from the given string-array field and returns
// the full targeting object with only that field changed.
func RemoveFromTargetingList(targeting json.RawMessage, field TargetingListField, ids ...string) (json.RawMessage, error) {
	obj, current, err := decodeTargetingList(targeting, field)
	if err != nil {
		return nil, err
	}

	remove := make(map[string]bool, len(ids))
	for _, id := range ids {
		remove[id] = true
	}
	set := make(map[string]bool, len(current))
	for _, v := range current {
		if !remove[v] {
			set[v] = true
		}
	}
	return encodeTargetingList(obj, field, set)
}

// TargetingListValues returns the current string values of a targeting array field.
func TargetingListValues(targeting json.RawMessage, field TargetingListField) ([]string, error) {
	_, current, err := decodeTargetingList(targeting, field)
	return current, err
}

func decodeTargetingList(targeting json.RawMessage, field TargetingListField) (map[string]json.RawMessage, []string, error) {
	obj := map[string]json.RawMessage{}
	if len(targeting) > 0 && string(targeting) != "null" {
		if err := json.Unmarshal(targeting, &obj); err != nil {
			return nil, nil, err
		}
	}
	var current []string
	if raw, ok := obj[string(field)]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, &current); err != nil {
			return nil, nil, err
		}
	}
	return obj, current, nil
}

func encodeTargetingList(obj map[string]json.RawMessage, field TargetingListField, set map[string]bool) (json.RawMessage, error) {
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out) // stable output so dry-run diffs are readable

	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	obj[string(field)] = raw
	return json.Marshal(obj)
}
