# Card Schema v3 + Prompt Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure the reader's paper-card contract from the flat 2.0 schema into the richer v3 schema (introduction / related_work / problem→method methodology / metric-centric results / module-based implementation) and rewrite both prompts to drive it.

**Architecture:** Change the `PaperCard` Go struct + its strict OpenAI `cardJSONSchema` + `Validate` in `internal/reader/schema.go`, bump the schema-version constant, fix the test/fake call sites that build cards, then rewrite `prompts/system.md` and `prompts/system_zh.md`. The card is a JSON blob and evidence rows use a free-string `claim_key`, so there is no DB migration and no change to persistence or page-resolution logic.

**Tech Stack:** Go (module `scholarflow_server`), OpenAI Structured Outputs (strict `json_schema`), standard `testing`.

## Global Constraints

- Module path is `scholarflow_server`; run `go fmt ./...` before committing.
- No DB migration. `SavePaperCard` (marshals the card) and `resolveCardPages` (operate on `Evidence`/`Figures`/`claim_key`) are generic and must NOT change beyond the schema-version constant.
- `limitations` remains a forbidden top-level key (`ValidateRawKeys` unchanged).
- Strict OpenAI schema discipline: every object has `additionalProperties:false` and lists every property in `required`; unknown values are empty strings / empty arrays / `false` / null — never invented.
- Web viewer (`scholarflow-web`) is out of scope; new v3 cards render with missing sections there until a later slice.
- Tests use the standard `testing` package, co-located `*_test.go`, no Docker/network.
- Both prompts keep the opening phrase `objective research-paper summarizer`; `system_zh.md` keeps its Language-requirements block (English keys, Simplified-Chinese values, preserve proper nouns / model / dataset / benchmark names / URLs / math notation).
- Anchorable `claim_key` values: `introduction`, `related_work`, `methodology` (list), `results` (list), `implementation` (scalar overview → `claim_index:null`), `modules` (list). Figures anchor a metric's chart with `claim_key:"results"` and the architecture diagram with `claim_key:"implementation"`.

---

### Task 1: v3 card schema (Go struct, strict schema, validation)

**Files:**
- Modify: `internal/reader/schema.go` (replace `PaperCard` + add nested types; update `Validate`; rewrite `cardJSONSchema`)
- Modify: `internal/reader/schema_test.go` (Validate + strict-schema tests)
- Modify: `internal/reader/openai_test.go` (v3 JSON literals + asserts)
- Modify: `internal/reader/fake.go` (FakeReader builds a v3 card)
- Modify: `internal/jobs/read_pipeline.go:15` (`cardSchemaVersion` → `"3.0"`)
- Modify: `internal/jobs/read_pipeline_test.go` (two fakeReader cards + one assert)

