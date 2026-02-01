package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Settings represent a subset of Claude-style settings relevant to OpenClaude.
type Settings struct {
	// Model is the configured model alias or provider model name.
	Model string
	// EnabledPlugins mirrors Claude Code settings for compatibility.
	EnabledPlugins map[string]bool
	// Raw retains the full JSON map for future compatibility.
	Raw map[string]any
}

// LoadClaudeSettings loads settings from user/project/local sources and merges them.
func LoadClaudeSettings(cwd string, sources []string, extraSettings string) (*Settings, error) {
	sourceSet := normalizeSources(sources)
	paths, err := settingsPaths(cwd)
	if err != nil {
		return nil, err
	}

	var merged *Settings
	for _, item := range paths {
		if len(sourceSet) > 0 && !sourceSet[item.Source] {
			continue
		}
		// Missing files are ignored to match Claude Code behavior.
		settings, err := loadSettingsFromFile(item.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		merged = mergeSettings(merged, settings)
	}

	if extraSettings != "" {
		override, err := loadSettingsFlag(extraSettings)
		if err != nil {
			return nil, err
		}
		merged = mergeSettings(merged, override)
	}

	if merged == nil {
		return &Settings{Raw: map[string]any{}}, nil
	}

	return merged, nil
}

type settingsSource struct {
	Source string
	Path   string
}

// settingsPaths resolves user, project, and local settings files.
func settingsPaths(cwd string) ([]settingsSource, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	projectRoot := findProjectRoot(cwd)

	return []settingsSource{
		{Source: "user", Path: filepath.Join(home, ".claude", "settings.json")},
		{Source: "project", Path: filepath.Join(projectRoot, ".claude", "settings.json")},
		{Source: "local", Path: filepath.Join(cwd, ".claude", "settings.json")},
	}, nil
}

// normalizeSources returns a set of allowed sources, or nil if unrestricted.
func normalizeSources(sources []string) map[string]bool {
	if len(sources) == 0 {
		return nil
	}
	set := make(map[string]bool)
	for _, entry := range sources {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		set[strings.ToLower(entry)] = true
	}
	return set
}

// loadSettingsFromFile reads settings JSON from disk.
func loadSettingsFromFile(path string) (*Settings, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseSettings(raw)
}

// loadSettingsFlag resolves a settings override from a path or inline JSON.
func loadSettingsFlag(value string) (*Settings, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "{") {
		return parseSettings([]byte(trimmed))
	}
	return loadSettingsFromFile(trimmed)
}

// parseSettings parses Claude-style settings JSON.
func parseSettings(raw []byte) (*Settings, error) {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse settings: %w", err)
	}

	settings := &Settings{
		Raw:            data,
		EnabledPlugins: map[string]bool{},
	}

	if model, ok := data["model"].(string); ok {
		settings.Model = model
	}

	if plugins, ok := data["enabledPlugins"].(map[string]any); ok {
		for key, value := range plugins {
			switch typed := value.(type) {
			case bool:
				settings.EnabledPlugins[key] = typed
			}
		}
	}

	return settings, nil
}

// mergeSettings applies overlay values on top of the base settings.
func mergeSettings(base *Settings, overlay *Settings) *Settings {
	if base == nil {
		return overlay
	}
	if overlay == nil {
		return base
	}

	merged := &Settings{
		Model:          base.Model,
		EnabledPlugins: map[string]bool{},
		Raw:            map[string]any{},
	}

	for key, value := range base.Raw {
		merged.Raw[key] = value
	}
	for key, value := range overlay.Raw {
		merged.Raw[key] = value
	}

	if overlay.Model != "" {
		merged.Model = overlay.Model
	}

	for key, value := range base.EnabledPlugins {
		merged.EnabledPlugins[key] = value
	}
	for key, value := range overlay.EnabledPlugins {
		merged.EnabledPlugins[key] = value
	}

	return merged
}

// findProjectRoot locates the nearest parent directory containing .git.
func findProjectRoot(cwd string) string {
	current := filepath.Clean(cwd)
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			// If no repository root is found, fall back to the current directory.
			return cwd
		}
		current = parent
	}
}
