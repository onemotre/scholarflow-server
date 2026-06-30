package settings

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	data       map[string]string
	upserts    [][2]string
	deletes    []string
	listCalls  int
	failUpsert error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{data: map[string]string{}} }
func (r *fakeRepo) List(ctx context.Context) (map[string]string, error) {
	r.listCalls++
	cp := make(map[string]string, len(r.data))
	for k, v := range r.data {
		cp[k] = v
	}
	return cp, nil
}
func (r *fakeRepo) Upsert(ctx context.Context, key, value string) error {
	if r.failUpsert != nil {
		return r.failUpsert
	}
	r.data[key] = value
	r.upserts = append(r.upserts, [2]string{key, value})
	return nil
}
func (r *fakeRepo) Delete(ctx context.Context, key string) error {
	delete(r.data, key)
	r.deletes = append(r.deletes, key)
	return nil
}

func TestProviderResolutionPrecedence(t *testing.T) {
	repo := newFakeRepo()
	p := NewProvider(repo, time.Minute)
	ctx := context.Background()

	// default (no env, no override)
	t.Setenv("OPENAI_MODEL", "")
	if got := p.String(ctx, "OPENAI_MODEL"); got != "gpt-4o-mini" {
		t.Fatalf("default = %q, want gpt-4o-mini", got)
	}
	// env overrides default
	t.Setenv("OPENAI_MODEL", "from-env")
	p.bust()
	if got := p.String(ctx, "OPENAI_MODEL"); got != "from-env" {
		t.Fatalf("env = %q, want from-env", got)
	}
	// DB override beats env
	repo.data["OPENAI_MODEL"] = "from-db"
	p.bust()
	if got := p.String(ctx, "OPENAI_MODEL"); got != "from-db" {
		t.Fatalf("db = %q, want from-db", got)
	}
}

func TestProviderTypedGetters(t *testing.T) {
	repo := newFakeRepo()
	repo.data["READ_MAX_RETRY"] = "9"
	repo.data["FIGURE_EXTRACT_ENABLED"] = "false"
	repo.data["ARXIV_HARVEST_CATEGORIES"] = "cs.AI, cs.LG ,"
	p := NewProvider(repo, time.Minute)
	ctx := context.Background()
	if p.Int(ctx, "READ_MAX_RETRY") != 9 {
		t.Fatalf("int = %d, want 9", p.Int(ctx, "READ_MAX_RETRY"))
	}
	if p.Bool(ctx, "FIGURE_EXTRACT_ENABLED") {
		t.Fatal("bool = true, want false")
	}
	cats := p.CSV(ctx, "ARXIV_HARVEST_CATEGORIES")
	if len(cats) != 2 || cats[0] != "cs.AI" || cats[1] != "cs.LG" {
		t.Fatalf("csv = %#v, want [cs.AI cs.LG]", cats)
	}
}

func TestProviderSnapshotAppliesOverrides(t *testing.T) {
	repo := newFakeRepo()
	repo.data["GROBID_URL"] = "http://grobid.internal:8070"
	repo.data["MAX_UPLOAD_BYTES"] = "999"
	p := NewProvider(repo, time.Minute)
	cfg := p.Snapshot(context.Background())
	if cfg.GROBIDURL != "http://grobid.internal:8070" {
		t.Fatalf("snapshot GROBIDURL = %q", cfg.GROBIDURL)
	}
	if cfg.MaxUploadBytes != 999 {
		t.Fatalf("snapshot MaxUploadBytes = %d, want 999", cfg.MaxUploadBytes)
	}
}

