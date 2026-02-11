package terminal

import (
	"fmt"
	"os"
	"os/exec"
)

// TerminalLauncher is the interface for spawning processes in terminal windows
type TerminalLauncher interface {
	// Spawn opens a new terminal window and runs the command
	// name: displayed in terminal title
	// workDir: working directory for the command
	// command: the command to run
	// args: arguments to the command
	// Returns error if spawn fails
	Spawn(name string, workDir string, command string, args []string) error

	// IsAvailable checks if this terminal is installed and usable
	IsAvailable() bool

	// Name returns the terminal name for display purposes
	Name() string
}

// NewLauncher creates a TerminalLauncher for the specified terminal app
// Supported values: "ghostty", "terminal" (Apple Terminal.app), "iterm", "wezterm", "kitty"
// If empty string, attempts to auto-detect an available terminal
func NewLauncher(terminalApp string) (TerminalLauncher, error) {
	if terminalApp == "" {
		return autoDetect()
	}

	switch terminalApp {
	case "ghostty":
		launcher := &GhosttyLauncher{}
		if !launcher.IsAvailable() {
			return nil, fmt.Errorf("ghostty is not installed or not in PATH")
		}
		return launcher, nil

	case "terminal", "apple":
		launcher := &AppleTerminalLauncher{}
		if !launcher.IsAvailable() {
			return nil, fmt.Errorf("Terminal.app is not available (are you on macOS?)")
		}
		return launcher, nil

	case "iterm", "iterm2":
		launcher := &ITermLauncher{}
		if !launcher.IsAvailable() {
			return nil, fmt.Errorf("iTerm2 is not installed")
		}
		return launcher, nil

	case "wezterm":
		launcher := &WeztermLauncher{}
		if !launcher.IsAvailable() {
			return nil, fmt.Errorf("wezterm is not installed or not in PATH")
		}
		return launcher, nil

	case "kitty":
		launcher := &KittyLauncher{}
		if !launcher.IsAvailable() {
			return nil, fmt.Errorf("kitty is not installed or not in PATH")
		}
		return launcher, nil

	default:
		return nil, fmt.Errorf("unknown terminal: %s (supported: ghostty, terminal, iterm, wezterm, kitty)", terminalApp)
	}
}

// autoDetect tries to find an available terminal in order of preference
func autoDetect() (TerminalLauncher, error) {
	// Try in order of preference
	launchers := []TerminalLauncher{
		&GhosttyLauncher{},
		&WeztermLauncher{},
		&KittyLauncher{},
		&ITermLauncher{},
		&AppleTerminalLauncher{},
	}

	for _, l := range launchers {
		if l.IsAvailable() {
			return l, nil
		}
	}

	return nil, fmt.Errorf("no supported terminal found (tried: ghostty, wezterm, kitty, iterm, terminal)")
}

// GetTerminalFromEnv returns the terminal app from CRUX_TERMINAL env var
func GetTerminalFromEnv() string {
	return os.Getenv("CRUX_TERMINAL")
}

// commandExists checks if a command is available in PATH
func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// appExists checks if a macOS app bundle exists
func appExists(appName string) bool {
	// Check common locations
	paths := []string{
		"/Applications/" + appName + ".app",
		os.Getenv("HOME") + "/Applications/" + appName + ".app",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}
