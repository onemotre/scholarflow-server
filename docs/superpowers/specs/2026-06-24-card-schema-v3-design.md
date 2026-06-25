# Card Schema v3 + Prompt Refactor — Design

**Date:** 2026-06-24
**Status:** Approved (brainstorming)
**Module:** `scholarflow-server`

## Context

Part of the larger paper-reading quality effort:

- **P1** — Figure & diagram image extraction. *Done, merged.*
- **P2 (this spec)** — Card schema v3 + prompt refactor: restructure the reader's
  paper-card contract for deeper, better-grounded output.
- **Next slice — self-refine loop** — the reader produces a draft v3 card, then runs
  N refinement passes (model sees its own draft + paper, asked to deepen / correct /
  ground evidence) before returning. Single-agent iterative refinement; configurable
  iteration count, toggleable. *Deferred; needs the v3 schema to exist first.*
- **Later** — web viewer rendering of v3, and (optionally) the full
  Analyst→Explainer→Synthesizer multi-agent reader.

### Current state (schema 2.0)

`internal/reader/schema.go` defines a flat `PaperCard`: `background`, `problem`,
`method`, `implementation` (scalars), `benchmarks`, `baselines`, `results`,
`code_links`, `data_links` (string arrays), plus `figures` (figure anchors) and
`evidence` (claim-anchored). The single LLM call enforces the shape via OpenAI
Structured Outputs (`cardJSONSchema`, strict). Two prompts drive it:
`prompts/system.md` (English) and `prompts/system_zh.md` (Chinese values, English
keys). The card is stored as a JSON blob in `paper_cards.content_json`; evidence
rows persist with a free-string `claim_key`.

### Why this change

The flat 2.0 fields produce shallow summaries and rough evidence. v3 makes the
structure carry the analysis: the introduction states the problem, methodology maps
each problem to the method that addresses it, results are metric-centric with explicit
competitor comparisons, and implementation is broken into modules with their design
principles and formulas.

## Scope

**In scope:** `internal/reader` (the `PaperCard` struct, `cardJSONSchema`,
`Validate`, `ValidateRawKeys`), both prompts, `internal/reader` tests + `fake.go`,
the `cardSchemaVersion` constant in `internal/jobs/read_pipeline.go`, and the
`internal/jobs` / `internal/reader` test updates.

**Out of scope (deferred):**

- The self-refine loop (reader orchestration) — next slice.
- The `scholarflow-web` viewer (`apiclient` Card struct, `paper.tmpl`, `view.go`) —
  it keeps reading 2.0 fields; new v3 cards render with missing sections until a
  follow-up updates it.
- Any DB migration. The card is a JSON blob and evidence `claim_key` is a free
  string, so no schema change is needed. Existing stored 2.0 cards remain 2.0 until
  their paper is re-read.

## The v3 schema

```json
{
  "introduction": "",
  "related_work": "",
  "methodology": [{"problem": "", "method": ""}],
  "results": [
    {
      "metric": "",
      "finding": "",
      "comparisons": [{"work": "", "value": "", "reference": ""}],
      "self_only": false
    }
  ],
  "implementation": {
    "overview": "",
    "modules": [{"name": "", "function": "", "design": "", "principle": ""}]
  },
  "code_links": [],
  "data_links": [],
  "figures": [{"label": "", "claim_key": "", "claim_index": null}],
  "evidence": [
    {
      "claim_key": "", "claim_index": null, "evidence_type": "section",
      "section_id": "", "asset_id": "", "page": null, "locator": "",
      "snippet": "", "confidence": 0.0
    }
  ]
}
```

### Mapping from 2.0

| 2.0 | v3 |
|-----|-----|
| `background` + `problem` | `introduction` |
| *(new)* | `related_work` |
| `method` | `methodology[]` (problem→method pairs) |
| `benchmarks` + `baselines` + `results[]string` | `results[]` (metric/finding + `comparisons[].work`) |
| `implementation` (string) | `implementation{overview, modules[]}` |
| `code_links`, `data_links`, `figures`, `evidence` | unchanged |

`principle` carries the module's underlying idea **including formulas** (LaTeX or
plain text). `comparisons[].reference` is the cited work's reference (e.g. `[12]` or
a title) where available, else empty. `self_only` is `true` for a result where the
authors evaluated only their own method (no competitor), directly flagging the
"tested only their own method" case.

### Evidence & figure anchoring

Anchoring keeps the existing `claim_key` + `claim_index` model (no persistence
change). Anchorable `claim_key` values:

- Scalars (`claim_index: null`): `introduction`, `related_work`, `implementation`
  (anchors the overview).
- Lists (`claim_index` = 0-based item): `methodology`, `results`, `modules`
  (the `implementation.modules` array), `comparisons` *is not separately anchored* —
  evidence for a comparison attaches to its parent `results` item.

Figures are placed with the same `figures[]` mechanism as today: a metric's figure
is `{"label": "...", "claim_key": "results", "claim_index": i}`; the system
architecture diagram is `{"label": "...", "claim_key": "implementation",
"claim_index": null}`. This reuses P1's server-side page resolution
(`resolveCardPages` by label) and avoids a second, redundant figure mechanism — so
the nested objects deliberately do **not** carry their own `figure_label` fields.

