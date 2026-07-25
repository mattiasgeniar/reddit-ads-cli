package api

import (
	"encoding/json"
	"testing"
)

func TestAddToTargetingListPreservesSiblings(t *testing.T) {
	// gender and interests must survive an edit to communities untouched.
	in := json.RawMessage(`{"gender":["FEMALE"],"interests":["INT_1"],"communities":["t5_a"]}`)
	out, err := AddToTargetingList(in, FieldCommunities, "t5_b")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["gender"]; !ok {
		t.Error("gender sibling was dropped")
	}
	if _, ok := got["interests"]; !ok {
		t.Error("interests sibling was dropped")
	}
	comms := toStrings(got["communities"])
	if len(comms) != 2 || comms[0] != "t5_a" || comms[1] != "t5_b" {
		t.Errorf("communities = %v, want [t5_a t5_b]", comms)
	}
}

func TestAddToTargetingListDeduplicates(t *testing.T) {
	in := json.RawMessage(`{"communities":["t5_a"]}`)
	out, _ := AddToTargetingList(in, FieldCommunities, "t5_a", "t5_a")
	vals, _ := TargetingListValues(out, FieldCommunities)
	if len(vals) != 1 {
		t.Errorf("expected dedup to 1 entry, got %v", vals)
	}
}

func TestRemoveFromTargetingList(t *testing.T) {
	in := json.RawMessage(`{"communities":["t5_a","t5_b","t5_c"]}`)
	out, err := RemoveFromTargetingList(in, FieldCommunities, "t5_b")
	if err != nil {
		t.Fatal(err)
	}
	vals, _ := TargetingListValues(out, FieldCommunities)
	if len(vals) != 2 || contains(vals, "t5_b") {
		t.Errorf("expected t5_b removed, got %v", vals)
	}
}

func TestAddToNilTargeting(t *testing.T) {
	out, err := AddToTargetingList(nil, FieldExcludedCommunities, "t5_x")
	if err != nil {
		t.Fatal(err)
	}
	vals, _ := TargetingListValues(out, FieldExcludedCommunities)
	if len(vals) != 1 || vals[0] != "t5_x" {
		t.Errorf("expected [t5_x], got %v", vals)
	}
}

func TestAddToTargetingListNullField(t *testing.T) {
	// keywords: null is a real shape Reddit returns; adding to communities must cope.
	in := json.RawMessage(`{"keywords":null,"communities":null}`)
	out, err := AddToTargetingList(in, FieldCommunities, "t5_a")
	if err != nil {
		t.Fatal(err)
	}
	vals, _ := TargetingListValues(out, FieldCommunities)
	if len(vals) != 1 || vals[0] != "t5_a" {
		t.Errorf("expected [t5_a], got %v", vals)
	}
}

func toStrings(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