**Interfaces:**
- Produces (consumed by Task 2's prompts and by existing pipeline/persistence code unchanged):
  - `PaperCard{Introduction, RelatedWork string; Methodology []MethodologyItem; Results []ResultItem; Implementation Implementation; CodeLinks, DataLinks []string; Figures []FigureRef; Evidence []Evidence}`
  - `MethodologyItem{Problem, Method string}`
  - `Comparison{Work, Value, Reference string}`
  - `ResultItem{Metric, Finding string; Comparisons []Comparison; SelfOnly bool}`
  - `Module{Name, Function, Design, Principle string}`
  - `Implementation{Overview string; Modules []Module}`
  - `FigureRef`, `Evidence`, `Context`, `Section`, `Figure`, `Reader` interface: unchanged.
  - `(PaperCard).Validate()` passes when `Introduction != ""` OR `len(Methodology) > 0`.

- [ ] **Step 1: Update the Validate + strict-schema tests (RED)**

Replace the body of `internal/reader/schema_test.go` with:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/reader/ -run 'TestValidatePaperCard|TestCardJSONSchema' -v`
Expected: FAIL — `undefined: MethodologyItem` (and the strict-schema assertions fail against the old schema).

- [ ] **Step 3: Replace the struct, Validate, and cardJSONSchema**

In `internal/reader/schema.go`, replace the `PaperCard` struct (the `type PaperCard struct {...}` block) with these types (keep `Evidence` and `FigureRef` as they are, above `PaperCard`):

```go
type MethodologyItem struct {
	Problem string `json:"problem"`
	Method  string `json:"method"`
}

type Comparison struct {
	Work      string `json:"work"`
	Value     string `json:"value"`
	Reference string `json:"reference"`
}

type ResultItem struct {
	Metric      string       `json:"metric"`
	Finding     string       `json:"finding"`
	Comparisons []Comparison `json:"comparisons"`
	SelfOnly    bool         `json:"self_only"`
}

type Module struct {
	Name      string `json:"name"`
	Function  string `json:"function"`
	Design    string `json:"design"`
	Principle string `json:"principle"`
}

type Implementation struct {
	Overview string   `json:"overview"`
	Modules  []Module `json:"modules"`
}

type PaperCard struct {
	Introduction   string            `json:"introduction"`
	RelatedWork    string            `json:"related_work"`
	Methodology    []MethodologyItem `json:"methodology"`
	Results        []ResultItem      `json:"results"`
	Implementation Implementation    `json:"implementation"`
	CodeLinks      []string          `json:"code_links"`
	DataLinks      []string          `json:"data_links"`
	Figures        []FigureRef       `json:"figures"`
	Evidence       []Evidence        `json:"evidence"`
}
```

Replace `Validate` with:

```go
func (c PaperCard) Validate() error {
	if c.Introduction == "" && len(c.Methodology) == 0 {
		return fmt.Errorf("paper card has no core content")
	}
	return nil
}
```

Replace the entire `cardJSONSchema` function with:

```go
func cardJSONSchema() map[string]any {
	str := func() map[string]any { return map[string]any{"type": "string"} }
	strArray := func() map[string]any {
		return map[string]any{"type": "array", "items": str()}
	}
	obj := func(required []string, props map[string]any) map[string]any {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             required,
			"properties":           props,
		}
	}
	arrayOf := func(item map[string]any) map[string]any {
		return map[string]any{"type": "array", "items": item}
	}

	methodologyItem := obj([]string{"problem", "method"}, map[string]any{
		"problem": str(),
		"method":  str(),
	})
	comparison := obj([]string{"work", "value", "reference"}, map[string]any{
		"work":      str(),
		"value":     str(),
		"reference": str(),
	})
	resultItem := obj([]string{"metric", "finding", "comparisons", "self_only"}, map[string]any{
		"metric":      str(),
		"finding":     str(),
		"comparisons": arrayOf(comparison),
		"self_only":   map[string]any{"type": "boolean"},
	})
	module := obj([]string{"name", "function", "design", "principle"}, map[string]any{
		"name":      str(),
		"function":  str(),
		"design":    str(),
		"principle": str(),
	})
	implementation := obj([]string{"overview", "modules"}, map[string]any{
		"overview": str(),
		"modules":  arrayOf(module),
	})
	evidence := obj(
		[]string{"claim_key", "claim_index", "evidence_type", "section_id", "asset_id", "page", "locator", "snippet", "confidence"},
		map[string]any{
			"claim_key":     str(),
			"claim_index":   map[string]any{"type": []string{"integer", "null"}},
			"evidence_type": str(),
			"section_id":    str(),
			"asset_id":      str(),
			"page":          map[string]any{"type": []string{"integer", "null"}},
			"locator":       str(),
			"snippet":       str(),
			"confidence":    map[string]any{"type": "number"},
		},
	)
	figure := obj([]string{"label", "claim_key", "claim_index"}, map[string]any{
		"label":       str(),
		"claim_key":   str(),
		"claim_index": map[string]any{"type": []string{"integer", "null"}},
	})
	return obj(
		[]string{"introduction", "related_work", "methodology", "results", "implementation", "code_links", "data_links", "figures", "evidence"},
		map[string]any{
			"introduction":   str(),
			"related_work":   str(),
			"methodology":    arrayOf(methodologyItem),
			"results":        arrayOf(resultItem),
			"implementation": implementation,
			"code_links":     strArray(),
			"data_links":     strArray(),
			"figures":        arrayOf(figure),
			"evidence":       arrayOf(evidence),
		},
	)
}
```

Leave `ErrDisallowedKey`, `ValidateRawKeys`, `Context`, `Section`, `Figure`, `Reader`, `Evidence`, and `FigureRef` unchanged.

- [ ] **Step 4: Run the schema tests to verify they pass**

Run: `go test ./internal/reader/ -run 'TestValidatePaperCard|TestCardJSONSchema' -v`
Expected: PASS. (The rest of the `reader` package will not compile yet — fixed in Step 5.)

- [ ] **Step 5: Fix the remaining card-building sites in `internal/reader`**

In `internal/reader/fake.go`, replace the `ReadPaper` body:

```go
func (FakeReader) ReadPaper(ctx context.Context, input Context) (PaperCard, error) {
	card := PaperCard{
		Introduction: input.Abstract,
		Methodology:  []MethodologyItem{{Problem: "not found", Method: "not found"}},
	}
	return card, card.Validate()
}
```

In `internal/reader/openai_test.go`, update each card literal and assertion:

- `TestOpenAIReaderParsesCard`:
```go
	card := `{"introduction":"intro","methodology":[{"problem":"p","method":"m"}],"evidence":[{"claim_key":"methodology","claim_index":0,"evidence_type":"section","section_id":"1","confidence":0.8}]}`
