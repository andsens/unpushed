package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type Config struct {
	Roots    []string `json:"roots"`
	Depth    int      `json:"depth"`
	Max      int      `json:"max"`
	Terminal []string `json:"terminal"`
	// Repos replaces scanning the roots, Ignore filters what the scan finds
	Repos  []string `json:"repos"`
	Ignore []string `json:"ignore"`
	// CheckRemote lists repos whose remotes are queried with ls-remote
	CheckRemote []string `json:"checkRemote"`
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
		return fmt.Errorf("Unable to parse config: %w", err)
	}
	return nil
}

func loadConfig() (Config, error) {
	cfg := Config{Roots: []string{"~"}, Depth: 5, Max: 5, Terminal: []string{"konsole", "--workdir"}}
	if err := readConfig(&cfg); err != nil {
		return cfg, err
	}
	if len(cfg.Repos) > 0 && len(cfg.Ignore) > 0 {
		return cfg, fmt.Errorf("Config sets both repos and ignore, use one")
	}
	for _, paths := range [][]string{cfg.Roots, cfg.Repos, cfg.Ignore, cfg.CheckRemote} {
		for i, path := range paths {
			paths[i] = expand(path)
		}
	}
	return cfg, nil
}

func expand(path string) string {
	if home, err := os.UserHomeDir(); err == nil && (path == "~" || strings.HasPrefix(path, "~/")) {
		path = home + path[1:]
	}
	return filepath.Clean(path)
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

func tilde(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}

// editConfig changes the config file in place. Other keys are kept, though
// json reorders them.
func editConfig(edit func(raw map[string]any) error) error {
	raw := map[string]any{}
	if err := readConfig(&raw); err != nil {
		return err
	}
	if err := edit(raw); err != nil {
		return err
	}
	return writeConfig(raw)
}

// ignoreRepo removes repo from repos when that is set, else adds it to ignore.
func ignoreRepo(repo string) error {
	return editConfig(func(raw map[string]any) error {
		if repos := strs(raw["repos"]); len(repos) > 0 {
			repos = slices.DeleteFunc(repos, func(p string) bool { return expand(p) == repo })
			if len(repos) == 0 {
				return fmt.Errorf("Ignoring %s would empty repos and start a scan", repo)
			}
			raw["repos"] = repos
		} else {
			raw["ignore"] = append(strs(raw["ignore"]), tilde(repo))
		}
		return nil
	})
}

func addRepo(repo string) error {
	if fi, err := os.Stat(filepath.Join(repo, ".git")); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a git repository", repo)
	}
	return editConfig(func(raw map[string]any) error {
		raw["repos"] = append(strs(raw["repos"]), tilde(repo))
		return nil
	})
}

func writeConfig(raw map[string]any) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("Unable to encode config: %w", err)
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
