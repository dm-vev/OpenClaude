package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"golang.org/x/term"

	"github.com/openclaude/openclaude/internal/agent"
	"github.com/openclaude/openclaude/internal/llm/openai"
	"github.com/openclaude/openclaude/internal/session"
	"github.com/openclaude/openclaude/internal/tools"
)

// tuiMessageKind declares the semantic role of a chat line for formatting.
type tuiMessageKind string

const (
	// tuiMessageUserPrompt renders a standard user prompt line.
	tuiMessageUserPrompt tuiMessageKind = "user_prompt"
	// tuiMessageUserBash renders a direct bash input line.
	tuiMessageUserBash tuiMessageKind = "user_bash"
	// tuiMessageUserCommand renders a slash-command input line.
	tuiMessageUserCommand tuiMessageKind = "user_command"
	// tuiMessageAssistantText renders assistant text output.
	tuiMessageAssistantText tuiMessageKind = "assistant_text"
	// tuiMessageAssistantToolUse renders an assistant tool-use line.
	tuiMessageAssistantToolUse tuiMessageKind = "assistant_tool_use"
	// tuiMessageToolResult renders tool output or error lines.
	tuiMessageToolResult tuiMessageKind = "tool_result"
	// tuiMessageAssistantThinking renders a thinking block line.
	tuiMessageAssistantThinking tuiMessageKind = "assistant_thinking"
	// tuiMessageSystem renders a system or informational line.
	tuiMessageSystem tuiMessageKind = "system"
)

// tuiToolStatus captures tool execution state for display.
type tuiToolStatus string

const (
	// tuiToolQueued indicates the tool is queued and not yet running.
	tuiToolQueued tuiToolStatus = "queued"
	// tuiToolRunning indicates the tool is actively running.
	tuiToolRunning tuiToolStatus = "running"
	// tuiToolCompleted indicates the tool completed successfully.
	tuiToolCompleted tuiToolStatus = "completed"
	// tuiToolFailed indicates the tool finished with an error.
	tuiToolFailed tuiToolStatus = "failed"
)

// tuiInputMode tracks whether the prompt is in standard or bash mode.
type tuiInputMode string

const (
	// tuiInputPrompt is the normal prompt mode.
	tuiInputPrompt tuiInputMode = "prompt"
	// tuiInputBash is the direct bash execution mode.
	tuiInputBash tuiInputMode = "bash"
)

const (
	// tuiPasteIdleDelay defines the quiet window before a paste is finalized.
	tuiPasteIdleDelay = 100 * time.Millisecond
	// tuiPasteRuneThreshold triggers paste buffering for large rune counts.
	tuiPasteRuneThreshold = 800
	// tuiPasteLineThreshold triggers paste buffering for multi-line input.
	tuiPasteLineThreshold = 3
	// tuiLogoMinWidth matches Claude Code's minimum logo width.
	tuiLogoMinWidth = 46
	// tuiSlashSuggestionLimit caps the number of suggestions shown.
	tuiSlashSuggestionLimit = 7
	// tuiDoublePressWindow defines the maximum gap for double-press actions.
	tuiDoublePressWindow = 1 * time.Second
	// tuiToolSpinnerInterval defines the tool-use animation cadence.
	tuiToolSpinnerInterval = 600 * time.Millisecond
	// tuiSpinnerInterval defines the "thinking" spinner cadence.
	tuiSpinnerInterval = 120 * time.Millisecond
	// tuiMaxRenderedLines caps rendered tool output lines.
	tuiMaxRenderedLines = 50
)

// tuiSpinnerMessages mirrors the playful Claude Code spinner verbs.
var tuiSpinnerMessages = []string{
	"Accomplishing",
	"Actioning",
	"Actualizing",
	"Baking",
	"Brewing",
	"Calculating",
	"Cerebrating",
	"Churning",
	"Clauding",
	"Coalescing",
	"Cogitating",
	"Computing",
	"Conjuring",
	"Considering",
	"Cooking",
	"Crafting",
	"Creating",
	"Crunching",
	"Deliberating",
	"Determining",
	"Doing",
	"Effecting",
	"Finagling",
	"Forging",
	"Forming",
	"Generating",
	"Hatching",
	"Herding",
	"Honking",
	"Hustling",
	"Ideating",
	"Inferring",
	"Manifesting",
	"Marinating",
	"Moseying",
	"Mulling",
	"Mustering",
	"Musing",
	"Noodling",
	"Percolating",
	"Pondering",
	"Processing",
	"Puttering",
	"Reticulating",
	"Ruminating",
	"Schlepping",
	"Shucking",
	"Simmering",
	"Smooshing",
	"Spinning",
	"Stewing",
	"Synthesizing",
	"Thinking",
	"Transmuting",
	"Vibing",
	"Working",
}

// spinnerFrames returns the platform-specific Claude Code spinner frames.
func spinnerFrames() []string {
	base := []string{"·", "✢", "✳", "∗", "✻", "✽"}
	if runtime.GOOS != "darwin" {
		// Windows/Linux terminals sometimes render a green background for ✳.
		base = []string{"·", "✢", "*", "∗", "✻", "✽"}
	}
	frames := make([]string, 0, len(base)*2)
	frames = append(frames, base...)
	for index := len(base) - 1; index >= 0; index-- {
		frames = append(frames, base[index])
	}
	return frames
}

// assistantDot returns the black-circle glyph used by Claude Code.
func assistantDot() string {
	if runtime.GOOS == "darwin" {
		return "⏺"
	}
	return "●"
}

// pickSpinnerMessage selects a deterministic-but-varied spinner verb.
func pickSpinnerMessage() string {
	if len(tuiSpinnerMessages) == 0 {
		return "Thinking"
	}
	index := int(time.Now().UnixNano() % int64(len(tuiSpinnerMessages)))
	return tuiSpinnerMessages[index]
}

// tuiMessage is a rendered chat entry in the interactive UI.
type tuiMessage struct {
	// Kind selects the renderer used for the message.
	Kind tuiMessageKind
	// Role labels the message origin for debugging and fallback rendering.
	Role string
	// Content is the message text displayed in the chat viewport.
	Content string
	// ShowDot toggles the leading assistant indicator dot.
	ShowDot bool
	// ToolName is the tool name for tool-use and tool-result lines.
	ToolName string
	// ToolID associates tool-use and tool-result messages.
	ToolID string
	// ToolArgs holds a short argument summary for tool-use lines.
	ToolArgs string
	// ToolStatus indicates the current execution status for tool-use lines.
	ToolStatus tuiToolStatus
	// ToolError marks tool-result output as an error.
	ToolError bool
}

// streamDeltaMsg carries streamed text chunks into the TUI event loop.
type streamDeltaMsg struct {
	// Text is the assistant delta text chunk.
	Text string
}

// streamDoneMsg signals a completed streaming run with the final result.
type streamDoneMsg struct {
	// Result is the full run result to reconcile history.
	Result *agent.RunResult
}

// streamErrorMsg reports an error that occurred during streaming.
type streamErrorMsg struct {
	// Err is the underlying streaming error.
	Err error
}

// spinnerTickMsg toggles tool-use animation frames.
type spinnerTickMsg struct{}

// spinnerFrameMsg advances the main "thinking" spinner animation.
type spinnerFrameMsg struct{}

// pasteDoneMsg signals that a buffered paste should be finalized.
type pasteDoneMsg struct{}

// bashDoneMsg delivers the result of a direct bash invocation.
type bashDoneMsg struct {
	// ToolID ties the bash run to its tool-use line.
	ToolID string
	// Output captures the combined stdout/stderr output for display.
	Output string
	// IsError reports whether the bash run failed.
	IsError bool
}

// toolEventMsg wraps a tool event for the UI.
type toolEventMsg struct {
	// Event is the tool event emitted by the agent.
	Event agent.ToolEvent
}

// permissionRequest describes a tool permission prompt issued by the agent.
type permissionRequest struct {
	// ToolName is the tool being requested.
	ToolName string
	// Args holds the raw tool arguments for display.
	Args json.RawMessage
	// Response is used to return the user's decision.
	Response chan bool
}

// permissionRequestMsg delivers a permission prompt to the UI loop.
type permissionRequestMsg struct {
	// Request carries the tool permission prompt details.
	Request *permissionRequest
}

// tuiSlashSuggestion represents a slash command suggestion entry.
type tuiSlashSuggestion struct {
	// Name is the primary command name without the leading slash.
	Name string
	// Description provides short guidance for the command.
	Description string
	// Aliases lists any alternative trigger names.
	Aliases []string
	// AcceptsArgs reports whether the command expects arguments.
	AcceptsArgs bool
}

// tuiSelectorItem captures a selectable message for history forking.
type tuiSelectorItem struct {
	// Index references the message index in history.
	Index int
	// Preview is the human-readable preview text.
	Preview string
	// Input is the value to restore into the input field.
	Input string
	// Mode is the input mode to restore for the selection.
	Mode tuiInputMode
	// IsCurrent marks the virtual “current prompt” entry.
	IsCurrent bool
}

// tuiPendingPaste tracks a large paste placeholder and its original content.
type tuiPendingPaste struct {
	// Placeholder is the text inserted into the input buffer.
	Placeholder string
	// Content is the full pasted text to substitute on submit.
	Content string
}

// tuiPasteBuffer accumulates large paste chunks before finalizing.
type tuiPasteBuffer struct {
	// Chunks holds buffered paste chunks.
	Chunks []string
	// BaseValue stores the input value before paste buffering began.
	BaseValue string
	// Active reports whether a paste buffer is currently running.
	Active bool
	// Last marks the time of the most recent paste chunk.
	Last time.Time
}

// tuiToolState tracks tool-use UI state to allow updates on completion.
type tuiToolState struct {
	// Index points at the message entry representing the tool use.
	Index int
	// Status reflects the most recent tool status.
	Status tuiToolStatus
}

// tuiDoublePress tracks double-press exit/clear affordances.
type tuiDoublePress struct {
	// Key is the last key that was pressed.
	Key string
	// At records when the last key press happened.
	At time.Time
}

// tuiTheme collects the colors used for TUI rendering.
type tuiTheme struct {
	// Text is the primary foreground color.
	Text lipgloss.AdaptiveColor
	// Secondary is used for dim or secondary text.
	Secondary lipgloss.AdaptiveColor
	// SecondaryBorder is used for subtle borders and separators.
	SecondaryBorder lipgloss.AdaptiveColor
	// Bash highlights bash-mode prompts.
	Bash lipgloss.AdaptiveColor
	// Claude highlights Claude-branded accents like the logo.
	Claude lipgloss.AdaptiveColor
	// Permission highlights permission request borders and labels.
	Permission lipgloss.AdaptiveColor
	// Error highlights errors and warnings.
	Error lipgloss.AdaptiveColor
	// Success highlights successful completion.
	Success lipgloss.AdaptiveColor
	// Warning highlights cautionary text.
	Warning lipgloss.AdaptiveColor
	// Suggestion highlights slash command suggestions.
	Suggestion lipgloss.AdaptiveColor
}

const (
	// tuiInterruptMessage matches Claude Code's interrupt placeholder.
	tuiInterruptMessage = "[Request interrupted by user]"
	// tuiInterruptForToolMessage matches Claude Code tool interrupt placeholder.
	tuiInterruptForToolMessage = "[Request interrupted by user for tool use]"
	// tuiCancelMessage matches Claude Code cancel placeholder.
	tuiCancelMessage = "The user doesn't want to take this action right now. " +
		"STOP what you are doing and wait for the user to tell you how to proceed."
	// tuiRejectMessage matches Claude Code reject placeholder.
	tuiRejectMessage = "The user doesn't want to proceed with this tool use. " +
		"The tool use was rejected (eg. if it was a file edit, the new_string was " +
		"NOT written to the file). STOP what you are doing and wait for the user " +
		"to tell you how to proceed."
	// tuiNoResponseRequested matches Claude Code's no-response placeholder.
	tuiNoResponseRequested = "No response requested."
	// tuiPromptTooLongMessage matches Claude Code prompt-length errors.
	tuiPromptTooLongMessage = "Prompt is too long"
	// tuiCreditTooLowMessage matches Claude Code credit errors.
	tuiCreditTooLowMessage = "Credit balance is too low"
	// tuiInvalidAPIKeyMessage matches Claude Code invalid key errors.
	tuiInvalidAPIKeyMessage = "Invalid API key"
	// tuiAPIErrorPrefix matches Claude Code API error prefix.
	tuiAPIErrorPrefix = "API Error"
)

