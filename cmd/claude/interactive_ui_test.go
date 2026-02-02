package main

import "testing"

// TestHandleSlashCommandKnown verifies known slash commands are handled.
func TestHandleSlashCommandKnown(testingHandle *testing.T) {
	opts := &options{}

	handled, output := handleSlashCommand("/context", opts)
	if !handled {
		testingHandle.Fatalf("expected slash command to be handled")
	}
	if output == "" {
		testingHandle.Fatalf("expected output for known slash command")
	}
}

// TestHandleSlashCommandUnknown verifies unknown commands are reported.
func TestHandleSlashCommandUnknown(testingHandle *testing.T) {
	opts := &options{}

	handled, output := handleSlashCommand("/nope", opts)
	if !handled {
		testingHandle.Fatalf("expected unknown slash command to be handled")
	}
	if output == "" {
		testingHandle.Fatalf("expected output for unknown slash command")
	}
}

// TestHandleSlashCommandDisabled verifies the disable flag bypasses handling.
func TestHandleSlashCommandDisabled(testingHandle *testing.T) {
	opts := &options{DisableSlashCommands: true}

	handled, output := handleSlashCommand("/context", opts)
	if handled {
		testingHandle.Fatalf("expected slash command handling to be disabled")
	}
	if output != "" {
		testingHandle.Fatalf("expected no output when disabled")
	}
}

// TestHandleSlashCommandNonSlash verifies non-slash input is ignored.
func TestHandleSlashCommandNonSlash(testingHandle *testing.T) {
	opts := &options{}

	handled, output := handleSlashCommand("hello", opts)
	if handled {
		testingHandle.Fatalf("expected non-slash input to be ignored")
	}
	if output != "" {
		testingHandle.Fatalf("expected no output for non-slash input")
	}
}
