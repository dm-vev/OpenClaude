package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/openclaude/openclaude/internal/agent"
	"github.com/openclaude/openclaude/internal/llm/openai"
)

// interactiveStreamPrinter renders streaming output for interactive runs.
type interactiveStreamPrinter struct {
	// out is the primary output writer for assistant text.
	out io.Writer
	// errOut is used for warnings or informational messages.
	errOut io.Writer
	// verbose toggles extra tool output detail in the UI.
	verbose bool
	// wroteText tracks whether any text deltas were printed.
	wroteText bool
	// lineOpen tracks whether a streaming line is in progress.
	lineOpen bool
}

// newInteractiveStreamPrinter constructs a printer for interactive streaming.
func newInteractiveStreamPrinter(out io.Writer, errOut io.Writer, verbose bool) *interactiveStreamPrinter {
	return &interactiveStreamPrinter{
		out:     out,
		errOut:  errOut,
		verbose: verbose,
	}
}

// Reset clears state before a new streamed response begins.
func (p *interactiveStreamPrinter) Reset() {
	p.wroteText = false
	p.lineOpen = false
}

// EnsureNewline terminates a streaming line if one is active.
func (p *interactiveStreamPrinter) EnsureNewline() {
	if p == nil {
		return
	}
	if !p.lineOpen {
		return
	}
	fmt.Fprintln(p.out)
	p.lineOpen = false
}

// OnStreamStart resets state for a new streaming assistant response.
func (p *interactiveStreamPrinter) OnStreamStart(_ string) error {
	p.Reset()
	return nil
}

// OnStreamEvent prints incremental text deltas as they arrive.
func (p *interactiveStreamPrinter) OnStreamEvent(event openai.StreamResponse) error {
	for _, choice := range event.Choices {
		if choice.Index != 0 {
			continue
		}
		delta := choice.Delta
		if delta.Content == "" {
			continue
		}
		if !p.lineOpen {
			p.lineOpen = true
		}
		fmt.Fprint(p.out, delta.Content)
		p.wroteText = true
	}
	return nil
}

// OnStreamComplete ensures the assistant response ends with a newline.
func (p *interactiveStreamPrinter) OnStreamComplete(summary agent.StreamSummary) error {
	if p.wroteText {
		p.EnsureNewline()
		return nil
	}
	text := extractMessageText(summary.Message)
	if text != "" {
		fmt.Fprintln(p.out, text)
		return nil
	}
	p.EnsureNewline()
	return nil
}

// OnToolCall prints a short tool-start marker for interactive output.
func (p *interactiveStreamPrinter) OnToolCall(event agent.ToolEvent) error {
	if event.ToolName == "" {
		return nil
	}
	p.EnsureNewline()
	fmt.Fprintf(p.out, "-> tool %s started\n", event.ToolName)
	p.lineOpen = false
	return nil
}

// OnToolResult prints tool completion status and optional output summaries.
func (p *interactiveStreamPrinter) OnToolResult(event agent.ToolEvent, _ openai.Message) error {
	if event.ToolName == "" {
		return nil
	}
	p.EnsureNewline()
	status := "completed"
	if event.IsError {
		status = "failed"
	}
	fmt.Fprintf(p.out, "-> tool %s %s\n", event.ToolName, status)
	if event.IsError || p.verbose {
		summary := summarizeToolOutput(event.Result, 240)
		if summary != "" {
			fmt.Fprintf(p.out, "   output: %s\n", summary)
		}
	}
	p.lineOpen = false
	return nil
}

// buildInteractiveCallbacks wires the printer into stream callbacks.
func buildInteractiveCallbacks(printer *interactiveStreamPrinter) *agent.StreamCallbacks {
	return &agent.StreamCallbacks{
		OnStreamStart:    printer.OnStreamStart,
		OnStreamEvent:    printer.OnStreamEvent,
		OnStreamComplete: printer.OnStreamComplete,
		OnToolCall:       printer.OnToolCall,
		OnToolResult:     printer.OnToolResult,
	}
}

// handleSlashCommand routes known slash commands to stub handlers.
func handleSlashCommand(line string, opts *options) (bool, string) {
	if opts != nil && opts.DisableSlashCommands {
		return false, ""
	}
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "/") {
		return false, ""
	}
	trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "/"))
	if trimmed == "" {
		return false, ""
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return false, ""
	}
	command := strings.ToLower(parts[0])
	if !isKnownSlashCommand(command) {
		return true, fmt.Sprintf("Unknown command: /%s", command)
	}
	return true, fmt.Sprintf("Command /%s is not implemented in OpenClaude yet. See docs/compat.md.", command)
}

// isKnownSlashCommand checks against the canonical Claude Code list.
func isKnownSlashCommand(command string) bool {
	for _, known := range defaultSlashCommandList() {
		if strings.EqualFold(command, known) {
			return true
		}
	}
	return false
}

// summarizeToolArgs formats tool arguments for prompt display.
func summarizeToolArgs(args json.RawMessage, max int) string {
	if len(args) == 0 {
		return ""
	}
	trimmed := strings.TrimSpace(string(args))
	if trimmed == "" {
		return ""
	}
	compact := compactWhitespace(trimmed)
	return truncateForDisplay(compact, max)
}

// summarizeToolOutput formats tool output for optional display.
func summarizeToolOutput(output string, max int) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	compact := compactWhitespace(trimmed)
	return truncateForDisplay(compact, max)
}

// compactWhitespace collapses internal whitespace into single spaces.
func compactWhitespace(value string) string {
	fields := strings.Fields(value)
	return strings.Join(fields, " ")
}

// truncateForDisplay shortens long strings without breaking runes.
func truncateForDisplay(value string, max int) string {
	if max <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "...(truncated)"
}

// withInterrupt builds a context that is cancelled on SIGINT.
func withInterrupt(parent context.Context, onInterrupt func()) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	done := make(chan struct{})

	go func() {
		select {
		case <-interrupt:
			if onInterrupt != nil {
				onInterrupt()
			}
			cancel()
		case <-done:
			return
		}
	}()

	return ctx, func() {
		close(done)
		signal.Stop(interrupt)
		cancel()
	}
}

// formatInteractiveError normalizes common agent errors for TTY output.
func formatInteractiveError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "Request cancelled."
	case errors.Is(err, agent.ErrPlanMode):
		return "Plan mode is active. Use ExitPlanMode to enable tools."
	case errors.Is(err, agent.ErrMaxTurns):
		return "Max turns exceeded."
	case errors.Is(err, agent.ErrMaxBudget):
		return "Max budget exceeded."
	case errors.Is(err, agent.ErrToolDenied):
		return err.Error()
	default:
		return err.Error()
	}
}