## Components

### `internal/reader/schema.go`

- Replace the `PaperCard` fields per the v3 shape. New nested types:
  `MethodologyItem{Problem, Method string}`,
  `ResultItem{Metric, Finding string; Comparisons []Comparison; SelfOnly bool}`,
  `Comparison{Work, Value, Reference string}`,
  `Implementation{Overview string; Modules []Module}`,
  `Module{Name, Function, Design, Principle string}`.
  Keep `FigureRef`, `Evidence`, `Context`, `Section`, `Figure` unchanged.
- `Validate()`: core content = non-empty `Introduction` **or** non-empty
  `Methodology`. (Was: any of background/problem/method/implementation.)
- `ValidateRawKeys`: keep the `limitations`-forbidden check.
- `cardJSONSchema()`: build the strict schema for the nested shape —
  `additionalProperties:false` everywhere, every property in `required`, nested
  object/array item schemas for methodology/results/comparisons/implementation/
  modules. Nullable `claim_index`/`page` preserved on evidence/figures.

### Prompts — `prompts/system.md` and `prompts/system_zh.md`

Both rewritten to describe the v3 shape and rules. Keep the opening phrase
"objective research-paper summarizer" (a test asserts it) and, for `system_zh.md`,
all existing language rules (English keys, Simplified-Chinese values, preserve proper
nouns / model / dataset / benchmark names / URLs / math notation; evidence snippets
may stay in the source language). New field guidance:

- `introduction`: merge background + problem; clearly state the problem the paper
  solves.
- `related_work`: how others have approached this problem.
- `methodology`: one entry per problem, mapping it to the method that addresses it.
- `results`: metric-centric. For each metric give the `finding`, list the competing
  works compared under `comparisons` (with `reference` where available), and set
  `self_only: true` when only the authors' own method was evaluated on that metric.
  Place the metric's figure/chart via `figures[]` (`claim_key: "results"`).
- `implementation`: `overview` of the system design; one `modules[]` entry per
  module with its `function`, `design`, and `principle` (include formulas in
  `principle`). Place the architecture diagram via `figures[]`
  (`claim_key: "implementation"`).
- Anchoring/evidence rules restated for the v3 `claim_key` values and list indices.
- Unchanged rules: output only the JSON object (no prose/fences); never include
  `limitations`; be factual, use empty strings/arrays when unknown; do not invent
  works, results, URLs, figures, or evidence.

End both prompts with the v3 `JSON shape:` example (matching the schema above).

### `internal/jobs/read_pipeline.go`

- `cardSchemaVersion` → `"3.0"`.
- `resolveCardPages` and `SavePaperCard` are unaffected: they operate generically on
  `card.Evidence` / `card.Figures` / `claim_key`, not on the renamed content fields.
  No logic change beyond the version constant.

## Data flow

Unchanged from 2.0: parsed `Context` → `OpenAIReader.ReadPaper` (single call, strict
v3 schema) → `parseCard` (`ValidateRawKeys` + `Validate`) → `resolveCardPages` →
`SavePaperCard` (content_json = v3 card, evidence rows by `claim_key`). Only the
card's internal shape and the prompts change.

## Error handling

- Strict Structured Outputs rejects extra/missing keys at the provider; `parseCard`
  still enforces `limitations`-forbidden and core-content validation, and the
  existing 2-attempt retry on parse failure is unchanged.
- Unknown values → empty strings / empty arrays / `false` (never invented).

## Testing

- `schema_test.go`: assert the new `required` set
  (`introduction, related_work, methodology, results, implementation, code_links,
  data_links, figures, evidence`); nested-object strictness
  (`additionalProperties:false`, required sub-fields on
  methodology/results/comparisons/implementation/modules); nullable
  `claim_index`/`page`; figures item requires `label, claim_key, claim_index`.
- New `Validate` tests: passes with only `introduction`; passes with only
  `methodology`; fails when both empty.
- `openai_test.go`: replace the 2.0 JSON literals with v3 cards; keep the
  `limitations`-rejected case; assert a representative nested field round-trips
  (e.g. `results[0].metric`, `methodology[0].problem`).
- `fake.go` (`FakeReader`): build a minimal valid v3 card (e.g. `Introduction` from
  the abstract, one `MethodologyItem`) so existing pipeline tests still pass.
- `prompt_test.go`: unchanged (still asserts the "objective research-paper
  summarizer" opener, which both rewritten prompts retain).
- Update any `internal/jobs` read-pipeline tests that construct a `PaperCard` with
  2.0 fields.

## Known caveats

- New v3 cards will not render fully in the current web viewer until the deferred web
  slice lands; this is an accepted, explicit consequence of the chosen scope.
- Deeper nesting raises the bar on the model/provider for valid Structured Outputs;
  the strict schema keeps it well-formed, and the self-refine loop (next slice) is
  where output *depth* is further improved.