// tuiModel drives the interactive terminal UI for OpenClaude.
type tuiModel struct {
	// opts holds CLI options to respect slash-command disabling.
	opts *options
	// runner executes agent runs and tool calls.
	runner *agent.Runner
	// store persists session history.
	store *session.Store
	// sessionID identifies the current session.
	sessionID string
	// model is the current model identifier.
	model string
	// systemPrompt is the resolved system prompt string.
	systemPrompt string
	// history is the full message history used for agent calls.
	history []openai.Message
	// chatMessages holds display-friendly message entries.
	chatMessages []tuiMessage
	// toolLines keeps a rolling log of tool events.
	toolLines []string
	// toolStates tracks tool-use message indices for updates.
	toolStates map[string]tuiToolState
	// inputHistory stores prior user inputs for recall.
	inputHistory []string
	// historyIndex tracks the active position in inputHistory.
	historyIndex int
	// historyDraft preserves the in-progress input when browsing history.
	historyDraft string
	// chatView renders the main conversation history.
	chatView viewport.Model
	// toolView renders tool activity information.
	toolView viewport.Model
	// input collects user input for new turns.
	input textarea.Model
	// inputMode controls prompt behavior and bash execution.
	inputMode tuiInputMode
	// submitCount tracks how many prompts have been submitted.
	submitCount int
	// slashSuggestions holds the current slash command suggestions.
	slashSuggestions []tuiSlashSuggestion
	// slashSelection indexes the currently highlighted suggestion.
	slashSelection int
	// showMessageSelector toggles the history selector overlay.
	showMessageSelector bool
	// selectorItems contains the selectable history entries.
	selectorItems []tuiSelectorItem
	// selectorIndex tracks the active selection in selectorItems.
	selectorIndex int
	// pendingPaste holds a large paste placeholder awaiting submission.
	pendingPaste *tuiPendingPaste
	// pasteBuffer accumulates large paste chunks before finalizing.
	pasteBuffer tuiPasteBuffer
	// markdownRenderer formats assistant output when available.
	markdownRenderer *glamour.TermRenderer
	// statusText is the bottom status line.
	statusText string
	// inputHint shows short-lived messages under the input.
	inputHint string
	// lastUsage tracks token usage for the most recent run.
	lastUsage openai.Usage
	// totalCost tracks accumulated cost across runs.
	totalCost float64
	// chatAutoScroll keeps the chat viewport pinned to the bottom.
	chatAutoScroll bool
	// toolAutoScroll keeps the tool viewport pinned to the bottom.
	toolAutoScroll bool
	// width tracks the terminal width.
	width int
	// height tracks the terminal height.
	height int
	// activePane identifies which pane is focused.
	activePane string
	// permissionMode reports the current tool permission mode.
	permissionMode string
	// planMode indicates whether plan-only mode is active.
	planMode bool
	// running indicates an in-flight request.
	running bool
	// streamBuffer accumulates streamed assistant text.
	streamBuffer strings.Builder
	// streamCh delivers stream messages into the update loop.
	streamCh chan tea.Msg
	// cancel cancels the current request when present.
	cancel context.CancelFunc
	// pendingPermission is the active permission prompt, when any.
	pendingPermission *permissionRequest
	// quitting indicates a user-requested exit.
	quitting bool
	// spinnerOn toggles animated tool-use indicators.
	spinnerOn bool
	// spinnerFrames stores the animated glyphs for the main spinner.
	spinnerFrames []string
	// spinnerFrame indexes the current spinner frame.
	spinnerFrame int
	// spinnerMessage stores the current spinner verb.
	spinnerMessage string
	// spinnerStarted records when the spinner began.
	spinnerStarted time.Time
	// spinnerEnabled gates whether the main spinner should be shown.
	spinnerEnabled bool
	// doublePress tracks exit/clear key timing.
	doublePress tuiDoublePress
	// theme holds colors for rendering.
	theme tuiTheme
}

