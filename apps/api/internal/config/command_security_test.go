package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Release commands must not contain an unauthenticated password-reset utility.
func TestCommandInventoryHasNoHardCodedPasswordReset(t *testing.T) {
	cmdRoot := filepath.Join("..", "..", "cmd")
	err := filepath.WalkDir(cmdRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(content)
		if strings.Contains(source, "UPDATE users SET password_hash") ||
			(strings.Contains(source, "GenerateFromPassword([]byte(\"") && strings.Contains(source, "WHERE email =")) {
			t.Errorf("unsafe hard-coded account reset found in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan command inventory: %v", err)
	}
}
