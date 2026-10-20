package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

type Config struct {
	Roots []string `json:"roots"`
	Depth int      `json:"depth"`
	Max   int      `json:"max"`
	// Ignore filters what the search finds
	Ignore []string `json:"ignore"`
	// CheckRemote lists repos whose remotes are queried with ls-remote
	CheckRemote    []string `json:"checkRemote"`
	ScanInterval   Duration `json:"scanInterval"`
	RemoteInterval Duration `json:"remoteInterval"`
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

func defaultConfig() Config {
	return Config{
		Roots:          []string{"~"},
		Depth:          5,
		Max:            5,
		ScanInterval:   Duration{5 * time.Minute},
		RemoteInterval: Duration{5 * time.Minute},
	}
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("Unable to find config directory: %w", err)
	}
	return filepath.Join(dir, "unpushed.json"), nil
}

func readConfig(v any) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("Unable to read config: %w", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("Unable to parse %s: %w", path, err)
	}
	return nil
}

// loadConfig reads the config with paths expanded.
func loadConfig() (Config, error) {
	cfg := defaultConfig()
	if err := readConfig(&cfg); err != nil {
		return cfg, err
	}
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	for _, paths := range cfg.paths() {
		for i, path := range *paths {
			(*paths)[i] = expand(path)
		}
	}
	return cfg, nil
}

func (cfg Config) validate() error {
	if len(cfg.Roots) == 0 {
		return fmt.Errorf("Config sets no roots")
	}
	if cfg.Depth < 1 || cfg.Max < 1 {
		return fmt.Errorf("Config sets depth or max below 1")
	}
	return nil
}

func (cfg *Config) paths() []*[]string {
	return []*[]string{&cfg.Roots, &cfg.Ignore, &cfg.CheckRemote}
}

// saveConfig writes cfg, leaving out keys at their default so a changed
// default still applies. Keys it doesn't know are kept, though json reorders
// them.
func saveConfig(cfg Config) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	for _, paths := range cfg.paths() {
		tilded := make([]string, len(*paths))
		for i, path := range *paths {
			tilded[i] = tilde(path)
		}
		*paths = tilded
	}
	raw := map[string]any{}
	if err := readConfig(&raw); err != nil {
		return err
	}
	value, defaults := reflect.ValueOf(cfg), reflect.ValueOf(defaultConfig())
	for i := range value.NumField() {
		key := value.Type().Field(i).Tag.Get("json")
		v, d := value.Field(i), defaults.Field(i)
		// A nil and an empty list are both the default
		if reflect.DeepEqual(v.Interface(), d.Interface()) || (v.Kind() == reflect.Slice && v.Len() == 0 && d.Len() == 0) {
			delete(raw, key)
		} else {
			raw[key] = v.Interface()
		}
	}
	path, err := configPath()
	if err != nil {
		return err
	}
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
