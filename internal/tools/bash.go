package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// maxCommandOutput limits combined stdout/stderr output.
const maxCommandOutput = 64 * 1024

// BashTool runs shell commands.
type BashTool struct{}

func (t *BashTool) Name() string {
	return "Bash"
}

func (t *BashTool) Description() string {
	return "Run a shell command."
}

func (t *BashTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to execute.",
			},
			"cwd": map[string]any{
				"type":        "string",
				"description": "Working directory.",
			},
		},
		"required": []string{"command"},
	}
}

func (t *BashTool) Run(ctx context.Context, input json.RawMessage, toolCtx ToolContext) (ToolResult, error) {
	var payload struct {
		Command string `json:"command"`
		CWD     string `json:"cwd"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if strings.TrimSpace(payload.Command) == "" {
		return ToolResult{IsError: true, Content: "command is required"}, nil
	}

	// Default to the current working directory, or validate the provided one.
	workingDir := toolCtx.CWD
	if payload.CWD != "" {
		resolved, err := toolCtx.Sandbox.ResolvePath(payload.CWD, true)
		if err != nil {
			return ToolResult{IsError: true, Content: err.Error()}, nil
		}
		workingDir = resolved
	}

	// Execute commands through bash -lc to match common CLI behavior.
	cmd := exec.CommandContext(ctx, "bash", "-lc", payload.Command)
	cmd.Dir = workingDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := strings.TrimSpace(stdout.String())
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += strings.TrimSpace(stderr.String())
	}

	// Truncate to keep responses bounded.
	if len(output) > maxCommandOutput {
		output = output[:maxCommandOutput] + "\n...[truncated]"
	}

	// Return errors with captured output for debugging.
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("command failed: %v\n%s", err, output)}, nil
	}

	return ToolResult{Content: output}, nil
}
