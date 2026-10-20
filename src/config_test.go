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

	cfg.Repos = []string{a}
	if err := saveConfig(cfg); err == nil {
		t.Fatal("Saved both repos and ignore")
	}
}

func TestParseSettings(t *testing.T) {
	d := defaultConfig()
	d.Ignore = []string{"/x"}
	d.Repos = []string{"/a", "/b"}
	d.CheckRemote = []string{"/a", "/x"}
	cfg, err := parseSettings(d, modeList, "/r one\n\n/r2\n", " 3 ", "7", "10m", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Ignore != nil || !slices.Equal(cfg.CheckRemote, []string{"/a"}) || !slices.Equal(cfg.Roots, []string{"/r one", "/r2"}) ||
		cfg.Depth != 3 || cfg.Max != 7 ||
		cfg.ScanInterval.Duration != 10*time.Minute || cfg.RemoteInterval.Duration != time.Hour {
		t.Fatalf("Parsed %+v", cfg)
	}
	if cfg, _ = parseSettings(d, modeScan, "/r", "3", "7", "10m", "1h"); cfg.Repos != nil || cfg.Ignore == nil {
		t.Fatalf("Scan mode kept repos or dropped ignore: %+v", cfg)
	}
	for _, bad := range [][]string{{"x", "7", "10m"}, {"3", "7", "soon"}, {"3", "7", "-5m"}} {
		if _, err := parseSettings(d, modeScan, "/r", bad[0], bad[1], bad[2], "1h"); err == nil {
			t.Fatalf("Parsed %v", bad)
		}
	}
	if _, err := parseSettings(d, modeScan, "\n", "3", "7", "10m", "1h"); err == nil {
		t.Fatal("Parsed scan mode without roots")
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
