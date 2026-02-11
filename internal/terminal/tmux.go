package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// TmuxLauncher manages processes inside a tmux session
type TmuxLauncher struct {
	sessionName string
	windowCount int
}

// NewTmuxLauncher creates a new tmux-based launcher
func NewTmuxLauncher(sessionName string) *TmuxLauncher {
	return &TmuxLauncher{
		sessionName: sessionName,
		windowCount: 0,
	}
}

func (t *TmuxLauncher) Name() string {
	return "tmux"
}

func (t *TmuxLauncher) IsAvailable() bool {
	return commandExists("tmux")
}

// SessionName returns the tmux session name
func (t *TmuxLauncher) SessionName() string {
	return t.sessionName
}

// CreateSession creates a new tmux session (call once at start)
func (t *TmuxLauncher) CreateSession() error {
	// Check if session already exists
	cmd := exec.Command("tmux", "has-session", "-t", t.sessionName)
	if err := cmd.Run(); err == nil {
		// Session exists, kill it first
		exec.Command("tmux", "kill-session", "-t", t.sessionName).Run()
	}

	// Create new detached session
	cmd = exec.Command("tmux", "new-session", "-d", "-s", t.sessionName, "-n", "crux")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Spawn creates a new tmux window and runs the command in it
func (t *TmuxLauncher) Spawn(name string, workDir string, command string, args []string) error {
	// Build the full command
	fullCmd := command
	if len(args) > 0 {
		fullCmd = command + " " + strings.Join(args, " ")
	}

	// Add cd if workDir specified
	if workDir != "" {
		fullCmd = fmt.Sprintf("cd %q && %s", workDir, fullCmd)
	}

	if t.windowCount == 0 {
		// First window - rename the default window and send command
		exec.Command("tmux", "rename-window", "-t", t.sessionName+":0", name).Run()
		cmd := exec.Command("tmux", "send-keys", "-t", t.sessionName+":"+name, fullCmd, "Enter")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to send command to tmux: %w", err)
		}
	} else {
		// Create new window with the command
		cmd := exec.Command("tmux", "new-window", "-t", t.sessionName, "-n", name)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create tmux window: %w", err)
		}

		// Send the command
		cmd = exec.Command("tmux", "send-keys", "-t", t.sessionName+":"+name, fullCmd, "Enter")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to send command to tmux: %w", err)
		}
	}

	t.windowCount++
	return nil
}

// SendKeys sends keys to a specific window
func (t *TmuxLauncher) SendKeys(windowName string, keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", t.sessionName+":"+windowName, keys)
	return cmd.Run()
}

// SendCtrlC sends Ctrl+C to a window (to stop/restart)
func (t *TmuxLauncher) SendCtrlC(windowName string) error {
	return t.SendKeys(windowName, "C-c")
}

// KillWindow kills a specific window
func (t *TmuxLauncher) KillWindow(windowName string) error {
	cmd := exec.Command("tmux", "kill-window", "-t", t.sessionName+":"+windowName)
	return cmd.Run()
}

// KillSession kills the entire session
func (t *TmuxLauncher) KillSession() error {
	cmd := exec.Command("tmux", "kill-session", "-t", t.sessionName)
	return cmd.Run()
}

// AttachCommand returns the command to attach to the session
func (t *TmuxLauncher) AttachCommand() string {
	return fmt.Sprintf("tmux attach -t %s", t.sessionName)
}

// Attach attaches to the tmux session interactively
func (t *TmuxLauncher) Attach() error {
	cmd := exec.Command("tmux", "attach", "-t", t.sessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ListWindows returns list of window names
func (t *TmuxLauncher) ListWindows() ([]string, error) {
	cmd := exec.Command("tmux", "list-windows", "-t", t.sessionName, "-F", "#{window_name}")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var windows []string
	for _, line := range lines {
		if line != "" {
			windows = append(windows, line)
		}
	}
	return windows, nil
}