```
and replace the field assertion block with:
```go
	if got.Introduction != "intro" {
		t.Fatalf("introduction = %q", got.Introduction)
	}
	if len(got.Methodology) != 1 || got.Methodology[0].Method != "m" {
		t.Fatalf("methodology = %#v", got.Methodology)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].SectionID != "1" {
		t.Fatalf("evidence = %#v", got.Evidence)
	}
```

- `TestOpenAIReaderRejectsLimitations`: change the literal to
```go
	card := `{"introduction":"intro","limitations":"nope"}`
```

- `TestOpenAIReaderRetriesBadJSON`: change `good` and the assertion:
```go
	good := `{"introduction":"intro"}`
```
```go
	if got.Introduction != "intro" {
		t.Fatalf("introduction = %q", got.Introduction)
	}
```

- `TestResponsesStyleParsesCard`, `TestResponsesStyleHonorsOutputText`: change each `card` literal to `{"introduction":"intro"}` and each `if got.Background != "bg"` block to:
```go
	if got.Introduction != "intro" {
		t.Fatalf("introduction = %q", got.Introduction)
	}
```

- `TestResponsesJSONSchemaRequestShape`, `TestChatJSONSchemaRequestShape`: change each `card` literal to `{"introduction":"intro"}` (these tests assert on the request body, not card fields, so no assertion change).

- [ ] **Step 6: Bump the schema version and fix the jobs tests**

In `internal/jobs/read_pipeline.go`, change line 15:
```go
const cardSchemaVersion = "3.0"
```

In `internal/jobs/read_pipeline_test.go`:

- The card around line 76 (in the persist test) becomes:
```go
	rdr := &fakeReader{card: reader.PaperCard{Introduction: "intro", Methodology: []reader.MethodologyItem{{Problem: "p", Method: "m"}}}}
```
- The assertion around line 86 becomes:
```go
	if len(repo.saved.Methodology) != 1 || repo.saved.Methodology[0].Method != "m" {
		t.Fatalf("saved methodology = %#v", repo.saved.Methodology)
	}
```
- The card in `TestReadPipelineResolvesPages` (lines 111-120) becomes:
```go
	rdr := &fakeReader{card: reader.PaperCard{
		Introduction: "intro",
		Results:      []reader.ResultItem{{Metric: "acc", Finding: "r0"}},
		Figures:      []reader.FigureRef{{Label: "figure 2", ClaimKey: "results", ClaimIndex: intPtr(0)}},
		Evidence: []reader.Evidence{
			{ClaimKey: "results", SectionID: "1", Page: &ev0},
			{ClaimKey: "results", SectionID: "1", Page: &ev1},
			{ClaimKey: "methodology", SectionID: "999"}, // unknown section -> untouched
		},
	}}