func TestProviderSetValidates(t *testing.T) {
	repo := newFakeRepo()
	p := NewProvider(repo, time.Minute)
	ctx := context.Background()

	if err := p.Set(ctx, "NOPE", "x"); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("unknown key err = %v, want ErrUnknownKey", err)
	}
	if err := p.Set(ctx, "DATABASE_URL", "x"); !errors.Is(err, ErrNotEditable) {
		t.Fatalf("bootstrap key err = %v, want ErrNotEditable", err)
	}
	if err := p.Set(ctx, "READ_MAX_RETRY", "notanint"); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("bad int err = %v, want ErrInvalidValue", err)
	}
	if err := p.Set(ctx, "FIGURE_EXTRACT_ENABLED", "maybe"); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("bad bool err = %v, want ErrInvalidValue", err)
	}
	if err := p.Set(ctx, "OPENAI_MODEL", "gpt-x"); err != nil {
		t.Fatalf("valid set err = %v", err)
	}
	if repo.data["OPENAI_MODEL"] != "gpt-x" {
		t.Fatalf("upsert not persisted: %v", repo.data)
	}
}

func TestProviderSetBustsCache(t *testing.T) {
	repo := newFakeRepo()
	p := NewProvider(repo, time.Hour) // long TTL: only a bust refreshes
	ctx := context.Background()
	t.Setenv("OPENAI_MODEL", "")
	_ = p.String(ctx, "OPENAI_MODEL") // primes cache
	if err := p.Set(ctx, "OPENAI_MODEL", "fresh"); err != nil {
		t.Fatal(err)
	}
	if got := p.String(ctx, "OPENAI_MODEL"); got != "fresh" {
		t.Fatalf("after Set, got %q, want fresh (cache not busted)", got)
	}
}

func TestProviderResetRemovesOverride(t *testing.T) {
	repo := newFakeRepo()
	repo.data["OPENAI_MODEL"] = "from-db"
	p := NewProvider(repo, time.Hour)
	ctx := context.Background()
	t.Setenv("OPENAI_MODEL", "")
	if err := p.Reset(ctx, "OPENAI_MODEL"); err != nil {
		t.Fatal(err)
	}
	if got := p.String(ctx, "OPENAI_MODEL"); got != "gpt-4o-mini" {
		t.Fatalf("after Reset, got %q, want default", got)
	}
	if len(repo.deletes) != 1 || repo.deletes[0] != "OPENAI_MODEL" {
		t.Fatalf("delete not called: %v", repo.deletes)
	}
}

func TestProviderEffectiveMasksNothingButReportsSource(t *testing.T) {
	repo := newFakeRepo()
	repo.data["OPENAI_MODEL"] = "from-db"
	p := NewProvider(repo, time.Minute)
	ctx := context.Background()
	t.Setenv("WRITE_API_TOKEN", "")
	var model, token *EffectiveSetting
	for _, es := range p.Effective(ctx) {
		es := es // avoid aliasing the loop variable
		if es.Key == "OPENAI_MODEL" {
			model = &es
		}
		if es.Key == "WRITE_API_TOKEN" {
			token = &es
		}
	}
	if model == nil || model.Source != "db" || model.Value != "from-db" {
		t.Fatalf("model effective = %+v, want source=db value=from-db", model)
	}
	// Effective() carries the raw value; the HTTP layer masks secrets. IsSet reflects non-empty.
	if token == nil || token.Source != "default" || token.IsSet {
		t.Fatalf("token effective = %+v, want source=default is_set=false", token)
	}
}

func TestProviderCacheTTLRefetches(t *testing.T) {
	repo := newFakeRepo()
	now := time.Unix(1000, 0)
	p := NewProviderWithClock(repo, 5*time.Second, func() time.Time { return now })
	ctx := context.Background()
	_ = p.Effective(ctx)
	first := repo.listCalls
	_ = p.Effective(ctx) // within TTL, no refetch
	if repo.listCalls != first {
		t.Fatalf("refetched within TTL: %d vs %d", repo.listCalls, first)
	}
	now = now.Add(6 * time.Second) // past TTL
	_ = p.Effective(ctx)
	if repo.listCalls != first+1 {
		t.Fatalf("did not refetch after TTL: %d", repo.listCalls)
	}
}
