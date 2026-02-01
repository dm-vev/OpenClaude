package main

import (
	"encoding/json"
	"path/filepath"
	"sort"

	"github.com/openclaude/openclaude/internal/agent"
	"github.com/openclaude/openclaude/internal/config"
)

// resolveSystemPrompt builds the system prompt from defaults and CLI overrides.
func resolveSystemPrompt(opts *options, runner *agent.Runner) string {
	// Start from the default Claude Code system prompt for the active tool set.
	toolNames := listToolNames(runner)
	prompt := agent.DefaultSystemPrompt(toolNames)

	// Apply the explicit system prompt override when provided.
	if opts.SystemPrompt != "" {
		prompt = opts.SystemPrompt
	}

	// Append extra instructions after any base prompt.
	if opts.AppendSystemPrompt != "" {
		prompt = prompt + "\n\n" + opts.AppendSystemPrompt
	}

	return prompt
}

// resolveOutputStyle returns the configured output style, defaulting to "default".
func resolveOutputStyle(settings *config.Settings) string {
	if settings == nil || settings.Raw == nil {
		return "default"
	}
	if value, ok := settings.Raw["outputStyle"].(string); ok && value != "" {
		return value
	}
	if value, ok := settings.Raw["output_style"].(string); ok && value != "" {
		return value
	}
	return "default"
}

// listAgentNames returns the sorted list of agent identifiers from the JSON payload.
func listAgentNames(opts *options) []any {
	if opts.AgentsJSON == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(opts.AgentsJSON), &payload); err != nil {
		return nil
	}
	names := make([]string, 0, len(payload))
	for name := range payload {
		names = append(names, name)
	}
	sort.Strings(names)
	return stringsToAny(names)
}

// listSkillNames reports available skills when configured.
func listSkillNames(_ *config.Settings) []any {
	// Skills are not yet surfaced in OpenClaude settings.
	return nil
}

// listPluginDescriptors returns plugin metadata for stream-json init events.
func listPluginDescriptors(opts *options, settings *config.Settings) []any {
	seen := map[string]bool{}
	var plugins []map[string]string

	for _, path := range opts.PluginDir {
		if path == "" {
			continue
		}
		name := filepath.Base(path)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		plugins = append(plugins, map[string]string{
			"name": name,
			"path": path,
		})
	}

	if settings != nil {
		for name, enabled := range settings.EnabledPlugins {
			if !enabled || seen[name] {
				continue
			}
			seen[name] = true
			plugins = append(plugins, map[string]string{
				"name": name,
			})
		}
	}

	return mapsToAny(plugins)
}

// stringsToAny converts a string slice into an any slice for JSON emission.
func stringsToAny(values []string) []any {
	if len(values) == 0 {
		return nil
	}
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

// mapsToAny converts a slice of string maps to an any slice for JSON emission.
func mapsToAny(values []map[string]string) []any {
	if len(values) == 0 {
		return nil
	}
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
