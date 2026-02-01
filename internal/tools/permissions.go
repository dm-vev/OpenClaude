package tools

// PermissionMode defines how tools should be authorized.
type PermissionMode string

const (
	// PermissionDefault prompts for risky actions.
	PermissionDefault PermissionMode = "default"
	// PermissionAcceptEdits auto-approves edits but prompts for bash commands.
	PermissionAcceptEdits PermissionMode = "acceptEdits"
	// PermissionDontAsk auto-approves within the sandbox.
	PermissionDontAsk PermissionMode = "dontAsk"
	// PermissionDelegate always asks the user for confirmation.
	PermissionDelegate PermissionMode = "delegate"
	// PermissionBypass allows all tool calls.
	PermissionBypass PermissionMode = "bypassPermissions"
	// PermissionPlan disables tool execution.
	PermissionPlan PermissionMode = "plan"
)

// Permissions controls tool access behavior.
type Permissions struct {
	Mode PermissionMode
}

// ShouldPrompt returns true if a tool should require user approval.
func (p Permissions) ShouldPrompt(toolName string) bool {
	switch p.Mode {
	case PermissionBypass, PermissionDontAsk:
		return false
	case PermissionAcceptEdits:
		return toolName == "Bash"
	case PermissionPlan:
		return false
	default:
		return toolName == "Bash" || toolName == "Edit"
	}
}

// AllowsTool returns true if the tool is allowed under the permission mode.
func (p Permissions) AllowsTool() bool {
	return p.Mode != PermissionPlan
}