// runInteractiveTUI starts the full-screen terminal UI for interactive sessions.
func runInteractiveTUI(
	opts *options,
	runner *agent.Runner,
	history []openai.Message,
	systemPrompt string,
	model string,
	sessionID string,
	store *session.Store,
) error {
	if !term.IsTerminal(int(0)) || !term.IsTerminal(int(1)) {
		return errors.New("interactive TUI requires a TTY")
	}
	modelState := newTUIModel(opts, runner, history, systemPrompt, model, sessionID, store)
	program := tea.NewProgram(modelState, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

// newTUIModel constructs the initial TUI model state.
func newTUIModel(
	opts *options,
	runner *agent.Runner,
	history []openai.Message,
	systemPrompt string,
	model string,
	sessionID string,
	store *session.Store,
) *tuiModel {
	input := textarea.New()
	input.Focus()
	input.CharLimit = 0
	input.Prompt = ""
	input.SetHeight(3)
	input.SetWidth(20)

	chatView := viewport.New(20, 10)
	toolView := viewport.New(20, 10)
	toolView.SetContent("No tool activity yet.")

	var renderer *glamour.TermRenderer
	if glam, err := glamour.NewTermRenderer(glamour.WithAutoStyle()); err == nil {
		renderer = glam
	}

	modelState := &tuiModel{
		opts:             opts,
		runner:           runner,
		store:            store,
		sessionID:        sessionID,
		model:            model,
		systemPrompt:     systemPrompt,
		history:          ensureSystem(history, systemPrompt),
		chatView:         chatView,
		toolView:         toolView,
		input:            input,
		inputMode:        tuiInputPrompt,
		toolStates:       map[string]tuiToolState{},
		slashSelection:   -1,
		spinnerFrames:    spinnerFrames(),
		theme:            defaultTUITheme(),
		markdownRenderer: renderer,
		statusText:       "",
		activePane:       "input",
		chatAutoScroll:   true,
		toolAutoScroll:   true,
	}
	if runner != nil {
		modelState.permissionMode = string(runner.Permissions.Mode)
	}
	modelState.syncInputPrompt()
	modelState.refreshPlanMode()
	modelState.historyIndex = len(modelState.inputHistory)
	modelState.bootstrapHistory()
	return modelState
}

// Init starts the blinking cursor for the input field.
func (m *tuiModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.scheduleSpinnerTick(), m.scheduleSpinnerFrameTick())
}

// Update handles UI events and streaming updates.
func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.applyWindowSize(typed)
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(typed)
	case spinnerTickMsg:
		m.spinnerOn = !m.spinnerOn
		m.refreshChat()
		return m, m.scheduleSpinnerTick()
	case spinnerFrameMsg:
		m.advanceSpinnerFrame()
		return m, m.scheduleSpinnerFrameTick()
	case pasteDoneMsg:
		return m, m.finalizePaste()
	case streamDeltaMsg:
		m.streamBuffer.WriteString(typed.Text)
		m.refreshChat()
		return m, m.listenStream()
	case toolEventMsg:
		m.appendToolEvent(typed.Event)
		return m, tea.Batch(m.listenStream(), m.scheduleSpinnerTick())
	case permissionRequestMsg:
		m.handlePermissionRequest(typed.Request)
		return m, m.listenStream()
	case bashDoneMsg:
		m.finishBash(typed)
		return m, nil
	case streamDoneMsg:
		m.finishRun(typed.Result)
		return m, nil
	case streamErrorMsg:
		m.finishError(typed.Err)
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// View renders the full UI layout.
func (m *tuiModel) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		return "Initializing..."
	}
	m.updateLayout()

	sections := []string{m.renderBody()}
	if spinner := m.renderSpinner(); spinner != "" {
		sections = append(sections, spinner)
	}
	if permission := m.renderPermissionRequest(); permission != "" {
		sections = append(sections, permission)
	}
	if m.showMessageSelector {
		sections = append(sections, m.renderMessageSelector())
	}
	if input := m.renderInput(); input != "" {
		sections = append(sections, input)
	}
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// handleKey routes keyboard input and command submission.
func (m *tuiModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pendingPermission != nil {
		switch strings.ToLower(key.String()) {
		case "y":
			m.resolvePermission(true)
			return m, nil
		case "n", "esc", "enter":
			m.resolvePermission(false)
			return m, nil
		}
	}

	if m.showMessageSelector {
		return m.handleSelectorKey(key)
	}

	if handled, cmd := m.handleSuggestionKey(key); handled {
		return m, cmd
	}

	switch key.String() {
	case "ctrl+c":
		if m.running {
			m.cancelRun("Cancelled.")
			m.appendInterruptMessage()
			m.refreshChat()
			return m, nil
		}
		if m.doublePressTriggered("ctrl+c", "Press Ctrl-C again to exit.") {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	case "ctrl+d":
		if m.input.Value() == "" {
			if m.doublePressTriggered("ctrl+d", "Press Ctrl-D again to exit.") {
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}
	case "ctrl+l":
		m.input.SetValue("")
		m.input.CursorEnd()
		m.clearInputHints()
		return m, nil
	case "ctrl+q":
		m.quitting = true
		return m, tea.Quit
	case "tab":
		m.cyclePane(1)
		return m, nil
	case "shift+tab":
		m.cyclePane(-1)
		return m, nil
	case "esc":
		if m.input.Value() == "" && !m.running && len(m.chatMessages) > 0 {
			m.openMessageSelector()
			return m, nil
		}
		if m.doublePressTriggered("esc", "Press Escape again to clear.") {
			m.input.SetValue("")
			m.input.CursorEnd()
			m.setInputMode(tuiInputPrompt)
			return m, nil
		}
		m.setActivePane("input")
		return m, nil
	case "pgup":
		m.scrollActivePane(-10)
		return m, nil
	case "pgdown":
		m.scrollActivePane(10)
		return m, nil
	case "home":
		m.gotoActivePaneTop()
		return m, nil
	case "end":
		m.gotoActivePaneBottom()
		return m, nil
	case "ctrl+p":
		if m.activePane == "input" {
			m.cycleInputHistory(-1)
			return m, nil
		}
	case "ctrl+n":
		if m.activePane == "input" {
			m.cycleInputHistory(1)
			return m, nil
		}
	}

	if key.Type == tea.KeyEnter {
		if key.Alt {
			m.input.InsertString("\n")
			return m, nil
		}
		if m.shouldInsertContinuationNewline() {
			return m, nil
		}
		return m.submitInput()
	}

	if key.String() == "ctrl+j" {
		m.input.InsertString("\n")
		return m, nil
	}

	if m.activePane != "input" {
		switch key.String() {
		case "up":
			m.scrollActivePane(-1)
			return m, nil
		case "down":
			m.scrollActivePane(1)
			return m, nil
		case "left":
			m.scrollActivePane(-1)
			return m, nil
		case "right":
			m.scrollActivePane(1)
			return m, nil
		}
	}

	if m.handleHistoryArrows(key) {
		return m, nil
	}

	var cmd tea.Cmd
	previousValue := m.input.Value()
	m.input, cmd = m.input.Update(key)
	if m.input.Value() != previousValue {
		pasteCmd := m.handlePasteKey(key, previousValue)
		m.syncInputState()
		if m.input.Value() == "" && (key.String() == "backspace" || key.String() == "delete") {
			m.setInputMode(tuiInputPrompt)
		}
		if pasteCmd != nil && cmd != nil {
			cmd = tea.Batch(cmd, pasteCmd)
		} else if pasteCmd != nil {
			cmd = pasteCmd
		}
	}
	return m, cmd
}

// submitInput sends the current input as a new user message.
func (m *tuiModel) submitInput() (tea.Model, tea.Cmd) {
	if m.running {
		m.statusText = "Wait for the current response or cancel with Ctrl+C."
		return m, nil
	}
	rawValue := m.input.Value()
	value := strings.TrimSpace(m.resolvePastedInput(rawValue))
	if value == "" {
		return m, nil
	}
	modeAtSubmit := m.inputMode
	m.submitCount++
	m.input.SetValue("")
	m.setInputMode(tuiInputPrompt)
	m.clearInputHints()
	m.statusText = ""
	m.appendInputHistory(value, modeAtSubmit)

	if modeAtSubmit == tuiInputBash {
		return m.submitBash(value)
	}

	if handled, output := handleSlashCommand(value, m.opts); handled {
		m.appendUserCommand(value)
		if output != "" {
			m.appendSystemMessage(output)
		}
		m.refreshChat()
		return m, nil
	}

	m.appendUserPrompt(value)
	m.refreshChat()

	m.history = append(m.history, openai.Message{Role: "user", Content: value})
	m.running = true
	m.startSpinner()
	m.streamBuffer.Reset()
	m.toolLines = nil
	m.toolView.SetContent("No tool activity yet.")
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.statusText = "Thinking..."
	m.streamCh = make(chan tea.Msg, 128)
	m.configureAuthorizer(ctx)

	cmd := m.startStream(ctx)
	return m, tea.Batch(cmd, m.listenStream(), m.scheduleSpinnerTick(), m.scheduleSpinnerFrameTick())
}

// startSpinner initializes the "thinking" spinner state for a new run.
func (m *tuiModel) startSpinner() {
	m.spinnerMessage = pickSpinnerMessage()
	m.spinnerStarted = time.Now()
	m.spinnerFrame = 0
	m.spinnerEnabled = true
}

// submitBash executes a direct bash command without invoking the agent loop.
func (m *tuiModel) submitBash(command string) (tea.Model, tea.Cmd) {
	if m.runner == nil || m.runner.ToolRunner == nil {
		// The UI cannot execute bash without an available tool runner.
		m.appendSystemMessage("Bash tool is unavailable.")
		m.refreshChat()
		return m, nil
	}
	if m.runner.Permissions.Mode == tools.PermissionPlan || tools.IsPlanMode(m.store, m.sessionID) {
		// Plan mode forbids tool execution, so report and bail early.
		m.appendSystemMessage("Tools are disabled in plan mode.")
		m.refreshChat()
		return m, nil
	}

	// Mirror Claude Code by storing bash input as a tagged user message.
	userTag := fmt.Sprintf("<bash-input>%s</bash-input>", command)
	m.appendUserBash(command)
	m.history = append(m.history, openai.Message{Role: "user", Content: userTag})
	if m.store != nil {
		// Persist the user message immediately so session history stays ordered.
		if err := persistSession(m.store, m.sessionID, []openai.Message{{Role: "user", Content: userTag}}, nil); err != nil {
			m.statusText = err.Error()
		}
	}

	// Handle local "cd" commands without invoking the Bash tool.
	if handled, output, isError := m.handleBashCD(command); handled {
		resultTag := wrapBashOutput(output, isError)
		m.appendAssistantText(resultTag)
		assistantMessage := openai.Message{Role: "assistant", Content: resultTag}
		m.history = append(m.history, assistantMessage)
		if m.store != nil {
			// Persist the synthetic assistant output for the cd operation.
			if err := persistSession(m.store, m.sessionID, []openai.Message{assistantMessage}, nil); err != nil {
				m.statusText = err.Error()
			}
		}
		m.refreshChat()
		return m, nil
	}

	toolID := uuid.NewString()
	argsPayload, err := json.Marshal(struct {
		Command string `json:"command"`
	}{
		Command: command,
	})
	if err != nil {
		// JSON marshaling failures are unexpected but must be surfaced.
		m.appendSystemMessage(fmt.Sprintf("Failed to encode bash command: %v", err))
		m.refreshChat()
		return m, nil
	}

	// Show a tool-use line in the chat while the command executes.
	m.appendToolUseMessage(agent.ToolEvent{
		Type:      "tool_call",
		ToolName:  "Bash",
		ToolID:    toolID,
		Arguments: argsPayload,
	}, tuiToolRunning)
	m.refreshChat()

	// Disable concurrent submissions until the bash run completes.
	m.running = true
	m.spinnerEnabled = false
	m.statusText = "Running Bash..."
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.streamCh = make(chan tea.Msg, 8)
	m.configureAuthorizer(ctx)

	cmd := func() tea.Msg {
		// Ask for permission if required by the configured policy.
		if m.runner.AuthorizeTool != nil && m.runner.Permissions.ShouldPrompt("Bash") {
			allowed, err := m.runner.AuthorizeTool("Bash", argsPayload)
			if err != nil {
				m.streamCh <- bashDoneMsg{ToolID: toolID, Output: err.Error(), IsError: true}
				close(m.streamCh)
				return nil
			}
			if !allowed {
				m.streamCh <- bashDoneMsg{ToolID: toolID, Output: "Tool denied.", IsError: true}
				close(m.streamCh)
				return nil
			}
		}

		// Execute the tool call and normalize any errors into the result payload.
		result, runErr := m.runner.ToolRunner.Run(ctx, "Bash", argsPayload, m.runner.ToolContext)
		if runErr != nil {
			result = tools.ToolResult{IsError: true, Content: runErr.Error()}
		}
		m.streamCh <- bashDoneMsg{ToolID: toolID, Output: result.Content, IsError: result.IsError}
		close(m.streamCh)
		return nil
	}

	return m, tea.Batch(cmd, m.listenStream())
}

// finishBash reconciles a completed bash invocation into history.
func (m *tuiModel) finishBash(message bashDoneMsg) {
	// Reset running state before mutating chat output.
	m.running = false
	m.spinnerEnabled = false
	m.statusText = ""
	m.cancel = nil
	m.pendingPermission = nil
	m.streamCh = nil

	if message.ToolID != "" {
		// Update the tool-use indicator to show completion status.
		status := tuiToolCompleted
		if message.IsError {
			status = tuiToolFailed
		}
		m.updateToolUseStatus(message.ToolID, status)
	}

	// Wrap tool output in bash tags so the renderer can display it consistently.
	resultTag := wrapBashOutput(message.Output, message.IsError)
	m.appendAssistantText(resultTag)
	assistantMessage := openai.Message{Role: "assistant", Content: resultTag}
	m.history = append(m.history, assistantMessage)
	if m.store != nil {
		// Persist the assistant output so the session can be replayed.
		if err := persistSession(m.store, m.sessionID, []openai.Message{assistantMessage}, nil); err != nil {
			m.statusText = err.Error()
		}
	}
	m.refreshChat()
}

// handleBashCD handles a direct "cd" command, updating the tool context.
func (m *tuiModel) handleBashCD(command string) (bool, string, bool) {
	trimmed := strings.TrimSpace(command)
	if !strings.HasPrefix(trimmed, "cd ") {
		return false, "", false
	}
	// Resolve "cd" paths relative to the current tool context.
	target := strings.TrimSpace(strings.TrimPrefix(trimmed, "cd "))
	if target == "" {
		return true, "cwd error: missing path", true
	}
	baseDir := m.runner.ToolContext.CWD
	if baseDir == "" {
		baseDir = mustCwd()
	}
	requested := filepath.Join(baseDir, target)
	// Use the sandbox to enforce allow/deny rules before updating CWD.
	resolved, err := resolveCWDPath(m.runner.ToolContext.Sandbox, requested)
	if err != nil {
		return true, fmt.Sprintf("cwd error: %v", err), true
	}
	m.runner.ToolContext.CWD = resolved
	return true, fmt.Sprintf("Changed directory to %s/", resolved), false
}

// resolveCWDPath validates the requested cwd against the sandbox.
func resolveCWDPath(sandbox *tools.Sandbox, path string) (string, error) {
	// Fall back to absolute resolution when no sandbox is configured.
	if sandbox == nil {
		return filepath.Abs(path)
	}
	return sandbox.ResolvePath(path, true)
}

// updateToolUseStatus mutates an existing tool-use message if it exists.
func (m *tuiModel) updateToolUseStatus(toolID string, status tuiToolStatus) {
	if toolID == "" {
		return
	}
	state, ok := m.toolStates[toolID]
	if !ok {
		return
	}
	// Mutate the existing message in place so the indicator updates.
	if state.Index >= 0 && state.Index < len(m.chatMessages) {
		updated := m.chatMessages[state.Index]
		updated.ToolStatus = status
		m.chatMessages[state.Index] = updated
	}
	state.Status = status
	m.toolStates[toolID] = state
}

// wrapBashOutput formats stdout/stderr tags for bash output rendering.
func wrapBashOutput(output string, isError bool) string {
	// Keep tags consistent with the upstream Claude Code format.
	if isError {
		return fmt.Sprintf("<bash-stderr>%s</bash-stderr>", output)
	}
	return fmt.Sprintf("<bash-stdout>%s</bash-stdout>", output)
}

// appendInputHistory records an input line for history navigation.
func (m *tuiModel) appendInputHistory(value string, mode tuiInputMode) {
	if value == "" {
		return
	}
	historyValue := value
	if mode == tuiInputBash {
		historyValue = "!" + value
	}
	m.inputHistory = append(m.inputHistory, historyValue)
	if len(m.inputHistory) > 200 {
		m.inputHistory = m.inputHistory[len(m.inputHistory)-200:]
	}
	m.historyIndex = len(m.inputHistory)
	m.historyDraft = ""
}

// cycleInputHistory moves the input buffer through stored history entries.
func (m *tuiModel) cycleInputHistory(delta int) {
	if len(m.inputHistory) == 0 {
		return
	}
	if m.historyIndex == len(m.inputHistory) {
		m.historyDraft = m.input.Value()
	}
	next := m.historyIndex + delta
	if next < 0 {
		next = 0
	}
	if next > len(m.inputHistory) {
		next = len(m.inputHistory)
	}
	m.historyIndex = next
	if m.historyIndex == len(m.inputHistory) {
		m.input.SetValue(m.historyDraft)
		m.setInputMode(tuiInputPrompt)
		return
	}
	entry := m.inputHistory[m.historyIndex]
	if strings.HasPrefix(entry, "!") {
		m.setInputMode(tuiInputBash)
		entry = strings.TrimPrefix(entry, "!")
	} else {
		m.setInputMode(tuiInputPrompt)
	}
	m.input.SetValue(entry)
	m.syncInputState()
}

// configureAuthorizer wires tool permission prompts into the interactive UI.
func (m *tuiModel) configureAuthorizer(ctx context.Context) {
	if m.runner == nil {
		return
	}
	streamCh := m.streamCh
	m.runner.AuthorizeTool = func(name string, args json.RawMessage) (bool, error) {
		if !m.runner.Permissions.ShouldPrompt(name) {
			return true, nil
		}
		request := &permissionRequest{
			ToolName: name,
			Args:     args,
			Response: make(chan bool, 1),
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case streamCh <- permissionRequestMsg{Request: request}:
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case allowed := <-request.Response:
			return allowed, nil
		}
	}
}

// startStream launches the agent run and feeds updates into the stream channel.
func (m *tuiModel) startStream(ctx context.Context) tea.Cmd {
	history := append([]openai.Message(nil), m.history...)
	runner := m.runner
	modelName := m.model
	toolsEnabled := runner != nil && runner.ToolRunner != nil
	streamCh := m.streamCh

	return func() tea.Msg {
		if runner == nil {
			streamCh <- streamErrorMsg{Err: errors.New("runner is required")}
			close(streamCh)
			return nil
		}

		callbacks := &agent.StreamCallbacks{
			OnStreamStart: func(_ string) error {
				return nil
			},
			OnStreamEvent: func(event openai.StreamResponse) error {
				for _, choice := range event.Choices {
					if choice.Index != 0 {
						continue
					}
					if choice.Delta.Content == "" {
						continue
					}
					select {
					case <-ctx.Done():
						return ctx.Err()
					case streamCh <- streamDeltaMsg{Text: choice.Delta.Content}:
					}
				}
				return nil
			},
			OnToolCall: func(event agent.ToolEvent) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case streamCh <- toolEventMsg{Event: event}:
				}
				return nil
			},
			OnToolResult: func(event agent.ToolEvent, _ openai.Message) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case streamCh <- toolEventMsg{Event: event}:
				}
				return nil
			},
		}

		result, err := runner.RunStream(ctx, history, "", modelName, toolsEnabled, callbacks)
		if err != nil {
			streamCh <- streamErrorMsg{Err: err}
			close(streamCh)
			return nil
		}
		streamCh <- streamDoneMsg{Result: result}
		close(streamCh)
		return nil
	}
}

// listenStream waits for the next streaming message.
func (m *tuiModel) listenStream() tea.Cmd {
	if m.streamCh == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-m.streamCh
		if !ok {
			return nil
		}
		return msg
	}
}

// finishRun reconciles history and appends the final assistant message.
func (m *tuiModel) finishRun(result *agent.RunResult) {
	m.running = false
	m.spinnerEnabled = false
	m.statusText = ""
	m.cancel = nil
	m.pendingPermission = nil
	if result == nil {
		m.appendAssistantText(m.streamBuffer.String())
		m.streamBuffer.Reset()
		m.refreshChat()
		return
	}
	m.history = result.Messages
	m.lastUsage = result.Usage
	m.totalCost = result.CostUSD
	finalText := formatContent(result.Final.Content)
	if finalText == "" {
		finalText = m.streamBuffer.String()
	}
	m.appendAssistantText(finalText)
	m.streamBuffer.Reset()
	m.refreshChat()
	if m.store != nil {
		m.persistRun(result)
	}
}

