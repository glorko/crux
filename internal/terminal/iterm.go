package terminal

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ITermLauncher implements TerminalLauncher for iTerm2
type ITermLauncher struct{}

func (i *ITermLauncher) Name() string {
	return "iTerm2"
}

func (i *ITermLauncher) IsAvailable() bool {
	// Only available on macOS and iTerm2 must be installed
	if runtime.GOOS != "darwin" {
		return false
	}
	return appExists("iTerm")
}

func (i *ITermLauncher) Spawn(name string, workDir string, command string, args []string) error {
	// Build the full command to execute
	fullCmd := command
	if len(args) > 0 {
		fullCmd = command + " " + strings.Join(args, " ")
	}

	// Build shell command with cd and exec
	shellCmd := fullCmd
	if workDir != "" {
		shellCmd = fmt.Sprintf("cd %q && %s", workDir, fullCmd)
	}

	// Use AppleScript to open a new iTerm2 window
	appleScript := fmt.Sprintf(`
tell application "iTerm"
	activate
	set newWindow to (create window with default profile)
	tell current session of newWindow
		set name to %q
		write text %q
	end tell
end tell
`, name, shellCmd)

	cmd := exec.Command("osascript", "-e", appleScript)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute AppleScript for iTerm2: %w", err)
	}

	return nil
}
