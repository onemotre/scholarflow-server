package settings

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"scholarflow_server/internal/config"
)

var (
	ErrUnknownKey   = errors.New("unknown setting key")
	ErrNotEditable  = errors.New("setting is not editable")
	ErrInvalidValue = errors.New("invalid setting value")
)

// EffectiveSetting is the resolved view of one setting for the API. The HTTP
// layer is responsible for masking Value when Secret is true.
type EffectiveSetting struct {
	Key    string `json:"key"`
	Group  string `json:"group"`
	Kind   string `json:"kind"`
	Apply  string `json:"apply"`
	Secret bool   `json:"secret"`
	Label  string `json:"label"`
	Help   string `json:"help"`
	Source string `json:"source"`          // "db" | "env" | "default"
	Value  string `json:"value,omitempty"` // raw; masked by the handler for secrets
	IsSet  bool   `json:"is_set"`          // effective value is non-empty
}

// Provider resolves effective settings (DB override -> env -> registry default)
// with a short-TTL cache over the override map, and validates writes.
type Provider struct {
	repo Repository
	ttl  time.Duration
	now  func() time.Time

	mu        sync.Mutex
	overrides map[string]string
	fetchedAt time.Time
	loaded    bool
}

func NewProvider(repo Repository, ttl time.Duration) *Provider {
	return NewProviderWithClock(repo, ttl, time.Now)
}

func NewProviderWithClock(repo Repository, ttl time.Duration, now func() time.Time) *Provider {
	return &Provider{repo: repo, ttl: ttl, now: now}
}

// bust forces the next resolution to refetch overrides.
func (p *Provider) bust() {
	p.mu.Lock()
	p.loaded = false
	p.mu.Unlock()
}

func (p *Provider) overrideMap(ctx context.Context) map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.loaded && p.now().Sub(p.fetchedAt) < p.ttl {
		return p.overrides
	}
	data, err := p.repo.List(ctx)
	if err != nil {
		// On a transient repo error, fall back to the last known map (or empty);
		// settings resolution must never hard-fail a request.
		if p.overrides == nil {
			p.overrides = map[string]string{}
		}
		return p.overrides
	}
	p.overrides = data
	p.fetchedAt = p.now()
	p.loaded = true
	return p.overrides
}

// rawWithSource resolves a key to its effective raw value and the source layer.
func (p *Provider) rawWithSource(ctx context.Context, key string) (string, string) {
	if v, ok := p.overrideMap(ctx)[key]; ok {
		return v, "db"
	}
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v, "env"
	}
	if d, ok := ByKey(key); ok {
		return d.Default, "default"
	}
	return "", "default"
}

func (p *Provider) raw(ctx context.Context, key string) string {
	v, _ := p.rawWithSource(ctx, key)
	return v
}

func (p *Provider) String(ctx context.Context, key string) string { return p.raw(ctx, key) }

func (p *Provider) Int(ctx context.Context, key string) int {
	n, err := strconv.Atoi(strings.TrimSpace(p.raw(ctx, key)))
	if err != nil {
		if d, ok := ByKey(key); ok {
			dn, _ := strconv.Atoi(d.Default)
			return dn
		}
		return 0
	}
	return n
}

func (p *Provider) Bool(ctx context.Context, key string) bool {
	b, err := strconv.ParseBool(strings.TrimSpace(p.raw(ctx, key)))
	if err != nil {
		if d, ok := ByKey(key); ok {
			db, _ := strconv.ParseBool(d.Default)
			return db
		}
		return false
	}
	return b
}

func (p *Provider) CSV(ctx context.Context, key string) []string {
	raw := p.raw(ctx, key)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// Snapshot builds a config.Config resolving every key through the override->env
// ->default chain. Callers take this once at startup.
func (p *Provider) Snapshot(ctx context.Context) config.Config {
	overrides := p.overrideMap(ctx)
	return config.Build(func(key string) (string, bool) {
		if v, ok := overrides[key]; ok {
			return v, true
		}
		return os.LookupEnv(key)
	})
}

// Effective returns the resolved view of every registry setting (raw values;
// the handler masks secrets).
func (p *Provider) Effective(ctx context.Context) []EffectiveSetting {
	out := make([]EffectiveSetting, 0, len(Registry))
	for _, d := range Registry {
		value, source := p.rawWithSource(ctx, d.Key)
		out = append(out, EffectiveSetting{
			Key:    d.Key,
			Group:  d.Group,
			Kind:   string(d.Kind),
			Apply:  string(d.Apply),
			Secret: d.Secret,
			Label:  d.Label,
			Help:   d.Help,
			Source: source,
			Value:  value,
			IsSet:  strings.TrimSpace(value) != "",
		})
	}
	return out
}

// Set validates and persists an override. Bootstrap settings and unknown keys
// are rejected; values must parse for their kind.
func (p *Provider) Set(ctx context.Context, key, value string) error {
	d, ok := ByKey(key)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	if d.Apply == ApplyBootstrap {
		return fmt.Errorf("%w: %s", ErrNotEditable, key)
	}
	if err := validateKind(d.Kind, value); err != nil {
		return err
	}
	if err := p.repo.Upsert(ctx, key, value); err != nil {
		return err
	}
	p.bust()
	return nil
}

// Reset removes an override, reverting to env/default.
func (p *Provider) Reset(ctx context.Context, key string) error {
	d, ok := ByKey(key)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	if d.Apply == ApplyBootstrap {
		return fmt.Errorf("%w: %s", ErrNotEditable, key)
	}
	if err := p.repo.Delete(ctx, key); err != nil {
		return err
	}
	p.bust()
	return nil
}

func validateKind(kind Kind, value string) error {
	switch kind {
	case KindInt:
		if _, err := strconv.Atoi(strings.TrimSpace(value)); err != nil {
			return fmt.Errorf("%w: expected integer", ErrInvalidValue)
		}
	case KindBool:
		if _, err := strconv.ParseBool(strings.TrimSpace(value)); err != nil {
			return fmt.Errorf("%w: expected boolean", ErrInvalidValue)
		}
	case KindString, KindCSV:
		// any string accepted
	default:
		return fmt.Errorf("%w: unknown kind", ErrInvalidValue)
	}
	return nil
}
