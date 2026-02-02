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
		return stringsToAny(defaultAgentList())
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(opts.AgentsJSON), &payload); err != nil {
		return stringsToAny(defaultAgentList())
	}
	names := make([]string, 0, len(payload))
	for name := range payload {
		names = append(names, name)
	}
	sort.Strings(names)
	return stringsToAny(names)
}

// listSkillNames reports available skills unless explicitly disabled.
func listSkillNames(opts *options, _ *config.Settings) []any {
	if opts != nil && opts.DisableSlashCommands {
		return []any{}
	}
	return stringsToAny(defaultSkillList())
}

// listSlashCommands reports available slash commands unless explicitly disabled.
func listSlashCommands(opts *options) []string {
	if opts != nil && opts.DisableSlashCommands {
		return []string{}
	}
	return defaultSlashCommandList()
}

// defaultSlashCommandList returns the built-in slash command identifiers.
// The ordering matches Claude Code output so stream-json consumers stay compatible.
func defaultSlashCommandList() []string {
	return []string{
		"keybindings-help",
		"compact",
		"context",
		"cost",
		"init",
		"pr-comments",
		"release-notes",
		"review",
		"security-review",
	}
}

// defaultAgentList returns the built-in agent profile identifiers.
// The list is intentionally fixed to mirror the upstream CLI defaults.
func defaultAgentList() []string {
	return []string{
		"Bash",
		"general-purpose",
		"statusline-setup",
		"Explore",
		"Plan",
	}
}

// defaultSkillList returns the built-in skill identifiers.
// Keeping this list stable avoids breaking stream-json init consumers.
func defaultSkillList() []string {
	return []string{"keybindings-help"}
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
		// Sort plugin names from settings to keep stream-json output deterministic.
		names := make([]string, 0, len(settings.EnabledPlugins))
		for name, enabled := range settings.EnabledPlugins {
			if !enabled || seen[name] {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			seen[name] = true
			plugins = append(plugins, map[string]string{
				"name": name,
			})
		}
	}

	return mapsToAny(plugins)
}

// stringsToAny converts a string slice into an any slice for JSON emission.
// It returns an empty slice (not nil) so JSON encodes [] instead of null.
func stringsToAny(values []string) []any {
	if len(values) == 0 {
		return []any{}
	}
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

// mapsToAny converts a slice of string maps to an any slice for JSON emission.
// It returns an empty slice (not nil) so JSON encodes [] instead of null.
func mapsToAny(values []map[string]string) []any {
	if len(values) == 0 {
		return []any{}
	}
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