// finishError handles errors from the streaming run.
func (m *tuiModel) finishError(err error) {
	m.running = false
	m.spinnerEnabled = false
	m.statusText = formatInteractiveError(err)
	m.cancel = nil
	m.pendingPermission = nil
	m.streamBuffer.Reset()
}

// cancelRun cancels an in-flight request and updates status.
func (m *tuiModel) cancelRun(reason string) {
	if m.cancel != nil {
		m.cancel()
	}
	if m.pendingPermission != nil {
		m.resolvePermission(false)
	}
	m.spinnerEnabled = false
	m.statusText = reason
}

// handlePermissionRequest stores the prompt and updates UI state.
func (m *tuiModel) handlePermissionRequest(request *permissionRequest) {
	if request == nil {
		return
	}
	m.pendingPermission = request
	m.input.Blur()
	summary := summarizeToolArgs(request.Args, 160)
	if summary != "" {
		m.toolLines = append(m.toolLines, fmt.Sprintf("%s args: %s", request.ToolName, summary))
		m.refreshTools()
	}
	m.statusText = fmt.Sprintf("Allow tool %s? [y/N]", request.ToolName)
}

// resolvePermission sends the user's decision back to the agent loop.
func (m *tuiModel) resolvePermission(allowed bool) {
	request := m.pendingPermission
	m.pendingPermission = nil
	if request != nil {
		select {
		case request.Response <- allowed:
		default:
		}
	}
	m.input.Focus()
	if allowed {
		m.statusText = "Tool allowed."
	} else {
		m.statusText = "Tool denied."
	}
}

// appendMessage adds a new chat message to the display list.
func (m *tuiModel) appendMessage(role string, content string) {
	m.chatMessages = append(m.chatMessages, tuiMessage{Role: role, Content: content})
}

// appendToolEvent records tool activity for the side panel.
func (m *tuiModel) appendToolEvent(event agent.ToolEvent) {
	if event.ToolName == "" {
		return
	}
	if event.Type == "tool_call" {
		m.appendToolUseMessage(event, tuiToolRunning)
	}
	if event.Type == "tool_result" {
		m.appendToolResultMessage(event)
	}
	status := "started"
	if event.Type == "tool_result" {
		status = "completed"
		if event.IsError {
			status = "failed"
		}
	}
	line := fmt.Sprintf("%s: %s", event.ToolName, status)
	m.toolLines = append(m.toolLines, line)
	if event.Type == "tool_result" {
		summary := summarizeToolOutput(event.Result, 160)
		if summary != "" {
			m.toolLines = append(m.toolLines, "  "+summary)
		}
	}
	if len(m.toolLines) > 200 {
		m.toolLines = m.toolLines[len(m.toolLines)-200:]
	}
	m.refreshTools()
	m.refreshChat()
	m.refreshPlanMode()
}

// appendUserPrompt stores a user prompt in the chat view.
func (m *tuiModel) appendUserPrompt(text string) {
	m.chatMessages = append(m.chatMessages, tuiMessage{
		Kind:    tuiMessageUserPrompt,
		Role:    "user",
		Content: text,
	})
}

// appendUserBash stores a user bash command in the chat view.
func (m *tuiModel) appendUserBash(command string) {
	m.chatMessages = append(m.chatMessages, tuiMessage{
		Kind:    tuiMessageUserBash,
		Role:    "user",
		Content: command,
	})
}

// appendUserCommand stores a user slash command in the chat view.
func (m *tuiModel) appendUserCommand(command string) {
	m.chatMessages = append(m.chatMessages, tuiMessage{
		Kind:    tuiMessageUserCommand,
		Role:    "user",
		Content: command,
	})
}

// appendAssistantText stores assistant output in the chat view.
func (m *tuiModel) appendAssistantText(text string) {
	m.chatMessages = append(m.chatMessages, tuiMessage{
		Kind:    tuiMessageAssistantText,
		Role:    "assistant",
		Content: text,
		ShowDot: true,
	})
}

// appendSystemMessage stores a system informational message in the chat view.
func (m *tuiModel) appendSystemMessage(text string) {
	m.chatMessages = append(m.chatMessages, tuiMessage{
		Kind:    tuiMessageSystem,
		Role:    "system",
		Content: text,
	})
}

// appendInterruptMessage records an interrupted-by-user placeholder.
func (m *tuiModel) appendInterruptMessage() {
	m.appendAssistantText(tuiInterruptMessage)
}

// appendToolUseMessage records a tool-use announcement in the chat view.
func (m *tuiModel) appendToolUseMessage(event agent.ToolEvent, status tuiToolStatus) {
	if event.ToolName == "" {
		return
	}
	toolArgs := summarizeToolArgs(event.Arguments, 120)
	message := tuiMessage{
		Kind:       tuiMessageAssistantToolUse,
		Role:       "assistant",
		ShowDot:    true,
		ToolName:   event.ToolName,
		ToolID:     event.ToolID,
		ToolArgs:   toolArgs,
		ToolStatus: status,
	}
	index := len(m.chatMessages)
	m.chatMessages = append(m.chatMessages, message)
	if event.ToolID != "" {
		m.toolStates[event.ToolID] = tuiToolState{Index: index, Status: status}
	}
}

// appendToolResultMessage records a tool result and updates tool status.
func (m *tuiModel) appendToolResultMessage(event agent.ToolEvent) {
	if event.ToolID != "" {
		status := tuiToolCompleted
		if event.IsError {
			status = tuiToolFailed
		}
		if state, ok := m.toolStates[event.ToolID]; ok {
			if state.Index >= 0 && state.Index < len(m.chatMessages) {
				updated := m.chatMessages[state.Index]
				updated.ToolStatus = status
				m.chatMessages[state.Index] = updated
			}
			state.Status = status
			m.toolStates[event.ToolID] = state
		}
	}
	m.chatMessages = append(m.chatMessages, tuiMessage{
		Kind:      tuiMessageToolResult,
		Role:      "tool",
		Content:   event.Result,
		ToolName:  event.ToolName,
		ToolID:    event.ToolID,
		ToolError: event.IsError,
	})
}

// appendUserMessageFromHistory reconstructs a user message from stored history.
func (m *tuiModel) appendUserMessageFromHistory(message openai.Message) {
	rawText := extractMessageText(message)
	if rawText == "" && message.Content != nil {
		rawText = formatContent(message.Content)
	}
	if rawText == "" {
		return
	}
	// Preserve bash and command tags so the renderer can rehydrate UX.
	if bashInput := extractTag(rawText, "bash-input"); bashInput != "" {
		m.appendUserBash(bashInput)
		return
	}
	if strings.Contains(rawText, "<command-message>") || strings.Contains(rawText, "<command-name>") {
		m.appendUserCommand(rawText)
		return
	}
	if strings.HasPrefix(strings.TrimSpace(rawText), "/") {
		m.appendUserCommand(rawText)
		return
	}
	m.appendUserPrompt(rawText)
}

// appendAssistantMessageFromHistory reconstructs assistant output from history.
func (m *tuiModel) appendAssistantMessageFromHistory(message openai.Message) {
	// Recreate tool-use lines before rendering assistant text.
	for _, call := range message.ToolCalls {
		arguments := json.RawMessage(call.Function.Arguments)
		m.appendToolUseMessage(agent.ToolEvent{
			Type:      "tool_call",
			ToolName:  call.Function.Name,
			ToolID:    call.ID,
			Arguments: arguments,
		}, tuiToolRunning)
	}
	text := extractMessageText(message)
	if text == "" && message.Content != nil {
		text = formatContent(message.Content)
	}
	if strings.TrimSpace(text) == "" {
		return
	}
	m.appendAssistantText(text)
}

// appendToolResultFromHistory reconstructs tool result lines from history.
func (m *tuiModel) appendToolResultFromHistory(message openai.Message, toolNames map[string]string) {
	content := formatContent(message.Content)
	if content == "" {
		content = extractMessageText(message)
	}
	// Tool results in history do not include explicit error flags.
	m.appendToolResultMessage(agent.ToolEvent{
		Type:     "tool_result",
		ToolName: toolNames[message.ToolCallID],
		ToolID:   message.ToolCallID,
		Result:   content,
		IsError:  false,
	})
}

// refreshChat rebuilds the chat viewport content.
func (m *tuiModel) refreshChat() {
	var builder strings.Builder
	welcome := m.renderWelcome()
	if welcome != "" {
		builder.WriteString(welcome)
		builder.WriteString("\n\n")
	}
	for _, msg := range m.chatMessages {
		builder.WriteString(m.renderMessage(msg, false))
		builder.WriteString("\n\n")
	}
	if m.running {
		streamText := m.streamBuffer.String()
		if streamText != "" {
			builder.WriteString(
				m.renderMessage(
					tuiMessage{Kind: tuiMessageAssistantText, Role: "assistant", Content: streamText, ShowDot: true},
					true,
				),
			)
			builder.WriteString("\n\n")
		}
	}
	m.chatView.SetContent(builder.String())
	if m.chatAutoScroll {
		m.chatView.GotoBottom()
	}
}

// scheduleSpinnerTick schedules tool-use animation ticks when needed.
func (m *tuiModel) scheduleSpinnerTick() tea.Cmd {
	if !m.shouldAnimateTools() {
		return nil
	}
	return tea.Tick(tuiToolSpinnerInterval, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// shouldAnimateTools reports whether the tool-use indicator should blink.
func (m *tuiModel) shouldAnimateTools() bool {
	if m.pendingPermission != nil {
		return false
	}
	if m.showMessageSelector {
		return false
	}
	for _, state := range m.toolStates {
		if state.Status == tuiToolQueued || state.Status == tuiToolRunning {
			return true
		}
	}
	return false
}

// advanceSpinnerFrame steps the main spinner animation when visible.
func (m *tuiModel) advanceSpinnerFrame() {
	if !m.shouldAnimateSpinner() {
		return
	}
	if len(m.spinnerFrames) == 0 {
		return
	}
	m.spinnerFrame = (m.spinnerFrame + 1) % len(m.spinnerFrames)
}

// scheduleSpinnerFrameTick schedules the next "thinking" spinner update.
func (m *tuiModel) scheduleSpinnerFrameTick() tea.Cmd {
	if !m.shouldAnimateSpinner() {
		return nil
	}
	return tea.Tick(tuiSpinnerInterval, func(time.Time) tea.Msg {
		return spinnerFrameMsg{}
	})
}

// shouldShowSpinner reports whether the loading spinner should be visible.
func (m *tuiModel) shouldShowSpinner() bool {
	if !m.running || !m.spinnerEnabled {
		return false
	}
	if m.pendingPermission != nil || m.showMessageSelector {
		return false
	}
	// Once assistant text starts streaming, show the message instead of the spinner.
	return m.streamBuffer.Len() == 0
}

// shouldAnimateSpinner reports whether the loading spinner should tick.
func (m *tuiModel) shouldAnimateSpinner() bool {
	return m.shouldShowSpinner()
}

// refreshTools rebuilds the tool viewport content.
func (m *tuiModel) refreshTools() {
	if len(m.toolLines) == 0 {
		m.toolView.SetContent("No tool activity yet.")
		return
	}
	m.toolView.SetContent(strings.Join(m.toolLines, "\n"))
	if m.toolAutoScroll {
		m.toolView.GotoBottom()
	}
}

// renderWelcome builds the welcome banner shown at the start of a session.
func (m *tuiModel) renderWelcome() string {
	accentStyle := lipgloss.NewStyle().Foreground(m.theme.Claude)
	titleStyle := lipgloss.NewStyle().Foreground(m.theme.Text).Bold(true)
	secondaryStyle := lipgloss.NewStyle().Foreground(m.theme.Secondary)

	cwd := mustCwd()
	width := maxInt(tuiLogoMinWidth, len(cwd)+12)
	if m.width > 0 && width > m.width {
		width = m.width
	}

	lines := []string{
		fmt.Sprintf("%s Welcome to %s research preview!", accentStyle.Render("✻"), titleStyle.Render("OpenClaude")),
		"",
		"  " + secondaryStyle.Render("/help for help"),
		"  " + secondaryStyle.Render("cwd: "+cwd),
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Claude).
		PaddingLeft(1).
		PaddingRight(1).
		Width(width)

	return boxStyle.Render(strings.Join(lines, "\n"))
}

// bootstrapHistory seeds the chat view with previous session messages.
func (m *tuiModel) bootstrapHistory() {
	m.chatMessages = nil
	m.toolStates = map[string]tuiToolState{}

	toolNames := map[string]string{}
	for _, message := range m.history {
		if message.Role != "assistant" {
			continue
		}
		for _, call := range message.ToolCalls {
			if call.ID != "" {
				toolNames[call.ID] = call.Function.Name
			}
		}
	}

	for _, message := range m.history {
		switch message.Role {
		case "system":
			continue
		case "user":
			m.appendUserMessageFromHistory(message)
		case "assistant":
			m.appendAssistantMessageFromHistory(message)
		case "tool":
			m.appendToolResultFromHistory(message, toolNames)
		default:
		}
	}
	m.refreshChat()
}

// persistRun appends new session messages and events to storage.
func (m *tuiModel) persistRun(result *agent.RunResult) {
	previousLen := len(m.history)
	newMessages := result.Messages
	if previousLen > 0 && len(result.Messages) >= previousLen {
		newMessages = result.Messages[previousLen:]
	}
	if err := persistSession(m.store, m.sessionID, newMessages, result.Events); err != nil {
		m.statusText = err.Error()
	}
	_ = m.store.SaveLastSession(session.ProjectHash(mustCwd()), m.sessionID)
}

// applyWindowSize recalculates the layout for a new window size.
func (m *tuiModel) applyWindowSize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height
	m.updateLayout()
	m.refreshChat()
}

// updateLayout recalculates viewport sizing based on visible UI sections.
func (m *tuiModel) updateLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	// Match Claude Code's prompt input width heuristic (columns - 6).
	inputWidth := maxInt(20, m.width-6)
	m.input.SetWidth(inputWidth)

	occupied := 0
	if m.shouldShowSpinner() {
		// Spinner uses a one-line row plus a blank line margin.
		occupied += 2
	}
	if m.pendingPermission != nil {
		occupied += lipgloss.Height(m.renderPermissionRequest())
	}
	if m.showMessageSelector {
		occupied += lipgloss.Height(m.renderMessageSelector())
	}
	if m.shouldShowInput() {
		occupied += lipgloss.Height(m.renderInput())
	}

	chatHeight := m.height - occupied
	if chatHeight < 4 {
		chatHeight = 4
	}
	m.chatView.Width = maxInt(20, m.width)
	m.chatView.Height = chatHeight
}

