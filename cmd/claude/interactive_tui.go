package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/openclaude/openclaude/internal/agent"
	"github.com/openclaude/openclaude/internal/llm/openai"
	"github.com/openclaude/openclaude/internal/session"
	"github.com/openclaude/openclaude/internal/tools"
)

// tuiMessage is a rendered chat entry in the interactive UI.
type tuiMessage struct {
	// Role labels the message origin (user, assistant, system, tool).
	Role string
	// Content is the message text displayed in the chat viewport.
	Content string
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
	// markdownRenderer formats assistant output when available.
	markdownRenderer *glamour.TermRenderer
	// statusText is the bottom status line.
	statusText string
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
	input.Placeholder = "Type a message..."
	input.Focus()
	input.CharLimit = 0
	input.Prompt = "> "
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
		markdownRenderer: renderer,
		statusText:       "Enter: send | Alt+Enter: newline | Ctrl+P/N: history | Tab: panes | Ctrl+C: cancel | Ctrl+Q: quit",
		activePane:       "input",
		chatAutoScroll:   true,
		toolAutoScroll:   true,
	}
	if runner != nil {
		modelState.permissionMode = string(runner.Permissions.Mode)
	}
	modelState.refreshPlanMode()
	modelState.historyIndex = len(modelState.inputHistory)
	modelState.bootstrapHistory()
	return modelState
}

// Init starts the blinking cursor for the input field.
func (m *tuiModel) Init() tea.Cmd {
	return textarea.Blink
}

// Update handles UI events and streaming updates.
func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.applyWindowSize(typed)
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(typed)
	case streamDeltaMsg:
		m.streamBuffer.WriteString(typed.Text)
		m.refreshChat()
		return m, m.listenStream()
	case toolEventMsg:
		m.appendToolEvent(typed.Event)
		return m, m.listenStream()
	case permissionRequestMsg:
		m.handlePermissionRequest(typed.Request)
		return m, m.listenStream()
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
	header := m.renderHeader()
	body := m.renderBody()
	input := m.renderInput()
	status := m.renderStatus()
	return lipgloss.JoinVertical(lipgloss.Left, header, body, input, status)
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

	switch key.String() {
	case "ctrl+c":
		if m.running {
			m.cancelRun("Cancelled.")
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
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

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(key)
	return m, cmd
}

// submitInput sends the current input as a new user message.
func (m *tuiModel) submitInput() (tea.Model, tea.Cmd) {
	if m.running {
		m.statusText = "Wait for the current response or cancel with Ctrl+C."
		return m, nil
	}
	value := strings.TrimSpace(m.input.Value())
	if value == "" {
		return m, nil
	}
	m.input.SetValue("")
	m.statusText = ""
	m.appendInputHistory(value)

	if handled, output := handleSlashCommand(value, m.opts); handled {
		m.appendMessage("system", output)
		m.refreshChat()
		return m, nil
	}

	m.appendMessage("user", value)
	m.refreshChat()

	m.history = append(m.history, openai.Message{Role: "user", Content: value})
	m.running = true
	m.streamBuffer.Reset()
	m.toolLines = nil
	m.toolView.SetContent("No tool activity yet.")
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.statusText = "Thinking..."
	m.streamCh = make(chan tea.Msg, 128)
	m.configureAuthorizer(ctx)

	cmd := m.startStream(ctx)
	return m, tea.Batch(cmd, m.listenStream())
}

// appendInputHistory records an input line for history navigation.
func (m *tuiModel) appendInputHistory(value string) {
	if value == "" {
		return
	}
	m.inputHistory = append(m.inputHistory, value)
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
		return
	}
	m.input.SetValue(m.inputHistory[m.historyIndex])
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
	m.statusText = ""
	m.cancel = nil
	m.pendingPermission = nil
	if result == nil {
		m.appendMessage("assistant", m.streamBuffer.String())
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
	m.appendMessage("assistant", finalText)
	m.streamBuffer.Reset()
	m.refreshChat()
	if m.store != nil {
		m.persistRun(result)
	}
}

// finishError handles errors from the streaming run.
func (m *tuiModel) finishError(err error) {
	m.running = false
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
	m.refreshPlanMode()
}

// refreshChat rebuilds the chat viewport content.
func (m *tuiModel) refreshChat() {
	var builder strings.Builder
	for _, msg := range m.chatMessages {
		builder.WriteString(m.renderMessage(msg, false))
		builder.WriteString("\n\n")
	}
	if m.running {
		streamText := m.streamBuffer.String()
		if streamText != "" {
			builder.WriteString(m.renderMessage(tuiMessage{Role: "assistant", Content: streamText}, true))
			builder.WriteString("\n\n")
		}
	}
	m.chatView.SetContent(builder.String())
	if m.chatAutoScroll {
		m.chatView.GotoBottom()
	}
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

// bootstrapHistory seeds the chat view with previous session messages.
func (m *tuiModel) bootstrapHistory() {
	for _, message := range m.history {
		if message.Role == "system" {
			continue
		}
		if message.Role == "tool" {
			m.appendMessage("tool", formatContent(message.Content))
			continue
		}
		m.appendMessage(message.Role, extractMessageText(message))
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

	headerHeight := 1
	statusHeight := 1
	inputHeight := m.input.Height()
	bodyHeight := m.height - headerHeight - statusHeight - inputHeight
	if bodyHeight < 4 {
		bodyHeight = 4
	}

	toolWidth := maxInt(24, m.width/4)
	if toolWidth > 60 {
		toolWidth = 60
	}
	chatWidth := m.width - toolWidth - 3
	if chatWidth < 20 {
		chatWidth = 20
		toolWidth = maxInt(20, m.width-chatWidth-3)
	}

	m.chatView.Width = chatWidth - 2
	m.chatView.Height = bodyHeight - 2
	m.toolView.Width = toolWidth - 2
	m.toolView.Height = bodyHeight - 2
	m.input.SetWidth(m.width - 2)

	m.refreshChat()
	m.refreshTools()
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
	chat := m.renderPane("Conversation", m.chatView.View(), m.chatView.Width+2)
	tools := m.renderPane("Tools", m.toolView.View(), m.toolView.Width+2)
	return lipgloss.JoinHorizontal(lipgloss.Top, chat, tools)
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
	style := lipgloss.NewStyle().Border(m.border()).Padding(0, 1)
	return style.Render(m.input.View())
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
	label := strings.ToUpper(message.Role)
	content := message.Content
	style := lipgloss.NewStyle()
	switch message.Role {
	case "user":
		style = style.Foreground(lipgloss.Color("39")).Bold(true)
		label = "YOU"
	case "assistant":
		style = style.Foreground(lipgloss.Color("10")).Bold(true)
		label = "ASSISTANT"
	case "tool":
		style = style.Foreground(lipgloss.Color("13"))
		label = "TOOL"
	case "system":
		style = style.Foreground(lipgloss.Color("3"))
		label = "SYSTEM"
	}
	if !streaming && message.Role != "user" {
		content = m.renderMarkdown(content)
	}
	return fmt.Sprintf("%s\n%s", style.Render(label+":"), content)
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
