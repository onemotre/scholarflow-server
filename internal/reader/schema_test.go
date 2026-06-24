package reader

import (
	"encoding/json"
	"testing"
)

func TestValidatePaperCardRequiresCoreFields(t *testing.T) {
	card := PaperCard{
		Background:     "background",
		Problem:        "problem",
		Method:         "method",
		Implementation: "implementation",
	}

	if err := card.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidatePaperCardRejectsLimitations(t *testing.T) {
	raw := map[string]any{
		"background":  "ok",
		"problem":     "ok",
		"limitations": "not allowed",
	}

	if err := ValidateRawKeys(raw); err == nil {
		t.Fatal("ValidateRawKeys returned nil, want error")
	}
}

func TestCardJSONSchemaIsStrict(t *testing.T) {
	schema := cardJSONSchema()
	if schema["additionalProperties"] != false {
		t.Fatalf("root additionalProperties = %v, want false", schema["additionalProperties"])
	}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("required is %T, want []string", schema["required"])
	}
	for _, key := range []string{"background", "problem", "method", "implementation", "benchmarks", "baselines", "results", "code_links", "data_links", "figures", "evidence"} {
		found := false
		for _, r := range required {
			if r == key {
				found = true
			}
		}
		if !found {
			t.Fatalf("required missing %q", key)
		}
	}
	// Must round-trip through json.Marshal (no unmarshalable values).
	if _, err := json.Marshal(schema); err != nil {
		t.Fatalf("schema does not marshal: %v", err)
	}
	// Evidence page must allow null.
	props := schema["properties"].(map[string]any)
	evidence := props["evidence"].(map[string]any)
	item := evidence["items"].(map[string]any)
	evProps := item["properties"].(map[string]any)
	page := evProps["page"].(map[string]any)
	types := page["type"].([]string)
	if len(types) != 2 || types[0] != "integer" || types[1] != "null" {
		t.Fatalf("page type = %v, want [integer null]", types)
	}
	// Evidence must carry a nullable claim_index.
	ci := evProps["claim_index"].(map[string]any)
	if ciTypes := ci["type"].([]string); len(ciTypes) != 2 || ciTypes[1] != "null" {
		t.Fatalf("claim_index type = %v, want [integer null]", ci["type"])
	}
	// Figures items require label, claim_key, claim_index.
	figure := props["figures"].(map[string]any)["items"].(map[string]any)
	figReq := figure["required"].([]string)
	for _, key := range []string{"label", "claim_key", "claim_index"} {
		found := false
		for _, r := range figReq {
			if r == key {
				found = true
			}
		}
		if !found {
			t.Fatalf("figure required missing %q", key)
		}
	}
}
