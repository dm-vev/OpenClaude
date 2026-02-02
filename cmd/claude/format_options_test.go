package main

import (
	"strings"
	"testing"
)

// TestValidateFormatOptions enforces stream-json flag gating to match Claude Code.
func TestValidateFormatOptions(testingHandle *testing.T) {
	// Define a table of flag combinations that should pass or fail.
	cases := []struct {
		name        string
		opts        options
		expectError string
	}{
		{
			name: "interactive default ok",
			opts: options{
				Print:        false,
				InputFormat:  "text",
				OutputFormat: "text",
			},
			expectError: "",
		},
		{
			name: "stream input requires stream output",
			opts: options{
				Print:        true,
				InputFormat:  "stream-json",
				OutputFormat: "text",
			},
			expectError: "requires output-format=stream-json",
		},
		{
			name: "output format requires print",
			opts: options{
				Print:        false,
				InputFormat:  "text",
				OutputFormat: "json",
			},
			expectError: "--output-format only works with --print",
		},
		{
			name: "input format requires print",
			opts: options{
				Print:        false,
				InputFormat:  "stream-json",
				OutputFormat: "stream-json",
			},
			expectError: "--input-format only works with --print",
		},
		{
			name: "stream-json output requires verbose",
			opts: options{
				Print:        true,
				InputFormat:  "text",
				OutputFormat: "stream-json",
				Verbose:      false,
			},
			expectError: "--output-format=stream-json requires --verbose",
		},
		{
			name: "partials require stream-json print",
			opts: options{
				Print:                  false,
				InputFormat:            "text",
				OutputFormat:           "text",
				IncludePartialMessages: true,
			},
			expectError: "--include-partial-messages requires --print and --output-format=stream-json",
		},
		{
			name: "valid stream-json print",
			opts: options{
				Print:        true,
				InputFormat:  "stream-json",
				OutputFormat: "stream-json",
				Verbose:      true,
			},
			expectError: "",
		},
	}

	for _, item := range cases {
		item := item
		testingHandle.Run(item.name, func(testingHandle *testing.T) {
			err := validateFormatOptions(&item.opts)
			if item.expectError == "" && err != nil {
				testingHandle.Fatalf("unexpected error: %v", err)
			}
			if item.expectError != "" {
				if err == nil {
					testingHandle.Fatalf("expected error containing %q", item.expectError)
				}
				if !strings.Contains(err.Error(), item.expectError) {
					testingHandle.Fatalf("expected error containing %q, got %v", item.expectError, err)
				}
			}
		})
	}
}

// TestRunPrintModeStreamJSONRequiresVerbose validates the early verbose guard.
func TestRunPrintModeStreamJSONRequiresVerbose(testingHandle *testing.T) {
	// Build minimal options to exercise the verbose check before deeper dependencies.
	opts := &options{
		Print:        true,
		InputFormat:  "text",
		OutputFormat: "stream-json",
		Verbose:      false,
	}

	err := runPrintModeStreamJSON(nil, opts, nil, nil, "", "model-x", "session-1", nil, nil, "config")
	if err == nil {
		testingHandle.Fatalf("expected verbose requirement error")
	}
	if !strings.Contains(err.Error(), "requires --verbose") {
		testingHandle.Fatalf("unexpected error: %v", err)
	}
}
