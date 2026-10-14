// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Each expected entry names a call and the source line that must reject it.
func checkCompileErrors(t *testing.T, source string, expected map[int]string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "constraints_test.go")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}

	// -e reports every invalid call rather than stopping after a few errors.
	cmd := exec.CommandContext(t.Context(),
		"go", "test", "-c", "-vet=off", "-gcflags=-e",
		"-o", filepath.Join(dir, "constraints.test"), path)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("invalid calls compiled successfully")
	}

	missing := false
	for line, call := range expected {
		if !strings.Contains(string(output), fmt.Sprintf("constraints_test.go:%d:", line)) {
			t.Errorf("missing compile error for %s at line %d", call, line)
			missing = true
		}
	}
	if missing {
		t.Logf("compiler output:\n%s", output)
	}
}
