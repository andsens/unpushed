package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// configHome points the home and config directories at a temp dir and
// returns where the user's config goes.
func configHome(t *testing.T) (home, file string) {
	home = t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("AppData", home)
	// Windows takes the home from USERPROFILE
	t.Setenv("USERPROFILE", home)
	// macOS ignores XDG_CONFIG_HOME, so ask where the file goes
	file, err := userConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	return home, file
}

func writeJSON(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	raw := map[string]any{}
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

// tl is rel below ~ as saveConfig writes it.
func tl(rel string) string { return "~" + string(filepath.Separator) + filepath.FromSlash(rel) }

func TestLayers(t *testing.T) {
	home, file := configHome(t)
	base := filepath.Join(home, "base.json")
	writeJSON(t, base, `{"roots": {"~/src": {"include": true}, "~/old": {"include": false}}, "repos": {"~/src/x": {"ignore": true, "checkRemote": true}}, "depth": 3}`)
	writeJSON(t, file, `{"unknown": 1, "repos": {"~/src/x": {"ignore": false}}, "scanInterval": "1h"}`)
	src, x, y := filepath.Join(home, "src"), filepath.Join(home, "src", "x"), filepath.Join(home, "src", "y")

	cfg, lower, err := loadConfig(base, file)
	if err != nil {
		t.Fatal(err)
	}
	// The user's config overrides a single property, the default ~ doesn't apply
	if !reflect.DeepEqual(cfg.Roots, map[string]Root{src: {true}, filepath.Join(home, "old"): {}}) || cfg.Repos[x] != (RepoConfig{CheckRemote: true}) ||
		cfg.Depth != 3 || cfg.Max != 5 || cfg.ScanInterval.Duration != time.Hour {
		t.Fatalf("Loaded %+v", cfg)
	}

	// Removing a base root writes null, the user's own root and repo
	// settings are written, as is a depth back at the built-in default but
	// not the base's. x goes back to the base's settings, so it's left out.
	delete(cfg.Roots, filepath.Join(home, "old"))
	cfg.Roots[src] = Root{}
	cfg.Roots[filepath.Join(home, "work")] = Root{true}
	cfg.Repos[x] = RepoConfig{Ignore: true, CheckRemote: true}
	cfg.Repos[y] = RepoConfig{Pinned: true}
	cfg.Depth = 5
	cfg.ScanInterval = Duration{5 * time.Minute}
	if err := saveConfig(file, cfg, lower); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"unknown": 1.0,
		"roots":   map[string]any{tl("src"): map[string]any{"include": false}, tl("old"): nil, tl("work"): map[string]any{"include": true}},
		"repos":   map[string]any{tl("src/y"): map[string]any{"pinned": true}},
		"depth":   5.0,
	}
	if raw := readJSON(t, file); !reflect.DeepEqual(raw, want) {
		t.Fatalf("Saved %v", raw)
	}
	loaded, _, err := loadConfig(base, file)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, cfg) {
		t.Fatalf("Loaded %+v, saved %+v", loaded, cfg)
	}

	// Added back as the base has it, the root is the base's again
	cfg.Roots[filepath.Join(home, "old")] = Root{}
	cfg.Roots[src] = Root{true}
	if err := saveConfig(file, cfg, lower); err != nil {
		t.Fatal(err)
	}
	if raw := readJSON(t, file); !reflect.DeepEqual(raw["roots"], map[string]any{tl("work"): map[string]any{"include": true}}) {
		t.Fatalf("Saved %v", raw)
	}

	cfg.Roots = map[string]Root{src: {}}
	if err := saveConfig(file, cfg, lower); err == nil {
		t.Fatal("Saved a config without included roots")
	}
}

func TestDefaultRoots(t *testing.T) {
	home, file := configHome(t)
	cfg, lower, err := loadConfig("", file)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Roots, map[string]Root{home: {true}}) {
		t.Fatalf("Default roots %v", cfg.Roots)
	}
	if err := saveConfig(file, cfg, lower); err != nil {
		t.Fatal(err)
	}
	if raw := readJSON(t, file); len(raw) != 0 {
		t.Fatalf("Saved %v", raw)
	}
	// With a root added, ~ has to be written too, or it'd be gone
	cfg.Roots[filepath.Join(home, "src")] = Root{true}
	if err := saveConfig(file, cfg, lower); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"~": map[string]any{"include": true}, tl("src"): map[string]any{"include": true}}
	if raw := readJSON(t, file); !reflect.DeepEqual(raw["roots"], want) {
		t.Fatalf("Saved %v", raw)
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

func TestDurationString(t *testing.T) {
	for d, want := range map[time.Duration]string{5 * time.Minute: "5m", time.Hour: "1h", 90 * time.Minute: "1h30m", 45 * time.Second: "45s", 61 * time.Second: "1m1s"} {
		if got := (Duration{d}).String(); got != want {
			t.Errorf("%s printed as %s, want %s", d, got, want)
		}
	}
}
