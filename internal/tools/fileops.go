package tools

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// backupFile stores a copy of the target file in the session backup folder.
// It uses a hashed filename to avoid collisions when different paths share a basename.
// Backups are best-effort: failures should block writes so users keep data safe.
func backupFile(toolCtx ToolContext, path string) error {
	if toolCtx.Store == nil || toolCtx.SessionID == "" {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}

	backupDir := filepath.Join(toolCtx.Store.BaseDir, "session-env", toolCtx.SessionID, "backup")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(path))
	backupName := fmt.Sprintf("%s-%x", filepath.Base(path), sum[:6])
	backupPath := filepath.Join(backupDir, backupName)
	return os.WriteFile(backupPath, data, 0o600)
}
