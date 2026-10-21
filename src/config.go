package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type Config struct {
	// Roots are searched for repositories, unless excluded. An excluded
	// directory below an included root is left out of the search.
	Roots          map[string]Root       `json:"roots"`
	Repos          map[string]RepoConfig `json:"repos"`
	Depth          int                   `json:"depth"`
	Max            int                   `json:"max"`
	ScanInterval   Duration              `json:"scanInterval"`
	RemoteInterval Duration              `json:"remoteInterval"`
}

type Root struct {
	Include bool `json:"include"`
}

type RepoConfig struct {
	// Ignore filters what the search finds
	Ignore bool `json:"ignore,omitempty"`
	// CheckRemote has the remotes queried with ls-remote
	CheckRemote bool `json:"checkRemote,omitempty"`
	// Pinned repos are listed first
	Pinned bool `json:"pinned,omitempty"`
}

// Duration is a time.Duration written as a string ("5m") in the config.
type Duration struct{ time.Duration }

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// String drops zero minutes and seconds: 5m rather than 5m0s.
func (d Duration) String() string {
	s := d.Duration.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	if v <= 0 {
		return fmt.Errorf("Duration %s isn't positive", s)
	}
	d.Duration = v
	return nil
}

// defaultConfig has no roots, defaultRoots apply when no layer sets any.
func defaultConfig() Config {
	return Config{
		Roots:          map[string]Root{},
		Repos:          map[string]RepoConfig{},
		Depth:          5,
		Max:            5,
		ScanInterval:   Duration{5 * time.Minute},
		RemoteInterval: Duration{5 * time.Minute},
	}
}

func defaultRoots() map[string]Root {
	return map[string]Root{expand("~"): {Include: true}}
}

// userConfigPath is the config the settings window writes, on top of the
// base config given with --ro-config, unless --config names another.
func userConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("Unable to find config directory: %w", err)
	}
	return filepath.Join(dir, "unpushed.json"), nil
}

// readLayer reads a config file with the paths in its keys expanded. A
// missing file is empty when optional.
func readLayer(path string, optional bool) (map[string]any, error) {
	raw := map[string]any{}
	data, err := os.ReadFile(path)
	if optional && errors.Is(err, os.ErrNotExist) {
		return raw, nil
	} else if err != nil {
		return nil, fmt.Errorf("Unable to read config: %w", err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("Unable to parse %s: %w", path, err)
	}
	for _, key := range []string{"roots", "repos"} {
		if paths, ok := raw[key].(map[string]any); ok {
			expanded := map[string]any{}
			for path, v := range paths {
				expanded[expand(path)] = v
			}
			raw[key] = expanded
		}
	}
	return raw, nil
}

// merge sets src's keys in dst. Objects merge key by key, so a layer can
// override a single property of a repo, and null removes a key.
func merge(dst, src map[string]any) {
	for k, v := range src {
		if v == nil {
			delete(dst, k)
			continue
		}
		sub, ok := v.(map[string]any)
		if !ok {
			dst[k] = v
			continue
		}
		// Merged into a copy, later layers would change src's otherwise
		into, ok := dst[k].(map[string]any)
		if !ok {
			into = map[string]any{}
			dst[k] = into
		}
		merge(into, sub)
	}
}

// decode puts the layers, merged in order, over the defaults.
func decode(layers ...map[string]any) (Config, error) {
	merged := map[string]any{}
	for _, layer := range layers {
		merge(merged, layer)
	}
	cfg := defaultConfig()
	data, _ := json.Marshal(merged)
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("Unable to parse config: %w", err)
	}
	if cfg.Roots == nil {
		cfg.Roots = map[string]Root{}
	}
	if cfg.Repos == nil {
		cfg.Repos = map[string]RepoConfig{}
	}
	return cfg, nil
}

