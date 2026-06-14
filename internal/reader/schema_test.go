package reader

import "testing"

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
