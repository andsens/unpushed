package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestSaveConfig(t *testing.T) {
	root, a, _ := setup(t)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	file := filepath.Join(root, "unpushed.json")
	if err := os.WriteFile(file, []byte(`{"unknown": 1, "depth": 3}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	cfg.Ignore = []string{a}
	cfg.ScanInterval = Duration{time.Hour}
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	raw := map[string]any{}
	data, _ := os.ReadFile(file)
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	// Defaults are left out, depth went back to its default, unknown keys stay
	if len(raw) != 3 || raw["unknown"] != 1.0 || raw["scanInterval"] != "1h" || !slices.Equal(strs(raw["ignore"]), []string{"~/a"}) {
		t.Fatalf("Saved %s", data)
	}
	loaded, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(loaded.Ignore, []string{a}) || loaded.ScanInterval.Duration != time.Hour || loaded.Depth != 5 {
		t.Fatalf("Loaded %+v", loaded)
	}

	cfg.Roots = nil
	if err := saveConfig(cfg); err == nil {
		t.Fatal("Saved a config without roots")
	}
}

func TestNearestInterval(t *testing.T) {
	for d, want := range map[time.Duration]time.Duration{
		time.Second: time.Minute, 5 * time.Minute: 5 * time.Minute, 40 * time.Minute: 30 * time.Minute,
		50 * time.Minute: time.Hour, 24 * time.Hour: 6 * time.Hour,
	} {
		if got := intervals[nearestInterval(d)]; got != want {
			t.Errorf("%s snapped to %s, want %s", d, got, want)
		}
	}
}

func strs(v any) []string {
	var out []string
	if list, ok := v.([]any); ok {
		for _, s := range list {
			if s, ok := s.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func TestDurationString(t *testing.T) {
	for d, want := range map[time.Duration]string{5 * time.Minute: "5m", time.Hour: "1h", 90 * time.Minute: "1h30m", 45 * time.Second: "45s", 61 * time.Second: "1m1s"} {
		if got := (Duration{d}).String(); got != want {
			t.Errorf("%s printed as %s, want %s", d, got, want)
		}
	}
}