```

- [ ] **Step 7: Run the full suite and build to verify green**

Run: `go test ./... 2>&1 | grep -v "no test files"`
Expected: every package `ok` (notably `internal/reader`, `internal/jobs`, `internal/httpapi`).

Run: `go build ./...`
Expected: builds with no error.

- [ ] **Step 8: Commit**

```bash
go fmt ./...
git add internal/reader/schema.go internal/reader/schema_test.go internal/reader/openai_test.go internal/reader/fake.go internal/jobs/read_pipeline.go internal/jobs/read_pipeline_test.go
git commit -m "feat(reader): restructure paper card into v3 schema"
```

---

### Task 2: Rewrite both prompts for v3

**Files:**
- Modify: `internal/reader/prompts/system.md` (full rewrite)
- Modify: `internal/reader/prompts/system_zh.md` (full rewrite)
- Modify: `internal/reader/prompt_test.go` (add v3-content assertions for both prompts)

**Interfaces:**
- Consumes: the v3 field names from Task 1 (`introduction`, `related_work`, `methodology`, `results`/`comparisons`/`self_only`, `implementation`/`modules`).
- Produces: nothing other tasks consume (terminal task).

- [ ] **Step 1: Add the failing prompt-content tests (RED)**

Append to `internal/reader/prompt_test.go`:

```go
func TestDefaultPromptDescribesV3(t *testing.T) {
	got := LoadSystemPrompt("")
	for _, marker := range []string{"introduction", "related_work", "methodology", "self_only", "modules"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("default prompt missing v3 marker %q", marker)
		}
	}
	if strings.Contains(got, "\"background\"") {
		t.Fatalf("default prompt still references removed 2.0 field \"background\"")
	}
}

func TestChinesePromptDescribesV3(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("prompts", "system_zh.md"))
	if err != nil {
		t.Fatalf("read system_zh.md: %v", err)
	}
	got := string(data)
	for _, marker := range []string{"introduction", "related_work", "methodology", "self_only", "modules", "Simplified Chinese"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("system_zh.md missing marker %q", marker)
		}
	}
}
```

(`os`, `path/filepath`, and `strings` are already imported by `prompt_test.go`.)

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/reader/ -run 'PromptDescribesV3' -v`
Expected: FAIL — the current prompts still describe the 2.0 fields (`background`, etc.) and lack the v3 markers.

- [ ] **Step 3: Rewrite `prompts/system.md`**

Replace the entire contents of `internal/reader/prompts/system.md` with:

