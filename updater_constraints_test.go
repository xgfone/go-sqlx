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

// Compile calls against the exported API and require an error at every invalid
// call, so widening even one function or method to any fails this test.
func TestArithmeticOperandConstraints(t *testing.T) {
	var source strings.Builder
	source.WriteString(`package arithmetic_test
import (
    "database/sql"
    sqlx "github.com/xgfone/go-sqlx"
)
func rejected() {
`)
	expected := make(map[int]string)
	for _, op := range []string{"Add", "Sub", "Mul", "Div"} {
		for _, value := range []string{
			`"10"`, `true`, `[]byte("10")`, `1i`, `uintptr(1)`, `new(int)`,
			`any(1)`, `nil`, `sql.Named("delta", 1)`, `sql.NullInt64{}`,
			`sqlx.Column("bonus")`,
		} {
			for _, call := range []string{
				fmt.Sprintf("sqlx.%s(\"v\", %s)", op, value),
				fmt.Sprintf("sqlx.Column(\"v\").%s(%s)", op, value),
			} {
				line := strings.Count(source.String(), "\n") + 1
				expected[line] = call
				fmt.Fprintf(&source, "    _ = %s\n", call)
			}
		}
	}
	source.WriteString("}\n")
	dir := t.TempDir()
	path := filepath.Join(dir, "constraints_test.go")
	if err := os.WriteFile(path, []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	// -e reports every invalid call rather than stopping after a few errors.
	cmd := exec.CommandContext(t.Context(), "go", "test", "-c", "-vet=off", "-gcflags=-e",
		"-o", filepath.Join(dir, "constraints.test"), path)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("invalid arithmetic operands compiled successfully")
	}
	for line, call := range expected {
		if !strings.Contains(string(output), fmt.Sprintf("constraints_test.go:%d:", line)) {
			t.Errorf("missing compile error for %s at line %d\n%s", call, line, output)
		}
	}
}
