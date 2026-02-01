package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ProviderConfig defines how OpenClaude connects to an OpenAI-compatible gateway.
type ProviderConfig struct {
	// APIBaseURL is the base URL for OpenAI-compatible chat completions.
	APIBaseURL string `json:"api_base_url"`
	// APIKey is the bearer token used for Authorization.
	APIKey string `json:"api_key"`
	// TimeoutMS configures request timeout in milliseconds.
	TimeoutMS int `json:"timeout_ms"`
	// DefaultModel is used when no CLI or settings override is provided.
	DefaultModel string `json:"default_model"`
	// ModelAliases maps friendly names (e.g., opus) to provider model ids.
	ModelAliases map[string]string `json:"model_aliases"`
	// Pricing holds per-model pricing metadata for budget enforcement.
	Pricing map[string]ModelPricing `json:"pricing"`
	// Telemetry controls optional telemetry behavior.
	Telemetry TelemetryConfig `json:"telemetry"`
}

// ModelPricing defines per-model pricing for budget enforcement.
type ModelPricing struct {
	// InputPer1M is the cost per 1M prompt tokens.
	InputPer1M float64 `json:"input_per_1m"`
	// OutputPer1M is the cost per 1M completion tokens.
	OutputPer1M float64 `json:"output_per_1m"`
}

// TelemetryConfig controls optional telemetry.
type TelemetryConfig struct {
	// Enabled toggles telemetry emission.
	Enabled bool `json:"enabled"`
}

var (
	// ErrProviderConfigMissing is returned when the config file does not exist.
	ErrProviderConfigMissing = errors.New("provider config missing")
	// ErrProviderConfigInvalid is returned when required fields are missing.
	ErrProviderConfigInvalid = errors.New("provider config invalid")
)

// ProviderConfigPath returns the default provider config path.
func ProviderConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	// Store under ~/.openclaude to avoid conflicts with Claude Code.
	return filepath.Join(home, ".openclaude", "config.json"), nil
}

// LoadProviderConfig reads and validates the provider config.
func LoadProviderConfig(path string) (*ProviderConfig, error) {
	if path == "" {
		var err error
		path, err = ProviderConfigPath()
		if err != nil {
			return nil, err
		}
	}

	// Read the entire config file; it is expected to be small.
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrProviderConfigMissing
		}
		return nil, fmt.Errorf("read provider config: %w", err)
	}

	var cfg ProviderConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse provider config: %w", err)
	}

	// Validate required fields.
	if cfg.APIBaseURL == "" || cfg.APIKey == "" || cfg.DefaultModel == "" {
		return nil, ErrProviderConfigInvalid
	}

	// Apply defaults for optional fields.
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = 600000
	}

	if cfg.ModelAliases == nil {
		cfg.ModelAliases = make(map[string]string)
	}

	if cfg.Pricing == nil {
		cfg.Pricing = make(map[string]ModelPricing)
	}

	return &cfg, nil
}

// ResolveModel returns the resolved model for the session.
func ResolveModel(cfg *ProviderConfig, cliModel string, settingsModel string) string {
	// CLI input takes precedence over settings.
	if cliModel != "" {
		return aliasModel(cfg, cliModel)
	}
	if settingsModel != "" {
		return aliasModel(cfg, settingsModel)
	}
	return cfg.DefaultModel
}

// aliasModel resolves an alias to a provider model name.
func aliasModel(cfg *ProviderConfig, name string) string {
	if cfg == nil {
		return name
	}
	if aliased, ok := cfg.ModelAliases[name]; ok {
		return aliased
	}
	return name
}
