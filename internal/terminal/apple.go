package terminal

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// AppleTerminalLauncher implements TerminalLauncher for macOS Terminal.app
type AppleTerminalLauncher struct{}

func (a *AppleTerminalLauncher) Name() string {
	return "Terminal.app"
}

func (a *AppleTerminalLauncher) IsAvailable() bool {
	// Only available on macOS
	return runtime.GOOS == "darwin"
}

func (a *AppleTerminalLauncher) Spawn(name string, workDir string, command string, args []string) error {
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

	// Use AppleScript to open a new Terminal window
	// The script:
	// 1. Opens a new window
	// 2. Runs the command
	// 3. Sets the window title (via custom title setting)
	appleScript := fmt.Sprintf(`
tell application "Terminal"
	activate
	set newWindow to do script %q
	set custom title of front window to %q
end tell
`, shellCmd, name)

	cmd := exec.Command("osascript", "-e", appleScript)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute AppleScript for Terminal.app: %w", err)
	}

	return nil
}