// renderHeader builds the top status line.
func (m *tuiModel) renderHeader() string {
	style := lipgloss.NewStyle().Bold(true)
	header := fmt.Sprintf("OpenClaude | session %s | model %s", m.sessionID, m.model)
	if m.running {
		header = header + " | running"
	}
	return style.Render(padRight(header, m.width))
}

// renderBody composes the chat and tool panes.
func (m *tuiModel) renderBody() string {
	return m.chatView.View()
}

// shouldShowInput reports whether the prompt input should be visible.
func (m *tuiModel) shouldShowInput() bool {
	if m.showMessageSelector {
		return false
	}
	if m.pendingPermission != nil {
		return false
	}
	return true
}

// renderSpinner renders the Claude Code-style "thinking" spinner row.
func (m *tuiModel) renderSpinner() string {
	if !m.shouldShowSpinner() {
		return ""
	}

	frame := "·"
	if len(m.spinnerFrames) > 0 {
		frame = m.spinnerFrames[m.spinnerFrame%len(m.spinnerFrames)]
	}
	message := m.spinnerMessage
	if message == "" {
		message = "Thinking"
	}
	elapsed := 0
	if !m.spinnerStarted.IsZero() {
		elapsed = int(time.Since(m.spinnerStarted).Seconds())
	}

	frameText := lipgloss.NewStyle().Foreground(m.theme.Claude).Render(frame)
	messageText := lipgloss.NewStyle().Foreground(m.theme.Claude).Render(message + "… ")
	escText := lipgloss.NewStyle().Foreground(m.theme.Secondary).Bold(true).Render("esc")
	metaText := lipgloss.NewStyle().Foreground(m.theme.Secondary).Render(
		fmt.Sprintf("(%ds · %s to interrupt)", elapsed, escText),
	)

	line := fmt.Sprintf("%s %s%s", frameText, messageText, metaText)
	return lipgloss.NewStyle().MarginTop(1).Render(line)
}

// openMessageSelector prepares and displays the message selector overlay.
func (m *tuiModel) openMessageSelector() {
	m.showMessageSelector = true
	m.selectorItems = m.buildSelectorItems()
	if len(m.selectorItems) == 0 {
		m.selectorItems = []tuiSelectorItem{{
			Index:     len(m.history),
			Preview:   "(current)",
			Input:     "",
			Mode:      tuiInputPrompt,
			IsCurrent: true,
		}}
	}
	m.selectorIndex = len(m.selectorItems) - 1
	m.input.Blur()
}

// closeMessageSelector hides the selector overlay and restores input focus.
func (m *tuiModel) closeMessageSelector() {
	m.showMessageSelector = false
	m.selectorItems = nil
	m.selectorIndex = 0
	m.input.Focus()
}

// renderMessageSelector draws the selector overlay for history forking.
func (m *tuiModel) renderMessageSelector() string {
	if len(m.selectorItems) == 0 {
		return "No messages to select."
	}
	visibleCount := len(m.selectorItems)
	if visibleCount > tuiSlashSuggestionLimit {
		visibleCount = tuiSlashSuggestionLimit
	}
	halfWindow := visibleCount / 2
	startIndex := m.selectorIndex - halfWindow
	if startIndex < 0 {
		startIndex = 0
	}
	if startIndex+visibleCount > len(m.selectorItems) {
		// Clamp the window to the end of the list when near the bottom.
		startIndex = len(m.selectorItems) - visibleCount
		if startIndex < 0 {
			startIndex = 0
		}
	}

	lines := []string{
		lipgloss.NewStyle().Bold(true).Render("Jump to a previous message"),
		lipgloss.NewStyle().Foreground(m.theme.Secondary).Render("This will fork the conversation"),
		"",
	}
	for offset := 0; offset < visibleCount; offset++ {
		index := startIndex + offset
		item := m.selectorItems[index]
		number := fmt.Sprintf("%2d", index+1)
		label := item.Preview
		if item.IsCurrent {
			label = "(current)"
		}
		if label == "" {
			label = "(empty message)"
		}
		line := fmt.Sprintf("%s %s", number, label)
		if index == m.selectorIndex {
			line = lipgloss.NewStyle().Foreground(m.theme.Suggestion).Bold(true).Render("> " + line)
		} else {
			line = lipgloss.NewStyle().Foreground(m.theme.Secondary).Render("  " + line)
		}
		lines = append(lines, line)
	}

	boxStyle := lipgloss.NewStyle().Border(m.border()).Padding(0, 1)
	boxWidth := maxInt(20, m.width-2)
	box := boxStyle.Width(boxWidth).Render(strings.Join(lines, "\n"))
	hint := m.renderInputHintLine("↑/↓ to select · Enter to confirm · Tab/Esc to cancel")
	return lipgloss.JoinVertical(lipgloss.Left, box, hint)
}

// handleSelectorKey processes navigation/selection keys for the selector.
func (m *tuiModel) handleSelectorKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "tab":
		m.closeMessageSelector()
		return m, nil
	case "up":
		if m.selectorIndex > 0 {
			m.selectorIndex--
		}
		return m, nil
	case "down":
		if m.selectorIndex < len(m.selectorItems)-1 {
			m.selectorIndex++
		}
		return m, nil
	case "home":
		m.selectorIndex = 0
		return m, nil
	case "end":
		m.selectorIndex = len(m.selectorItems) - 1
		return m, nil
	case "enter":
		m.applySelectorSelection()
		return m, nil
	default:
	}

	if key.Type == tea.KeyRunes && len(key.Runes) == 1 {
		runeValue := key.Runes[0]
		if runeValue >= '1' && runeValue <= '9' {
			index := int(runeValue - '1')
			if index >= 0 && index < len(m.selectorItems) {
				m.selectorIndex = index
				m.applySelectorSelection()
				return m, nil
			}
		}
	}

	return m, nil
}

// applySelectorSelection forks the conversation at the selected message.
func (m *tuiModel) applySelectorSelection() {
	if len(m.selectorItems) == 0 || m.selectorIndex < 0 || m.selectorIndex >= len(m.selectorItems) {
		m.closeMessageSelector()
		return
	}
	selected := m.selectorItems[m.selectorIndex]
	if selected.IsCurrent {
		m.closeMessageSelector()
		return
	}

	if m.running {
		// Abort any in-flight request before forking the conversation.
		m.cancelRun("Cancelled.")
		m.statusText = ""
	}
	// Reset transient state so the forked conversation is clean.
	m.pendingPermission = nil
	m.streamBuffer.Reset()
	m.toolLines = nil
	m.toolView.SetContent("No tool activity yet.")
	if selected.Index >= 0 && selected.Index <= len(m.history) {
		// Exclude the selected message so it can be edited and re-sent.
		m.history = m.history[:selected.Index]
	}

	m.bootstrapHistory()
	m.input.SetValue(selected.Input)
	m.input.CursorEnd()
	m.setInputMode(selected.Mode)
	m.syncInputState()
	m.closeMessageSelector()
}

// buildSelectorItems constructs the selector list from user messages.
func (m *tuiModel) buildSelectorItems() []tuiSelectorItem {
	items := make([]tuiSelectorItem, 0, len(m.history)+1)
	for index, message := range m.history {
		if message.Role != "user" {
			continue
		}
		// Only user messages are eligible for conversation forks.
		preview, inputValue, mode := selectorTextForMessage(message)
		preview = truncateForDisplay(compactWhitespace(preview), 120)
		items = append(items, tuiSelectorItem{
			Index:   index,
			Preview: preview,
			Input:   inputValue,
			Mode:    mode,
		})
	}
	items = append(items, tuiSelectorItem{
		Index:     len(m.history),
		Preview:   "(current)",
		Input:     "",
		Mode:      tuiInputPrompt,
		IsCurrent: true,
	})
	return items
}

// selectorTextForMessage extracts the selector preview and input restore values.
func selectorTextForMessage(message openai.Message) (string, string, tuiInputMode) {
	rawText := extractMessageText(message)
	if rawText == "" && message.Content != nil {
		rawText = formatContent(message.Content)
	}
	if rawText == "" {
		return "", "", tuiInputPrompt
	}
	// Prefer explicit tags to reconstruct bash and command inputs.
	if bashInput := extractTag(rawText, "bash-input"); bashInput != "" {
		return "!" + bashInput, bashInput, tuiInputBash
	}
	if command := extractTag(rawText, "command-message"); command != "" {
		args := strings.TrimSpace(extractTag(rawText, "command-args"))
		line := "/" + command
		if args != "" {
			line = line + " " + args
		}
		return line, line, tuiInputPrompt
	}
	trimmed := strings.TrimSpace(rawText)
	if strings.HasPrefix(trimmed, "/") {
		return trimmed, trimmed, tuiInputPrompt
	}
	return trimmed, trimmed, tuiInputPrompt
}

// setActivePane updates focus and input state for the requested pane.
func (m *tuiModel) setActivePane(pane string) {
	switch pane {
	case "chat", "tools":
		m.activePane = pane
		m.input.Blur()
	default:
		m.activePane = "input"
		m.input.Focus()
	}
}

// cyclePane moves focus between input, chat, and tools.
func (m *tuiModel) cyclePane(delta int) {
	order := []string{"input", "chat", "tools"}
	index := 0
	for i, name := range order {
		if name == m.activePane {
			index = i
			break
		}
	}
	next := (index + delta) % len(order)
	if next < 0 {
		next += len(order)
	}
	m.setActivePane(order[next])
}

// scrollActivePane scrolls the currently focused pane.
func (m *tuiModel) scrollActivePane(delta int) {
	switch m.activePane {
	case "tools":
		m.toolAutoScroll = false
		if delta > 0 {
			m.toolView.LineDown(delta)
		} else {
			m.toolView.LineUp(-delta)
		}
	case "chat":
		m.chatAutoScroll = false
		if delta > 0 {
			m.chatView.LineDown(delta)
		} else {
			m.chatView.LineUp(-delta)
		}
	}
}