// loadConfig reads the base config at base, if any, with the user's config at
// path on top. lower is the config without the user's, which saveConfig writes
// the difference to.
func loadConfig(base, path string) (cfg, lower Config, err error) {
	var baseLayer map[string]any
	if base != "" {
		if baseLayer, err = readLayer(base, false); err != nil {
			return cfg, lower, err
		}
	}
	user, err := readLayer(path, true)
	if err != nil {
		return cfg, lower, err
	}
	if lower, err = decode(baseLayer); err != nil {
		return cfg, lower, err
	}
	if cfg, err = decode(baseLayer, user); err != nil {
		return cfg, lower, err
	}
	if len(cfg.Roots) == 0 {
		cfg.Roots = defaultRoots()
	}
	return cfg, lower, cfg.validate()
}

func (cfg Config) validate() error {
	if !slices.ContainsFunc(slices.Collect(maps.Values(cfg.Roots)), func(r Root) bool { return r.Include }) {
		return fmt.Errorf("Config includes no roots")
	}
	if cfg.Depth < 1 || cfg.Max < 1 {
		return fmt.Errorf("Config sets depth or max below 1")
	}
	return nil
}

// searched are the roots to search. Each search prunes the roots below its
// own, so the most specific one decides.
func (cfg Config) searched() (searched, pruned []string) {
	for path, r := range cfg.Roots {
		if r.Include {
			searched = append(searched, path)
		}
		pruned = append(pruned, path)
	}
	slices.Sort(searched)
	return searched, pruned
}

// saveConfig writes what cfg changes over lower to the user's config at
// path, so a changed default or base config still applies where the user
// didn't override it. Keys it doesn't know are kept, though json reorders
// them.
func saveConfig(path string, cfg, lower Config) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	raw := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("Unable to parse %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("Unable to read config: %w", err)
	}
	set := func(key string, v any, changed bool) {
		if changed {
			raw[key] = v
		} else {
			delete(raw, key)
		}
	}
	roots := map[string]any{}
	// Left as they came from defaultRoots, they stay unwritten
	if len(lower.Roots) > 0 || !maps.Equal(cfg.Roots, defaultRoots()) {
		for path, r := range cfg.Roots {
			if l, ok := lower.Roots[path]; !ok || l != r {
				roots[tilde(path)] = r
			}
		}
		for path := range lower.Roots {
			if _, ok := cfg.Roots[path]; !ok {
				roots[tilde(path)] = nil
			}
		}
	}
	set("roots", roots, len(roots) > 0)
	repos := map[string]map[string]bool{}
	for path, r := range cfg.Repos {
		l, props := lower.Repos[path], map[string]bool{}
		for key, v := range map[string][2]bool{"ignore": {r.Ignore, l.Ignore}, "checkRemote": {r.CheckRemote, l.CheckRemote}, "pinned": {r.Pinned, l.Pinned}} {
			if v[0] != v[1] {
				props[key] = v[0]
			}
		}
		if len(props) > 0 {
			repos[tilde(path)] = props
		}
	}
	set("repos", repos, len(repos) > 0)
	set("depth", cfg.Depth, cfg.Depth != lower.Depth)
	set("max", cfg.Max, cfg.Max != lower.Max)
	set("scanInterval", cfg.ScanInterval, cfg.ScanInterval != lower.ScanInterval)
	set("remoteInterval", cfg.RemoteInterval, cfg.RemoteInterval != lower.RemoteInterval)
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("Unable to encode config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("Unable to create config directory: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("Unable to write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("Unable to replace config: %w", err)
	}
	return nil
}

// expand replaces a leading ~ with the home directory. Both / and the OS
// separator may follow it.
func expand(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && (path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~"+string(filepath.Separator))) {
		path = home + path[1:]
	}
	return filepath.Clean(path)
}

func tilde(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && (path == home || strings.HasPrefix(path, home+string(filepath.Separator))) {
		return "~" + path[len(home):]
	}
	return path
}

func isRepo(path string) bool {
	fi, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && fi.IsDir()
}
