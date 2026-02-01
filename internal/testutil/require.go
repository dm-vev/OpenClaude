package testutil

import (
	"reflect"
	"strings"
	"testing"
)

// RequireNoError fails the test immediately if err is non-nil.
func RequireNoError(testingHandle *testing.T, err error, message string) {
	testingHandle.Helper()
	if err == nil {
		return
	}
	if message == "" {
		testingHandle.Fatalf("unexpected error: %v", err)
	}
	testingHandle.Fatalf("%s: %v", message, err)
}

// RequireEqual fails the test immediately when values are not deeply equal.
func RequireEqual(testingHandle *testing.T, gotValue any, wantValue any, message string) {
	testingHandle.Helper()
	if reflect.DeepEqual(gotValue, wantValue) {
		return
	}
	if message == "" {
		testingHandle.Fatalf("values differ.\nwant: %#v\ngot: %#v", wantValue, gotValue)
	}
	testingHandle.Fatalf("%s.\nwant: %#v\ngot: %#v", message, wantValue, gotValue)
}

// AssertEqual reports a non-fatal error when values are not deeply equal.
func AssertEqual(testingHandle *testing.T, gotValue any, wantValue any, message string) {
	testingHandle.Helper()
	if reflect.DeepEqual(gotValue, wantValue) {
		return
	}
	if message == "" {
		testingHandle.Errorf("values differ.\nwant: %#v\ngot: %#v", wantValue, gotValue)
		return
	}
	testingHandle.Errorf("%s.\nwant: %#v\ngot: %#v", message, wantValue, gotValue)
}

// RequireTrue fails the test immediately if condition is false.
func RequireTrue(testingHandle *testing.T, condition bool, message string) {
	testingHandle.Helper()
	if condition {
		return
	}
	if message == "" {
		testingHandle.Fatalf("expected condition to be true")
		return
	}
	testingHandle.Fatalf("%s.", message)
}

// RequireStringContains fails the test immediately if substring is missing.
func RequireStringContains(testingHandle *testing.T, haystack string, needle string, message string) {
	testingHandle.Helper()
	if needle == "" || strings.Contains(haystack, needle) {
		return
	}
	if message == "" {
		testingHandle.Fatalf("expected %q to contain %q", haystack, needle)
		return
	}
	testingHandle.Fatalf("%s.", message)
}