// gotoActivePaneTop moves the active pane to the top.
func (m *tuiModel) gotoActivePaneTop() {
	switch m.activePane {
	case "tools":
		m.toolView.GotoTop()
		m.toolAutoScroll = false
	case "chat":
		m.chatView.GotoTop()
		m.chatAutoScroll = false
	}
}

// gotoActivePaneBottom moves the active pane to the bottom.
func (m *tuiModel) gotoActivePaneBottom() {
	switch m.activePane {
	case "tools":
		m.toolView.GotoBottom()
		m.toolAutoScroll = true
	case "chat":
		m.chatView.GotoBottom()
		m.chatAutoScroll = true
	}
}

// renderInput returns the input box rendering.
func (m *tuiModel) renderInput() string {
	if !m.shouldShowInput() {
		return ""
	}

	borderColor := m.theme.SecondaryBorder
	if m.inputMode == tuiInputBash {
		borderColor = m.theme.Bash
	}

	promptSymbol := ">"
	promptColor := lipgloss.AdaptiveColor{}
	if m.inputMode == tuiInputBash {
		promptSymbol = "!"
		promptColor = m.theme.Bash
	} else if m.running {
		promptColor = m.theme.Secondary
	}

	promptStyle := lipgloss.NewStyle().Width(3)
	if promptColor != (lipgloss.AdaptiveColor{}) {
		promptStyle = promptStyle.Foreground(promptColor)
	}
	promptCell := promptStyle.Render(" " + promptSymbol + " ")

	inputRow := lipgloss.JoinHorizontal(lipgloss.Top, promptCell, m.input.View())
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		MarginTop(1).
		PaddingRight(1)
	if m.width > 0 {
		boxStyle = boxStyle.Width(m.width)
	}
	inputBox := boxStyle.Render(inputRow)

	footer := m.renderInputFooter()
	if footer == "" {
		return inputBox
	}
	return lipgloss.JoinVertical(lipgloss.Left, inputBox, footer)
}

// renderStatus returns the bottom status line.
func (m *tuiModel) renderStatus() string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	text := m.statusText
	if text == "" {
		text = "Ready"
	}
	info := m.renderStatusInfo()
	if info != "" {
		text = fmt.Sprintf("%s | %s", text, info)
	}
	return style.Render(padRight(text, m.width))
}

// renderStatusInfo assembles auxiliary status information.
func (m *tuiModel) renderStatusInfo() string {
	parts := []string{}
	if m.permissionMode != "" {
		parts = append(parts, fmt.Sprintf("perm:%s", m.permissionMode))
	}
	if m.planMode {
		parts = append(parts, "plan:on")
	} else {
		parts = append(parts, "plan:off")
	}
	if m.inputMode == tuiInputBash {
		parts = append(parts, "mode:bash")
	}
	if m.activePane != "" {
		parts = append(parts, fmt.Sprintf("focus:%s", m.activePane))
	}
	if m.lastUsage.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("tokens:%d", m.lastUsage.TotalTokens))
	}
	if m.totalCost > 0 {
		parts = append(parts, fmt.Sprintf("cost:$%.4f", m.totalCost))
	}
	return strings.Join(parts, " ")
}

// refreshPlanMode syncs the plan-only indicator from the session store.
func (m *tuiModel) refreshPlanMode() {
	if m.store == nil || m.sessionID == "" {
		m.planMode = false
		return
	}
	m.planMode = tools.IsPlanMode(m.store, m.sessionID)
}

// renderPane formats a bordered pane with a title.
func (m *tuiModel) renderPane(title string, content string, width int) string {
	style := lipgloss.NewStyle().Border(m.border()).Padding(0, 1)
	header := fmt.Sprintf("[%s]", title)
	pane := lipgloss.JoinVertical(lipgloss.Left, header, content)
	return style.Width(width).Render(pane)
}

// renderMessage formats a chat message for display.
func (m *tuiModel) renderMessage(message tuiMessage, streaming bool) string {
	switch message.Kind {
	case tuiMessageUserPrompt:
		return m.renderUserPromptMessage(message)
	case tuiMessageUserBash:
		return m.renderUserBashMessage(message)
	case tuiMessageUserCommand:
		return m.renderUserCommandMessage(message)
	case tuiMessageAssistantToolUse:
		return m.renderAssistantToolUseMessage(message)
	case tuiMessageToolResult:
		return m.renderToolResultMessage(message)
	case tuiMessageAssistantThinking:
		return m.renderAssistantThinkingMessage(message)
	case tuiMessageSystem:
		return m.renderSystemMessage(message)
	case tuiMessageAssistantText:
		return m.renderAssistantTextMessage(message, streaming)
	default:
		return m.renderFallbackMessage(message, streaming)
	}
}

// renderUserPromptMessage draws a standard user prompt entry.
func (m *tuiModel) renderUserPromptMessage(message tuiMessage) string {
	prefixStyle := lipgloss.NewStyle().Foreground(m.theme.Secondary)
	textStyle := lipgloss.NewStyle().Foreground(m.theme.Secondary)
	prefix := prefixStyle.Render(">")
	content := indentMultiline(message.Content, "  ")
	return fmt.Sprintf("%s %s", prefix, textStyle.Render(content))
}

// renderUserBashMessage draws a user bash invocation entry.
func (m *tuiModel) renderUserBashMessage(message tuiMessage) string {
	prefixStyle := lipgloss.NewStyle().Foreground(m.theme.Bash)
	textStyle := lipgloss.NewStyle().Foreground(m.theme.Secondary)
	prefix := prefixStyle.Render("!")
	content := indentMultiline(message.Content, "  ")
	return fmt.Sprintf("%s %s", prefix, textStyle.Render(content))
}

// renderUserCommandMessage draws a user slash command entry.
func (m *tuiModel) renderUserCommandMessage(message tuiMessage) string {
	prefixStyle := lipgloss.NewStyle().Foreground(m.theme.Secondary)
	textStyle := lipgloss.NewStyle().Foreground(m.theme.Secondary)
	commandText := formatCommandDisplay(message.Content)
	content := indentMultiline(commandText, "  ")
	return fmt.Sprintf("%s %s", prefixStyle.Render(">"), textStyle.Render(content))
}

// renderAssistantTextMessage renders assistant content, including special tags.
func (m *tuiModel) renderAssistantTextMessage(message tuiMessage, streaming bool) string {
	content := strings.TrimSpace(message.Content)
	if content == "" {
		return ""
	}
	if rendered, handled := m.renderAssistantSpecial(content, streaming); handled {
		return rendered
	}
	if !streaming {
		content = m.renderMarkdown(content)
	}
	indent := ""
	if message.ShowDot {
		indent = "  "
	}
	content = indentMultiline(content, indent)
	if !message.ShowDot {
		return content
	}
	prefix := lipgloss.NewStyle().Foreground(m.theme.Text).Render(assistantDot())
	return prefix + " " + content
}

// renderAssistantToolUseMessage renders tool-use announcements.
func (m *tuiModel) renderAssistantToolUseMessage(message tuiMessage) string {
	color := m.theme.Text
	isUnresolved := message.ToolStatus == tuiToolQueued || message.ToolStatus == tuiToolRunning
	switch message.ToolStatus {
	case tuiToolQueued, tuiToolRunning:
		color = m.theme.Secondary
	case tuiToolCompleted:
		color = m.theme.Success
	case tuiToolFailed:
		color = m.theme.Error
	}

	indicatorText := ""
	if message.ShowDot {
		indicator := assistantDot()
		if isUnresolved && m.shouldAnimateTools() && !m.spinnerOn {
			indicator = "  "
		}
		indicatorText = lipgloss.NewStyle().Foreground(color).Render(indicator) + " "
	}
	nameText := lipgloss.NewStyle().Foreground(color).Bold(message.ToolStatus != tuiToolQueued).Render(message.ToolName)
	args := ""
	if message.ToolArgs != "" {
		args = fmt.Sprintf("(%s)", message.ToolArgs)
	}
	if args != "" {
		args = " " + lipgloss.NewStyle().Foreground(color).Render(args)
	}
	return fmt.Sprintf("%s%s%s…", indicatorText, nameText, args)
}

// renderToolResultMessage renders tool result output lines.
func (m *tuiModel) renderToolResultMessage(message tuiMessage) string {
	content := strings.TrimSpace(message.Content)
	if content == "" {
		content = "(No content)"
	}
	content = truncateOutputLines(content, tuiMaxRenderedLines)
	return m.renderIndentedResultLine(content, message.ToolError)
}

// renderAssistantThinkingMessage renders a "thinking" block.
func (m *tuiModel) renderAssistantThinkingMessage(message tuiMessage) string {
	style := lipgloss.NewStyle().Foreground(m.theme.Secondary).Italic(true)
	heading := style.Render("✻ Thinking…")
	content := strings.TrimSpace(message.Content)
	if content == "" {
		return heading
	}
	body := style.Render(indentMultiline(content, "  "))
	return strings.Join([]string{heading, body}, "\n")
}

// renderSystemMessage renders system messages and stub responses.
func (m *tuiModel) renderSystemMessage(message tuiMessage) string {
	style := lipgloss.NewStyle().Foreground(m.theme.Warning)
	content := indentMultiline(message.Content, "  ")
	return style.Render(content)
}

// renderFallbackMessage preserves the legacy rendering for unknown kinds.
func (m *tuiModel) renderFallbackMessage(message tuiMessage, streaming bool) string {
	label := strings.ToUpper(message.Role)
	content := message.Content
	style := lipgloss.NewStyle().Foreground(m.theme.Secondary)
	switch message.Role {
	case "user":
		style = style.Foreground(m.theme.Secondary).Bold(true)
		label = "YOU"
	case "assistant":
		style = style.Foreground(m.theme.Text).Bold(true)
		label = "ASSISTANT"
	case "tool":
		style = style.Foreground(m.theme.Secondary)
		label = "TOOL"
	case "system":
		style = style.Foreground(m.theme.Warning)
		label = "SYSTEM"
	}
	if !streaming && message.Role != "user" {
		content = m.renderMarkdown(content)
	}
	return fmt.Sprintf("%s\n%s", style.Render(label+":"), content)
}

// renderAssistantSpecial handles tagged assistant payloads.
func (m *tuiModel) renderAssistantSpecial(content string, streaming bool) (string, bool) {
	if strings.HasPrefix(content, "<bash-stdout") || strings.HasPrefix(content, "<bash-stderr") {
		return m.renderBashOutputMessage(content), true
	}
	if strings.HasPrefix(content, "<local-command-stdout") || strings.HasPrefix(content, "<local-command-stderr") {
		return m.renderLocalCommandOutputMessage(content), true
	}
	if strings.HasPrefix(content, tuiAPIErrorPrefix) {
		message := content
		if content == tuiAPIErrorPrefix {
			message = tuiAPIErrorPrefix + ": Please wait a moment and try again."
		}
		return m.renderIndentedResultLine(message, true), true
	}
	switch content {
	case tuiNoResponseRequested, tuiInterruptForToolMessage:
		// These synthetic placeholders should be suppressed in the UI.
		return "", true
	case tuiInterruptMessage, tuiCancelMessage:
		return m.renderIndentedResultLine("Interrupted by user", true), true
	case tuiRejectMessage:
		return m.renderIndentedResultLine("Tool use rejected by user", true), true
	case tuiPromptTooLongMessage:
		return m.renderIndentedResultLine("Context low · Run /compact to compact & continue", true), true
	case tuiCreditTooLowMessage:
		return m.renderIndentedResultLine(
			"Credit balance too low · Add funds: https://console.anthropic.com/settings/billing",
			true,
		), true
	case tuiInvalidAPIKeyMessage:
		return m.renderIndentedResultLine(tuiInvalidAPIKeyMessage, true), true
	default:
	}
	_ = streaming
	return "", false
}

// renderBashOutputMessage renders bash stdout and stderr blocks.
func (m *tuiModel) renderBashOutputMessage(content string) string {
	stdout := extractTag(content, "bash-stdout")
	stderr := extractTag(content, "bash-stderr")
	lines := []string{}
	if stdout != "" {
		lines = append(lines, m.renderIndentedResultLine(truncateOutputLines(stdout, tuiMaxRenderedLines), false))
	}
	if stderr != "" {
		lines = append(lines, m.renderIndentedResultLine(truncateOutputLines(stderr, tuiMaxRenderedLines), true))
	}
	if len(lines) == 0 {
		lines = append(lines, m.renderIndentedResultLine("(No content)", false))
	}
	return strings.Join(lines, "\n")
}

