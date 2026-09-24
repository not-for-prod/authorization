package authorization_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Compile and exercise the example from outside this module's import boundary.
// This checks that the constructor and public drivers remain usable by a host
// with its own module and dependency resolution.
func TestExternalHost(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()

	for _, name := range []string{"go.mod", "go.sum", "examples/embedded/host_test.go"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}

		if name == "go.mod" {
			content = []byte(strings.Replace(string(content), "module authorization", "module example.com/auth-host", 1) +
				"\nrequire authorization v0.0.0\nreplace authorization => " + strconv.Quote(root) + "\n")
		}

		if err := os.WriteFile(filepath.Join(dir, filepath.Base(name)), content, 0600); err != nil {
			t.Fatal(err)
		}
	}

	command := exec.CommandContext(t.Context(), "go", "test", "-mod=mod", "-count=1", ".")
	command.Dir = dir

	command.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("external consumer: %v\n%s", err, output)
	}
}
