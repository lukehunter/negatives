package archivesync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Outcome is the meaning of one robocopy exit code. robocopy's exit code is
// a bit field: 1 files copied, 2 extra files on the destination, 4
// mismatched files or folders, 8 some copies failed, 16 fatal error. 0 to 7
// is success, 8 and above is failure.
type Outcome struct {
	Code     int
	OK       bool
	Copied   bool // 1: one or more files were copied
	Extra    bool // 2: the destination has files the source doesn't (kept)
	Mismatch bool // 4: a file on one side is a folder on the other
	Failed   bool // 8: some files or folders could not be copied
	Fatal    bool // 16: robocopy did nothing (bad arguments, no access)
}

// Interpret turns a robocopy exit code into an Outcome. Anything outside
// 0..7, including negative codes from a killed process, is a failure.
func Interpret(code int) Outcome {
	o := Outcome{Code: code, OK: code >= 0 && code <= 7}
	if code < 0 || code > 31 {
		o.Fatal = true
		return o
	}
	o.Copied = code&1 != 0
	o.Extra = code&2 != 0
	o.Mismatch = code&4 != 0
	o.Failed = code&8 != 0
	o.Fatal = code&16 != 0
	return o
}

func (o Outcome) String() string {
	var parts []string
	if o.Copied {
		parts = append(parts, "copied")
	}
	if o.Extra {
		parts = append(parts, "extra files on backup")
	}
	if o.Mismatch {
		parts = append(parts, "mismatched files or folders")
	}
	if o.Failed {
		parts = append(parts, "some copies failed")
	}
	if o.Fatal {
		parts = append(parts, "fatal error")
	}
	if len(parts) == 0 {
		parts = append(parts, "nothing to copy")
	}
	return fmt.Sprintf("exit %d (%s)", o.Code, strings.Join(parts, ", "))
}

// Flags that could delete, move or overwrite something archive-sync must
// never touch. No planned job may contain any of them.
var forbiddenFlags = []string{"/MIR", "/PURGE", "/MOV", "/MOVE", "/CREATE"}

// Forbidden reports the first flag in args that archive-sync never uses.
func Forbidden(args []string) (string, bool) {
	for _, a := range args {
		for _, f := range forbiddenFlags {
			if strings.EqualFold(a, f) {
				return a, true
			}
		}
	}
	return "", false
}

// Runner runs robocopy with the given arguments and returns its exit code.
// An error means robocopy could not be started or was killed.
type Runner interface {
	Robocopy(ctx context.Context, args []string) (int, error)
}

// ExecRunner runs the robocopy.exe that ships with Windows.
type ExecRunner struct{}

func (ExecRunner) Robocopy(ctx context.Context, args []string) (int, error) {
	exe := "robocopy"
	if root := os.Getenv("SystemRoot"); root != "" {
		// The full path, so nothing earlier on PATH can stand in for it.
		exe = filepath.Join(root, "System32", "robocopy.exe")
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	// With /UNILOG+ robocopy writes its output to the log, not the console.
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Start()
	if err == nil {
		// So robocopy can't outlive archive-sync and keep writing after the
		// lock is released.
		killWithUs(cmd.Process.Pid)
		err = cmd.Wait()
	}
	if ctx.Err() != nil {
		// Killed on cancel. On Windows a killed process exits with 1, which
		// would otherwise read as "files copied".
		return -1, fmt.Errorf("robocopy stopped: %w", ctx.Err())
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() >= 0 {
		return exit.ExitCode(), nil
	}
	if err != nil {
		return -1, fmt.Errorf("robocopy: %w %s", err, strings.TrimSpace(stderr.String()))
	}
	return 0, nil
}