// renderLocalCommandOutputMessage renders local command stdout/stderr blocks.
func (m *tuiModel) renderLocalCommandOutputMessage(content string) string {
	stdout := extractTag(content, "local-command-stdout")
	stderr := extractTag(content, "local-command-stderr")
	lines := []string{}
	if stdout != "" {
		lines = append(lines, m.renderIndentedResultLine(truncateOutputLines(stdout, tuiMaxRenderedLines), false))
	}
	if stderr != "" {
		lines = append(lines, m.renderIndentedResultLine(truncateOutputLines(stderr, tuiMaxRenderedLines), true))
	}
	if len(lines) == 0 {
		lines = append(lines, m.renderIndentedResultLine("(No content)", false))
	}
	return strings.Join(lines, "\n")
}

// renderIndentedResultLine prefixes tool-like output with the ⎿ marker.
func (m *tuiModel) renderIndentedResultLine(content string, isError bool) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		trimmed = "(No content)"
	}
	prefix := "  ⎿ "
	indent := strings.Repeat(" ", len(prefix))
	lineColor := m.theme.Text
	if isError {
		lineColor = m.theme.Error
	} else if trimmed == "(No content)" {
		lineColor = m.theme.Secondary
	}
	style := lipgloss.NewStyle().Foreground(lineColor)
	rendered := indentMultiline(trimmed, indent)
	return prefix + style.Render(rendered)
}

// indentMultiline indents all lines after the first with the given prefix.
func indentMultiline(text string, indent string) string {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return ""
	}
	for index := 1; index < len(lines); index++ {
		lines[index] = indent + lines[index]
	}
	return strings.Join(lines, "\n")
}

// formatCommandDisplay normalizes command tags into a readable slash command line.
func formatCommandDisplay(content string) string {
	if content == "" {
		return ""
	}
	command := extractTag(content, "command-message")
	if command == "" {
		return strings.TrimSpace(content)
	}
	args := strings.TrimSpace(extractTag(content, "command-args"))
	if args == "" {
		return "/" + command
	}
	return "/" + command + " " + args
}

// extractTag returns the inner text of a simple XML-like tag.
func extractTag(content string, tag string) string {
	if content == "" || tag == "" {
		return ""
	}
	openTag := "<" + tag
	openIndex := strings.Index(content, openTag)
	if openIndex == -1 {
		return ""
	}
	closeOpen := strings.Index(content[openIndex:], ">")
	if closeOpen == -1 {
		return ""
	}
	start := openIndex + closeOpen + 1
	closeTag := "</" + tag + ">"
	endIndex := strings.Index(content[start:], closeTag)
	if endIndex == -1 {
		return ""
	}
	return content[start : start+endIndex]
}

// truncateOutputLines limits output to a readable number of lines.
func truncateOutputLines(content string, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if maxLines <= 0 || len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	// Keep head and tail slices to preserve context around long outputs.
	headCount := maxLines / 2
	tailCount := maxLines - headCount
	omitted := len(lines) - maxLines
	truncated := append([]string{}, lines[:headCount]...)
	truncated = append(truncated, fmt.Sprintf("... (+%d lines)", omitted))
	truncated = append(truncated, lines[len(lines)-tailCount:]...)
	return strings.Join(truncated, "\n")
}

// renderMarkdown converts markdown into terminal-friendly output when possible.
func (m *tuiModel) renderMarkdown(content string) string {
	if m.markdownRenderer == nil {
		return content
	}
	rendered, err := m.markdownRenderer.Render(content)
	if err != nil {
		return content
	}
	return rendered
}

// border defines a simple ASCII border to avoid Unicode dependencies.
func (m *tuiModel) border() lipgloss.Border {
	return lipgloss.Border{
		Top:         "-",
		Bottom:      "-",
		Left:        "|",
		Right:       "|",
		TopLeft:     "+",
		TopRight:    "+",
		BottomLeft:  "+",
		BottomRight: "+",
	}
}

// defaultTUITheme defines the baseline adaptive colors for the TUI.
func defaultTUITheme() tuiTheme {
	return tuiTheme{
		Text:       lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"},
		Secondary:  lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"},
		Bash:       lipgloss.AdaptiveColor{Light: "#ff0087", Dark: "#fd5db1"},
		Error:      lipgloss.AdaptiveColor{Light: "#ab2b3f", Dark: "#ff6b80"},
		Success:    lipgloss.AdaptiveColor{Light: "#2c7a39", Dark: "#4eba65"},
		Warning:    lipgloss.AdaptiveColor{Light: "#966c1e", Dark: "#ffc107"},
		Suggestion: lipgloss.AdaptiveColor{Light: "#5769f7", Dark: "#b1b9f9"},
	}
}

// setInputMode switches the input between prompt and bash modes.
func (m *tuiModel) setInputMode(mode tuiInputMode) {
	if m.inputMode == mode {
		return
	}
	m.inputMode = mode
	m.syncInputPrompt()
	m.clearInputHints()
	m.updateSlashSuggestions(m.input.Value())
}

// syncInputPrompt updates the prompt label and placeholder for the active input mode.
func (m *tuiModel) syncInputPrompt() {
	// Claude Code renders the prompt glyph separately, so we keep the textarea prompt empty.
	m.input.Prompt = ""

	placeholderText := ""
	if m.submitCount == 0 {
		// Claude Code only shows the suggestion placeholder before the first submit.
		placeholderText = `Try "/help"`
	}
	m.input.Placeholder = placeholderText

	m.input.FocusedStyle.Prompt = lipgloss.NewStyle()
	m.input.BlurredStyle.Prompt = lipgloss.NewStyle()
	m.input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(m.theme.Text)
	m.input.BlurredStyle.Text = lipgloss.NewStyle().Foreground(m.theme.Secondary)
	m.input.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(m.theme.Secondary)
	m.input.BlurredStyle.Placeholder = lipgloss.NewStyle().Foreground(m.theme.Secondary)

	// Textarea width must track the terminal width for wrapping to feel natural.
	inputWidth := 20
	if m.width > 0 {
		inputWidth = maxInt(20, m.width-6)
	}
	m.input.SetWidth(inputWidth)
}

// syncInputState recomputes mode, suggestions, and paste placeholders after edits.
func (m *tuiModel) syncInputState() {
	inputValue := m.input.Value()
	m.syncPendingPaste(inputValue)
	if m.pendingPaste == nil {
		m.clearInputHints()
	}

	if m.inputMode == tuiInputPrompt {
		adjustedValue, switched := stripBashPrefix(inputValue)
		if switched {
			m.setInputMode(tuiInputBash)
			m.input.SetValue(adjustedValue)
			m.input.CursorEnd()
			inputValue = adjustedValue
		}
	}

	m.updateSlashSuggestions(inputValue)
}

// syncPendingPaste clears stale paste state when the placeholder is removed.
func (m *tuiModel) syncPendingPaste(inputValue string) {
	if m.pendingPaste == nil {
		return
	}
	if strings.Contains(inputValue, m.pendingPaste.Placeholder) {
		return
	}
	m.pendingPaste = nil
}

// stripBashPrefix trims a leading "!" prefix and reports whether a mode switch should occur.
func stripBashPrefix(inputValue string) (string, bool) {
	firstNonSpaceIndex := strings.IndexFunc(inputValue, func(runeValue rune) bool {
		return runeValue != ' ' && runeValue != '\t'
	})
	if firstNonSpaceIndex == -1 {
		return inputValue, false
	}
	if inputValue[firstNonSpaceIndex] != '!' {
		return inputValue, false
	}
	remaining := strings.TrimLeft(inputValue[firstNonSpaceIndex+1:], " \t")
	adjusted := inputValue[:firstNonSpaceIndex] + remaining
	return adjusted, true
}

// clearInputHints resets transient hints under the input box.
func (m *tuiModel) clearInputHints() {
	m.inputHint = ""
	m.doublePress = tuiDoublePress{}
}

// handleHistoryArrows maps up/down arrows to input history when appropriate.
func (m *tuiModel) handleHistoryArrows(key tea.KeyMsg) bool {
	if m.activePane != "input" {
		return false
	}
	if len(m.slashSuggestions) > 1 {
		return false
	}
	if strings.Contains(m.input.Value(), "\n") {
		return false
	}
	switch key.String() {
	case "up":
		m.cycleInputHistory(-1)
		return true
	case "down":
		m.cycleInputHistory(1)
		return true
	default:
		return false
	}
}

// shouldInsertContinuationNewline inserts a newline when a trailing continuation is detected.
func (m *tuiModel) shouldInsertContinuationNewline() bool {
	inputValue := m.input.Value()
	trimmedValue := strings.TrimRight(inputValue, " \t")
	if strings.HasSuffix(trimmedValue, "\\") {
		adjusted := strings.TrimSuffix(trimmedValue, "\\")
		if adjusted != inputValue {
			m.input.SetValue(adjusted)
			m.input.CursorEnd()
		}
		m.input.InsertString("\n")
		m.syncInputState()
		return true
	}
	return false
}

// handleSuggestionKey processes navigation and accept actions for slash suggestions.
func (m *tuiModel) handleSuggestionKey(key tea.KeyMsg) (bool, tea.Cmd) {
	if len(m.slashSuggestions) == 0 {
		return false, nil
	}
	switch key.String() {
	case "up", "ctrl+p", "shift+tab":
		m.moveSlashSelection(-1)
		return true, nil
	case "down", "ctrl+n":
		m.moveSlashSelection(1)
		return true, nil
	case "tab":
		if m.slashSelection < 0 {
			m.slashSelection = 0
		}
		m.applySlashSuggestion()
		return true, nil
	case "enter":
		if m.slashSelection < 0 {
			m.slashSelection = 0
		}
		selected := m.selectedSlashSuggestion()
		if selected == nil {
			return true, nil
		}
		m.applySlashSuggestion()
		if !selected.AcceptsArgs {
			_, cmd := m.submitInput()
			return true, cmd
		}
		return true, nil
	case "esc":
		m.clearSlashSuggestions()
		return true, nil
	default:
		return false, nil
	}
}

// moveSlashSelection advances the current suggestion index, wrapping as needed.
func (m *tuiModel) moveSlashSelection(delta int) {
	if len(m.slashSuggestions) == 0 {
		m.slashSelection = -1
		return
	}
	if m.slashSelection < 0 {
		m.slashSelection = 0
	}
	nextIndex := m.slashSelection + delta
	if nextIndex < 0 {
		nextIndex = len(m.slashSuggestions) - 1
	}
	if nextIndex >= len(m.slashSuggestions) {
		nextIndex = 0
	}
	m.slashSelection = nextIndex
}

// applySlashSuggestion fills the input with the selected slash command.
func (m *tuiModel) applySlashSuggestion() {
	if m.slashSelection < 0 || m.slashSelection >= len(m.slashSuggestions) {
		return
	}
	selected := m.slashSuggestions[m.slashSelection]
	commandText := "/" + selected.Name
	commandText += " "
	m.input.SetValue(commandText)
	m.input.CursorEnd()
	m.clearSlashSuggestions()
	m.syncInputState()
}

// selectedSlashSuggestion returns the currently highlighted suggestion.
func (m *tuiModel) selectedSlashSuggestion() *tuiSlashSuggestion {
	if m.slashSelection < 0 || m.slashSelection >= len(m.slashSuggestions) {
		return nil
	}
	return &m.slashSuggestions[m.slashSelection]
}

// clearSlashSuggestions resets the active suggestion list and selection.
func (m *tuiModel) clearSlashSuggestions() {
	m.slashSuggestions = nil
	m.slashSelection = -1
}

// updateSlashSuggestions refreshes suggestions based on the current input value.
func (m *tuiModel) updateSlashSuggestions(inputValue string) {
	if m.opts != nil && m.opts.DisableSlashCommands {
		m.clearSlashSuggestions()
		return
	}
	if m.inputMode != tuiInputPrompt {
		m.clearSlashSuggestions()
		return
	}
	query, hasArgs := parseSlashInput(inputValue)
	if hasArgs {
		m.clearSlashSuggestions()
		return
	}
	if query == "" && !strings.HasPrefix(strings.TrimSpace(inputValue), "/") {
		m.clearSlashSuggestions()
		return
	}

	allSuggestions := buildSlashSuggestions()
	filtered := filterSlashSuggestions(allSuggestions, query)
	if len(filtered) == 0 {
		m.clearSlashSuggestions()
		return
	}
	previous := m.selectedSlashSuggestion()
	m.slashSuggestions = filtered
	m.slashSelection = 0
	if previous != nil {
		for index, suggestion := range filtered {
			if suggestion.Name == previous.Name {
				m.slashSelection = index
				break
			}
		}
	}
}

