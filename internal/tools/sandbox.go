package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sandbox controls filesystem access for tool operations.
type Sandbox struct {
	// Roots is the allowlist of permitted directories.
	Roots []string
	// Deny is the denylist of forbidden directory prefixes.
	Deny []string
}

var (
	// ErrPathNotAllowed indicates the path is outside allowed roots.
	ErrPathNotAllowed = errors.New("path not allowed")
	// ErrPathDenied indicates the path is explicitly denied.
	ErrPathDenied = errors.New("path denied")
)

// NewSandbox builds a sandbox from root allowlist and default denylist.
func NewSandbox(roots []string) *Sandbox {
	deny := []string{"/proc", "/sys", "/dev"}
	home, err := os.UserHomeDir()
	if err == nil {
		// Protect SSH keys from accidental exfiltration.
		deny = append(deny, filepath.Join(home, ".ssh"))
	}
	return &Sandbox{Roots: roots, Deny: deny}
}

// ResolvePath validates and returns a normalized absolute path.
func (s *Sandbox) ResolvePath(path string, requireExisting bool) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path: %w", ErrPathNotAllowed)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	clean := filepath.Clean(absolute)

	if requireExisting {
		// For read-only operations we must ensure the path exists.
		if _, err := os.Stat(clean); err != nil {
			return "", err
		}
	}

	realPath := clean
	if _, err := os.Lstat(clean); err == nil {
		// Resolve symlinks to prevent path traversal.
		if resolved, err := filepath.EvalSymlinks(clean); err == nil {
			realPath = resolved
		}
	}

	for _, denied := range s.Deny {
		if isSubpath(denied, realPath) {
			return "", fmt.Errorf("%w: %s", ErrPathDenied, realPath)
		}
	}

	for _, root := range s.Roots {
		if root == "" {
			continue
		}
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if isSubpath(rootAbs, realPath) {
			return realPath, nil
		}
	}

	return "", fmt.Errorf("%w: %s", ErrPathNotAllowed, realPath)
}

// isSubpath returns true when target is equal to or inside root.
func isSubpath(root string, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}
