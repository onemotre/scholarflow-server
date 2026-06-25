package reader

import (
	"encoding/json"
	"testing"
)

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestValidatePaperCardRequiresCoreFields(t *testing.T) {
	if err := (PaperCard{Introduction: "intro"}).Validate(); err != nil {
		t.Fatalf("introduction-only card should validate: %v", err)
	}
	if err := (PaperCard{Methodology: []MethodologyItem{{Problem: "p", Method: "m"}}}).Validate(); err != nil {
		t.Fatalf("methodology-only card should validate: %v", err)
	}
	if err := (PaperCard{}).Validate(); err == nil {
		t.Fatal("empty card should fail validation")
	}
}

func TestValidatePaperCardRejectsLimitations(t *testing.T) {
	raw := map[string]any{"introduction": "ok", "limitations": "not allowed"}
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
	for _, key := range []string{"introduction", "related_work", "methodology", "results", "implementation", "code_links", "data_links", "figures", "evidence"} {
		if !contains(required, key) {
			t.Fatalf("root required missing %q", key)
		}
	}
	if _, err := json.Marshal(schema); err != nil {
		t.Fatalf("schema does not marshal: %v", err)
	}
	props := schema["properties"].(map[string]any)

	resultItem := props["results"].(map[string]any)["items"].(map[string]any)
	if resultItem["additionalProperties"] != false {
		t.Fatalf("results item additionalProperties = %v, want false", resultItem["additionalProperties"])
	}
	for _, key := range []string{"metric", "finding", "comparisons", "self_only"} {
		if !contains(resultItem["required"].([]string), key) {
			t.Fatalf("results item required missing %q", key)
		}
	}
	comparison := resultItem["properties"].(map[string]any)["comparisons"].(map[string]any)["items"].(map[string]any)
	for _, key := range []string{"work", "value", "reference"} {
		if !contains(comparison["required"].([]string), key) {
			t.Fatalf("comparison required missing %q", key)
		}
	}

	impl := props["implementation"].(map[string]any)
	if impl["additionalProperties"] != false {
		t.Fatalf("implementation additionalProperties = %v, want false", impl["additionalProperties"])
	}
	for _, key := range []string{"overview", "modules"} {
		if !contains(impl["required"].([]string), key) {
			t.Fatalf("implementation required missing %q", key)
		}
	}
	module := impl["properties"].(map[string]any)["modules"].(map[string]any)["items"].(map[string]any)
	for _, key := range []string{"name", "function", "design", "principle"} {
		if !contains(module["required"].([]string), key) {
			t.Fatalf("module required missing %q", key)
		}
	}

	methItem := props["methodology"].(map[string]any)["items"].(map[string]any)
	for _, key := range []string{"problem", "method"} {
		if !contains(methItem["required"].([]string), key) {
			t.Fatalf("methodology item required missing %q", key)
		}
	}

	evProps := props["evidence"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if types := evProps["page"].(map[string]any)["type"].([]string); len(types) != 2 || types[0] != "integer" || types[1] != "null" {
		t.Fatalf("page type = %v, want [integer null]", evProps["page"])
	}
	if types := evProps["claim_index"].(map[string]any)["type"].([]string); len(types) != 2 || types[1] != "null" {
		t.Fatalf("claim_index type = %v, want [integer null]", evProps["claim_index"])
	}

	figure := props["figures"].(map[string]any)["items"].(map[string]any)
	for _, key := range []string{"label", "claim_key", "claim_index"} {
		if !contains(figure["required"].([]string), key) {
			t.Fatalf("figure required missing %q", key)
		}
	}
}