// parseSlashInput extracts the slash command prefix and reports if arguments are present.
func parseSlashInput(inputValue string) (string, bool) {
	trimmed := strings.TrimSpace(inputValue)
	if !strings.HasPrefix(trimmed, "/") {
		return "", false
	}
	withoutSlash := strings.TrimPrefix(trimmed, "/")
	if withoutSlash == "" {
		return "", false
	}
	firstSpaceIndex := strings.IndexFunc(withoutSlash, func(runeValue rune) bool {
		return runeValue == ' ' || runeValue == '\t' || runeValue == '\n'
	})
	if firstSpaceIndex == -1 {
		return strings.ToLower(withoutSlash), false
	}
	return strings.ToLower(withoutSlash[:firstSpaceIndex]), true
}

// buildSlashSuggestions constructs the full list of available suggestions.
func buildSlashSuggestions() []tuiSlashSuggestion {
	descriptions := map[string]string{
		"keybindings-help": "Show keybindings.",
		"compact":          "Compact the conversation.",
		"context":          "Manage context.",
		"cost":             "Show token usage and cost.",
		"init":             "Initialize session setup.",
		"pr-comments":      "Review pull request comments.",
		"release-notes":    "Show release notes.",
		"review":           "Review changes.",
		"security-review":  "Run a security review.",
	}
	acceptsArgs := map[string]bool{
		"context":         true,
		"pr-comments":     true,
		"review":          true,
		"security-review": true,
	}
	commands := defaultSlashCommandList()
	suggestions := make([]tuiSlashSuggestion, 0, len(commands))
	for _, commandName := range commands {
		suggestions = append(suggestions, tuiSlashSuggestion{
			Name:        commandName,
			Description: descriptions[commandName],
			Aliases:     nil,
			AcceptsArgs: acceptsArgs[commandName],
		})
	}
	return suggestions
}

// filterSlashSuggestions applies the typed prefix to the available list.
func filterSlashSuggestions(
	suggestions []tuiSlashSuggestion,
	query string,
) []tuiSlashSuggestion {
	if query == "" {
		return suggestions
	}
	filtered := make([]tuiSlashSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		if matchesSlashSuggestion(suggestion, query) {
			filtered = append(filtered, suggestion)
		}
	}
	return filtered
}

// matchesSlashSuggestion tests a suggestion against the lowercase query.
func matchesSlashSuggestion(suggestion tuiSlashSuggestion, query string) bool {
	if strings.HasPrefix(strings.ToLower(suggestion.Name), query) {
		return true
	}
	for _, alias := range suggestion.Aliases {
		if strings.HasPrefix(strings.ToLower(alias), query) {
			return true
		}
	}
	return false
}

// renderInputFooter renders slash suggestions and transient input hints.
func (m *tuiModel) renderInputFooter() string {
	suggestionLines := m.renderSlashSuggestionLines()
	if len(suggestionLines) > 0 {
		footerLines := append([]string{}, suggestionLines...)
		footerLines = append(footerLines, m.renderSuggestionHintLine())
		return strings.Join(footerLines, "\n")
	}
	if m.inputHint != "" {
		return m.renderInputHintLine(m.inputHint)
	}
	return m.renderDefaultHintLine()
}

// renderSlashSuggestionLines builds the formatted suggestion list for the footer.
func (m *tuiModel) renderSlashSuggestionLines() []string {
	if len(m.slashSuggestions) == 0 {
		return nil
	}
	suggestionCount := len(m.slashSuggestions)
	if suggestionCount > tuiSlashSuggestionLimit {
		suggestionCount = tuiSlashSuggestionLimit
	}

	commandWidth := 0
	for index := 0; index < suggestionCount; index++ {
		suggestion := m.slashSuggestions[index]
		commandText := "/" + suggestion.Name
		if len(suggestion.Aliases) > 0 {
			commandText += fmt.Sprintf(" (%s)", strings.Join(suggestion.Aliases, ", "))
		}
		if width := lipgloss.Width(commandText); width > commandWidth {
			commandWidth = width
		}
	}
	commandWidth += 2

	lines := make([]string, 0, suggestionCount)
	selectedStyle := lipgloss.NewStyle().Foreground(m.theme.Suggestion).Bold(true)
	secondaryStyle := lipgloss.NewStyle().Foreground(m.theme.Secondary)
	for suggestionIndex := 0; suggestionIndex < suggestionCount; suggestionIndex++ {
		suggestion := m.slashSuggestions[suggestionIndex]
		commandText := "/" + suggestion.Name
		if len(suggestion.Aliases) > 0 {
			commandText += fmt.Sprintf(" (%s)", strings.Join(suggestion.Aliases, ", "))
		}
		if suggestion.AcceptsArgs {
			commandText += " …"
		}
		lineText := commandText
		if suggestion.Description != "" {
			lineText = fmt.Sprintf("%s%s", padRight(commandText, commandWidth), suggestion.Description)
		}
		if suggestionIndex == m.slashSelection {
			lines = append(lines, selectedStyle.Render(lineText))
			continue
		}
		lines = append(lines, secondaryStyle.Render(lineText))
	}
	return lines
}

// renderInputHintLine formats the transient input hint text.
func (m *tuiModel) renderInputHintLine(hint string) string {
	style := lipgloss.NewStyle().Foreground(m.theme.Secondary)
	return style.Render(hint)
}

// renderSplitHintLine aligns a left and right hint on one line when possible.
func (m *tuiModel) renderSplitHintLine(left string, right string) string {
	if right == "" {
		return m.renderInputHintLine(left)
	}
	width := m.width
	if width <= 0 {
		return m.renderInputHintLine(left + " " + right)
	}
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	spacer := width - leftWidth - rightWidth
	if spacer < 1 {
		return m.renderInputHintLine(left + " " + right)
	}
	return m.renderInputHintLine(left + strings.Repeat(" ", spacer) + right)
}

// renderSuggestionHintLine renders the footer hint while suggestions are visible.
func (m *tuiModel) renderSuggestionHintLine() string {
	return m.renderInputHintLine("↑/↓ to select · Tab/Enter to accept · Esc to cancel")
}

// renderDefaultHintLine renders the default footer hint line.
func (m *tuiModel) renderDefaultHintLine() string {
	left := "! for bash mode · / for commands · esc to undo"
	if m.inputMode == tuiInputBash {
		left = "! bash mode · / for commands · esc to undo"
	}
	return m.renderSplitHintLine(left, "\\⏎ for newline")
}

// inputFooterHeight reports the number of footer lines for layout sizing.
func (m *tuiModel) inputFooterHeight() int {
	if len(m.slashSuggestions) > 0 {
		lineCount := len(m.slashSuggestions)
		if lineCount > tuiSlashSuggestionLimit {
			lineCount = tuiSlashSuggestionLimit
		}
		return lineCount + 1
	}
	return 1
}

// resolvePastedInput swaps any paste placeholder with the buffered content.
func (m *tuiModel) resolvePastedInput(inputValue string) string {
	if m.pendingPaste == nil {
		return inputValue
	}
	if !strings.Contains(inputValue, m.pendingPaste.Placeholder) {
		m.pendingPaste = nil
		return inputValue
	}
	resolved := strings.Replace(inputValue, m.pendingPaste.Placeholder, m.pendingPaste.Content, 1)
	m.pendingPaste = nil
	return resolved
}

// handlePasteKey buffers large paste payloads and schedules finalization.
func (m *tuiModel) handlePasteKey(key tea.KeyMsg, previousValue string) tea.Cmd {
	if !isPasteKey(key) {
		return nil
	}
	pasteChunk := string(key.Runes)
	if pasteChunk == "" {
		return nil
	}
	if !m.pasteBuffer.Active && !shouldBufferPasteChunk(pasteChunk) {
		return nil
	}
	if !m.pasteBuffer.Active {
		m.pasteBuffer = tuiPasteBuffer{
			Chunks:    nil,
			BaseValue: previousValue,
			Active:    true,
			Last:      time.Now(),
		}
	}
	m.pasteBuffer.Chunks = append(m.pasteBuffer.Chunks, pasteChunk)
	m.pasteBuffer.Last = time.Now()
	return m.schedulePasteFinalize()
}

// isPasteKey identifies key events that represent pasted text.
func isPasteKey(key tea.KeyMsg) bool {
	if key.Paste {
		return true
	}
	if key.Type != tea.KeyRunes {
		return false
	}
	return len(key.Runes) > 1
}

// shouldBufferPasteChunk decides whether the chunk is large enough to buffer.
func shouldBufferPasteChunk(chunk string) bool {
	if strings.Count(chunk, "\n")+1 >= tuiPasteLineThreshold {
		return true
	}
	return len([]rune(chunk)) >= tuiPasteRuneThreshold
}

// schedulePasteFinalize emits a delayed message to finalize paste buffering.
func (m *tuiModel) schedulePasteFinalize() tea.Cmd {
	return tea.Tick(tuiPasteIdleDelay, func(time.Time) tea.Msg {
		return pasteDoneMsg{}
	})
}

// finalizePaste replaces a buffered paste with a placeholder for safer rendering.
func (m *tuiModel) finalizePaste() tea.Cmd {
	if !m.pasteBuffer.Active {
		return nil
	}
	if time.Since(m.pasteBuffer.Last) < tuiPasteIdleDelay {
		return m.schedulePasteFinalize()
	}

	bufferedContent := strings.Join(m.pasteBuffer.Chunks, "")
	baseValue := m.pasteBuffer.BaseValue
	m.pasteBuffer = tuiPasteBuffer{}

	if bufferedContent == "" {
		return nil
	}

	currentValue := m.input.Value()
	prefix, inserted, suffix := diffInsertedSegment(baseValue, currentValue)
	if inserted == "" {
		inserted = currentValue
		prefix = ""
		suffix = ""
	}
	if inserted == "" {
		return nil
	}

	placeholder := buildPastePlaceholder(inserted)
	m.pendingPaste = &tuiPendingPaste{
		Placeholder: placeholder,
		Content:     inserted,
	}
	replacedValue := prefix + placeholder + suffix
	m.input.SetValue(replacedValue)
	m.input.CursorEnd()
	m.inputHint = fmt.Sprintf("Pasted %d lines. Submit to include full text.", countLines(inserted))
	m.syncInputState()
	return nil
}

// diffInsertedSegment derives the changed segment between a base and updated value.
func diffInsertedSegment(baseValue string, updatedValue string) (string, string, string) {
	baseRunes := []rune(baseValue)
	updatedRunes := []rune(updatedValue)

	prefixIndex := 0
	for prefixIndex < len(baseRunes) && prefixIndex < len(updatedRunes) {
		if baseRunes[prefixIndex] != updatedRunes[prefixIndex] {
			break
		}
		prefixIndex++
	}

	suffixIndex := 0
	for suffixIndex < len(baseRunes)-prefixIndex && suffixIndex < len(updatedRunes)-prefixIndex {
		baseRune := baseRunes[len(baseRunes)-1-suffixIndex]
		updatedRune := updatedRunes[len(updatedRunes)-1-suffixIndex]
		if baseRune != updatedRune {
			break
		}
		suffixIndex++
	}

	prefix := string(updatedRunes[:prefixIndex])
	inserted := string(updatedRunes[prefixIndex : len(updatedRunes)-suffixIndex])
	suffix := string(updatedRunes[len(updatedRunes)-suffixIndex:])
	return prefix, inserted, suffix
}

// buildPastePlaceholder formats a readable placeholder for buffered paste content.
func buildPastePlaceholder(content string) string {
	newlineCount := strings.Count(content, "\n")
	return fmt.Sprintf("[Pasted text +%d lines] ", newlineCount)
}

// countLines returns the number of lines in a string, counting an empty string as 0.
func countLines(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

// doublePressTriggered implements double-press confirmation for destructive actions.
func (m *tuiModel) doublePressTriggered(key string, hint string) bool {
	now := time.Now()
	if m.doublePress.Key == key && now.Sub(m.doublePress.At) <= tuiDoublePressWindow {
		m.doublePress = tuiDoublePress{}
		m.inputHint = ""
		return true
	}
	m.doublePress = tuiDoublePress{Key: key, At: now}
	if hint != "" {
		m.inputHint = hint
	}
	return false
}

// padRight pads a string with spaces to the target width.
func padRight(value string, width int) string {
	runes := []rune(value)
	if len(runes) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(runes))
}

// maxInt returns the maximum of two integers.
func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
