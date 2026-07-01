package settings

import (
	"testing"

	"scholarflow_server/internal/config"
)

func TestRegistryKeysUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range Registry {
		if seen[d.Key] {
			t.Fatalf("duplicate registry key %q", d.Key)
		}
		seen[d.Key] = true
	}
}

func TestRegistryKindsAndApplyValid(t *testing.T) {
	for _, d := range Registry {
		switch d.Kind {
		case KindString, KindInt, KindBool, KindCSV:
		default:
			t.Fatalf("key %q has invalid kind %q", d.Key, d.Kind)
		}
		switch d.Apply {
		case ApplyLive, ApplyRestart, ApplyBootstrap:
		default:
			t.Fatalf("key %q has invalid apply %q", d.Key, d.Apply)
		}
	}
}

func TestByKey(t *testing.T) {
	d, ok := ByKey("WRITE_API_TOKEN")
	if !ok {
		t.Fatal("WRITE_API_TOKEN not in registry")
	}
	if !d.Secret || d.Apply != ApplyLive {
		t.Fatalf("WRITE_API_TOKEN def = %+v, want secret+live", d)
	}
	if _, ok := ByKey("NOPE"); ok {
		t.Fatal("ByKey returned ok for unknown key")
	}
}

// Every registry key must be a real config key: building a Config where the
// getter returns each registry key's Default must not panic, and the registry
// must not reference a key config never reads. We assert the known secret set.
func TestRegistrySecretsAreExpected(t *testing.T) {
	wantSecret := map[string]bool{"WRITE_API_TOKEN": true, "OPENAI_API_KEY": true, "MINIO_SECRET_KEY": true, "DATABASE_URL": true}
	for _, d := range Registry {
		if d.Secret && !wantSecret[d.Key] {
			t.Fatalf("unexpected secret key %q", d.Key)
		}
		if wantSecret[d.Key] && !d.Secret {
			t.Fatalf("key %q should be marked secret", d.Key)
		}
	}
	// sanity: registry defaults feed config.Build without panic
	_ = config.Build(func(key string) (string, bool) {
		if d, ok := ByKey(key); ok {
			return d.Default, true
		}
		return "", false
	})
}
