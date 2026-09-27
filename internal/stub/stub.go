// Package stub gives tests the termstub program to run on a
// pseudo-terminal.
package stub

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	once  sync.Once
	built string
	err   error
)

// Path is the termstub executable: $KVIT_TERMSTUB where it is set, which is
// how tests cross-compiled for Windows find the one built beside them, and
// otherwise one built once into a temporary directory for this test binary.
func Path(tb testing.TB) string {
	tb.Helper()
	if p := os.Getenv("KVIT_TERMSTUB"); p != "" {
		return p
	}
	once.Do(func() {
		dir, e := os.MkdirTemp("", "termstub")
		if e != nil {
			err = e
			return
		}
		name := "termstub"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		built = filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-o", built, "github.com/kvit-s/kvit-term/internal/termstub")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, e := cmd.CombinedOutput(); e != nil {
			err = &buildError{string(out), e}
		}
	})
	if err != nil {
		tb.Fatalf("building termstub: %v", err)
	}
	return built
}

type buildError struct {
	out string
	err error
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }
