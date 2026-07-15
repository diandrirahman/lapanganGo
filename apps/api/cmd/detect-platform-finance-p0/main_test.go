package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func init() {
	os.Setenv("DATABASE_URL", "postgres://dummy:dummy@localhost:5432/dummy")
	os.Setenv("JWT_SECRET", "dummy_secret_for_testing_purposes")
	os.Setenv("REDIS_URL", "redis://localhost:6379")
}

func TestMainArgsValidation(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		exitCode int
		errStr   string
	}{
		{
			name:     "Invalid batch size 0",
			args:     []string{"--batch-size=0"},
			exitCode: 1,
			errStr:   "invalid batch size",
		},
		{
			name:     "Invalid batch size too large",
			args:     []string{"--batch-size=1001"},
			exitCode: 1,
			errStr:   "invalid batch size",
		},
		{
			name:     "Invalid max iterations",
			args:     []string{"--max-iterations=0"},
			exitCode: 1,
			errStr:   "invalid max iterations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			getenv := func(key string) string { return "" }

			code := run(tt.args, getenv, &stdout, &stderr)
			assert.Equal(t, tt.exitCode, code)
			assert.Contains(t, stderr.String(), tt.errStr)
		})
	}
}

func TestNoLeakOnFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	getenv := func(key string) string { return "" }

	code := run([]string{}, getenv, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.NotContains(t, stderr.String(), "postgres://") // No URL leak
	assert.NotContains(t, stderr.String(), "ERROR:")      // No raw DB error leak
}
