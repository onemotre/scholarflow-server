package papers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPaperSummaryMarshalsSourceFields(t *testing.T) {
	sid := "2301.00001"
	cat := "cs.CL"
	b, err := json.Marshal(PaperSummary{
		Status:          "completed",
		SourceType:      "arxiv",
		SourceID:        &sid,
		PrimaryCategory: &cat,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(b)
	for _, want := range []string{`"source_type":"arxiv"`, `"source_id":"2301.00001"`, `"primary_category":"cs.CL"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %s", want, out)
		}
	}
}