```
You are an objective research-paper summarizer. Produce a strict JSON object describing the paper.

Rules:
- Output ONLY a JSON object, no prose, no code fences.
- Never include a "limitations" field.
- Be factual; do not speculate. Use empty strings, empty arrays, or false when unknown. Do not invent works, metrics, results, URLs, figures, or evidence.

Fields:
- "introduction": state the problem the paper sets out to solve and why it matters. Merge the background and the problem statement into one clear explanation.
- "related_work": how prior work has approached this problem; use an empty string if the paper does not discuss it.
- "methodology": a list of {"problem","method"} pairs. Each entry names a specific problem and the method the paper uses to solve it; map each method to the problem it addresses.
- "results": a list of metric-centric entries, one per metric the authors use to show their work is better. Each is {"metric","finding","comparisons","self_only"}:
  - "metric": the metric or evaluation axis (e.g. accuracy, F1, latency, a dataset score).
  - "finding": what the paper reports on this metric.
  - "comparisons": a list of {"work","value","reference"} for each competing method compared on this metric. "work" is the competing method's name, "value" its reported number or relation if given, "reference" its citation (e.g. "[12]") when available, otherwise "".
  - "self_only": true when only the authors' own method is evaluated on this metric (no competitor compared), otherwise false.
- "implementation": {"overview","modules"}. "overview" describes the overall system design. "modules" is a list of {"name","function","design","principle"}: for each module give what it does ("function"), how it is built ("design"), and the underlying principle, including formulas where present ("principle").
- "code_links" / "data_links": URLs the paper provides for code or data.
- "figures": place each important figure or table at the claim it illustrates. Each entry is {"label","claim_key","claim_index"}, where "label" is the figure label exactly as given in the input. Put a metric's figure or chart at the result it shows ("claim_key":"results", "claim_index": that result's 0-based position). Put the system architecture diagram at the implementation ("claim_key":"implementation", "claim_index":null). Do NOT set a page for figures.
- "evidence": for each non-trivial claim, add an entry anchoring it to its source. "claim_key" is the field it supports ("introduction","related_work","methodology","results","implementation","modules"); "claim_index" is the 0-based item position for the list fields ("methodology","results","modules") or null for the scalar fields ("introduction","related_work","implementation"). "section_id" is the SECTION LABEL (the number shown in brackets) that supports the claim. Set "page" to the page where the supporting text appears (use the section's page range as a guide) or null if unknown. Use "evidence_type":"section" for text evidence and copy a short supporting "snippet" when helpful.

JSON shape:
{"introduction":"","related_work":"","methodology":[{"problem":"","method":""}],"results":[{"metric":"","finding":"","comparisons":[{"work":"","value":"","reference":""}],"self_only":false}],"implementation":{"overview":"","modules":[{"name":"","function":"","design":"","principle":""}]},"code_links":[],"data_links":[],"figures":[{"label":"Figure 2","claim_key":"results","claim_index":0}],"evidence":[{"claim_key":"results","claim_index":0,"evidence_type":"section","section_id":"<label>","asset_id":"","page":null,"locator":"","snippet":"","confidence":0.0}]}
```

- [ ] **Step 4: Rewrite `prompts/system_zh.md`**

Replace the entire contents of `internal/reader/prompts/system_zh.md` with:

```
You are an objective research-paper summarizer. Produce a strict JSON object describing the paper.

Language requirements:

- Keep all JSON keys exactly as specified in English.
- Write all human-readable field values in Simplified Chinese.
- Translate technical explanations into precise, natural Simplified Chinese.
- Keep official proper nouns, model names, dataset names, benchmark names, method names, paper titles, URLs, code identifiers, and mathematical notation in their original language unless there is a standard Chinese translation.
- Evidence snippets may be copied from the source text in the original language when needed for traceability, but explanatory claims must be in Simplified Chinese.

Rules:

- Output ONLY a JSON object, no prose, no code fences.
- Never include a "limitations" field.
- Be factual; do not speculate. Use empty strings, empty arrays, or false when unknown. Do not invent works, metrics, results, URLs, figures, or evidence.
- If the paper text is English, summarize and translate the meaning into Chinese rather than doing sentence-by-sentence literal translation.

Fields:
- "introduction": state the problem the paper sets out to solve and why it matters. Merge the background and the problem statement into one clear explanation.
- "related_work": how prior work has approached this problem; use an empty string if the paper does not discuss it.
- "methodology": a list of {"problem","method"} pairs. Each entry names a specific problem and the method the paper uses to solve it; map each method to the problem it addresses.
- "results": a list of metric-centric entries, one per metric the authors use to show their work is better. Each is {"metric","finding","comparisons","self_only"}:
  - "metric": the metric or evaluation axis (e.g. accuracy, F1, latency, a dataset score).
  - "finding": what the paper reports on this metric.
  - "comparisons": a list of {"work","value","reference"} for each competing method compared on this metric. "work" is the competing method's name, "value" its reported number or relation if given, "reference" its citation (e.g. "[12]") when available, otherwise "".
  - "self_only": true when only the authors' own method is evaluated on this metric (no competitor compared), otherwise false.
- "implementation": {"overview","modules"}. "overview" describes the overall system design. "modules" is a list of {"name","function","design","principle"}: for each module give what it does ("function"), how it is built ("design"), and the underlying principle, including formulas where present ("principle").
- "code_links" / "data_links": URLs the paper provides for code or data.
- "figures": place each important figure or table at the claim it illustrates. Each entry is {"label","claim_key","claim_index"}, where "label" is the figure label exactly as given in the input. Put a metric's figure or chart at the result it shows ("claim_key":"results", "claim_index": that result's 0-based position). Put the system architecture diagram at the implementation ("claim_key":"implementation", "claim_index":null). Do NOT set a page for figures.
- "evidence": for each non-trivial claim, add an entry anchoring it to its source. "claim_key" is the field it supports ("introduction","related_work","methodology","results","implementation","modules"); "claim_index" is the 0-based item position for the list fields ("methodology","results","modules") or null for the scalar fields ("introduction","related_work","implementation"). "section_id" is the SECTION LABEL (the number shown in brackets) that supports the claim. Set "page" to the page where the supporting text appears (use the section's page range as a guide) or null if unknown. Use "evidence_type":"section" for text evidence and copy a short supporting "snippet" when helpful.

JSON shape:
{"introduction":"","related_work":"","methodology":[{"problem":"","method":""}],"results":[{"metric":"","finding":"","comparisons":[{"work":"","value":"","reference":""}],"self_only":false}],"implementation":{"overview":"","modules":[{"name":"","function":"","design":"","principle":""}]},"code_links":[],"data_links":[],"figures":[{"label":"Figure 2","claim_key":"results","claim_index":0}],"evidence":[{"claim_key":"results","claim_index":0,"evidence_type":"section","section_id":"<label>","asset_id":"","page":null,"locator":"","snippet":"","confidence":0.0}]}
```

