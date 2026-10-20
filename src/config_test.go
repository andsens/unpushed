package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestIgnoreAndAdd(t *testing.T) {
	root, a, b := setup(t)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	config := func() Config {
		t.Helper()
		cfg, err := loadConfig()
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}

	if err := ignoreRepo(a); err != nil {
		t.Fatal(err)
	}
	if cfg := config(); !slices.Equal(cfg.Ignore, []string{a}) || cfg.Depth != 5 {
		t.Fatalf("Ignore gave %+v", cfg)
	}
	data, _ := os.ReadFile(filepath.Join(root, "unpushed.json"))
	if want := `"~/a"`; !strings.Contains(string(data), want) {
		t.Fatalf("Config %s doesn't contain %s", data, want)
	}

	os.Remove(filepath.Join(root, "unpushed.json"))
	if err := addRepo(root); err == nil {
		t.Fatal("Added a directory that isn't a repository")
	}
	for _, r := range []string{a, b} {
		if err := addRepo(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := ignoreRepo(a); err != nil {
		t.Fatal(err)
	}
	if cfg := config(); !slices.Equal(cfg.Repos, []string{b}) {
		t.Fatalf("Ignore with repos gave %+v", cfg)
	}
	if err := ignoreRepo(b); err == nil {
		t.Fatal("Ignored the last repo in repos")
	}
}
