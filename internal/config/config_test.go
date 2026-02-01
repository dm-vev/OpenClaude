package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadClaudeSettingsPrecedence(t *testing.T) {
	// Arrange a temporary HOME and project tree with layered settings.
	tempDir := t.TempDir()
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(filepath.Join(homeDir, ".claude"), 0o755); err != nil {
		t.Fatalf("create home dir: %v", err)
	}
	userSettings := `{"model":"user"}`
	if err := os.WriteFile(filepath.Join(homeDir, ".claude", "settings.json"), []byte(userSettings), 0o600); err != nil {
		t.Fatalf("write user settings: %v", err)
	}

	// Create a repo root with project settings.
	repoDir := filepath.Join(tempDir, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatalf("create repo dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, ".claude"), 0o755); err != nil {
		t.Fatalf("create project settings dir: %v", err)
	}
	projectSettings := `{"model":"project"}`
	if err := os.WriteFile(filepath.Join(repoDir, ".claude", "settings.json"), []byte(projectSettings), 0o600); err != nil {
		t.Fatalf("write project settings: %v", err)
	}

	// Add local settings in a subdirectory to override project settings.
	localDir := filepath.Join(repoDir, "sub")
	if err := os.MkdirAll(filepath.Join(localDir, ".claude"), 0o755); err != nil {
		t.Fatalf("create local dir: %v", err)
	}
	localSettings := `{"model":"local"}`
	if err := os.WriteFile(filepath.Join(localDir, ".claude", "settings.json"), []byte(localSettings), 0o600); err != nil {
		t.Fatalf("write local settings: %v", err)
	}

	// Override HOME so the loader reads our temp user settings.
	t.Setenv("HOME", homeDir)

	// Act.
	settings, err := LoadClaudeSettings(localDir, []string{"user", "project", "local"}, "")
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	// Assert.
	if settings.Model != "local" {
		t.Fatalf("expected local model, got %s", settings.Model)
	}
}

func TestResolveModelAliases(t *testing.T) {
	// Arrange a config with an alias.
	cfg := &ProviderConfig{
		DefaultModel: "base-model",
		ModelAliases: map[string]string{
			"opus": "alias-model",
		},
	}

	// Assert alias resolution.
	if got := ResolveModel(cfg, "", "opus"); got != "alias-model" {
		t.Fatalf("expected alias-model, got %s", got)
	}
	// CLI overrides settings.
	if got := ResolveModel(cfg, "custom", "opus"); got != "custom" {
		t.Fatalf("expected custom, got %s", got)
	}
}