- [ ] **Step 5: Run the prompt tests to verify they pass**

Run: `go test ./internal/reader/ -run 'Prompt' -v`
Expected: PASS (including the existing `TestLoadSystemPromptDefault`/`Override`/`UnreadableFallsBack` and the two new v3 tests).

- [ ] **Step 6: Run the full suite**

Run: `go test ./... 2>&1 | grep -v "no test files"`
Expected: every package `ok`.

- [ ] **Step 7: Commit**

```bash
go fmt ./...
git add internal/reader/prompts/system.md internal/reader/prompts/system_zh.md internal/reader/prompt_test.go
git commit -m "feat(reader): rewrite system prompts for v3 card schema"
```

---

## Self-Review

**Spec coverage:**
- v3 struct + nested types → Task 1 Step 3. ✓
- `Validate` core-content rule (introduction OR methodology) → Task 1 Steps 1, 3. ✓
- Strict nested `cardJSONSchema` (additionalProperties:false, required sub-fields, nullable claim_index/page) → Task 1 Steps 1, 3. ✓
- `limitations` still forbidden → kept in `ValidateRawKeys`; test retained (Task 1 Step 1). ✓
- `cardSchemaVersion` → "3.0" → Task 1 Step 6. ✓
- Persistence / page-resolution unchanged → no task touches them (only the version constant). ✓
- Both prompts rewritten with v3 fields + rules, opener kept, zh language block kept → Task 2 Steps 3, 4. ✓
- Figures anchored via `figures[]` (results / implementation), no per-field figure_label → reflected in struct (no figure_label fields) and prompt text. ✓
- Tests updated (schema_test, openai_test, fake, read_pipeline_test, prompt_test) → Tasks 1 & 2. ✓
- No DB migration; web deferred → no task; stated in Global Constraints. ✓

**Placeholder scan:** No TBD/TODO; every code/prompt step shows the full content.

**Type consistency:** `MethodologyItem`, `ResultItem`, `Comparison`, `Module`, `Implementation`, `PaperCard` field names and JSON tags in Task 1 Step 3 match the literals/asserts in Steps 1, 5, 6 (`Methodology[0].Method`, `Results[]reader.ResultItem{{Metric,Finding}}`, `Introduction`) and the prompt field names in Task 2. The `contains` test helper is defined once in `schema_test.go` and reused there only.
