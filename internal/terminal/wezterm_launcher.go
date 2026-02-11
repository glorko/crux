package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// WeztermLauncher manages services in Wezterm tabs
type WeztermLauncher struct {
	paneIDs     []string // Track pane IDs for each spawned tab
	firstPaneID string   // The first pane ID (used for spawning new tabs)
}

// NewWeztermLauncher creates a new Wezterm launcher
func NewWeztermLauncher() *WeztermLauncher {
	return &WeztermLauncher{
		paneIDs: make([]string, 0),
	}
}

// Name returns the launcher name
func (w *WeztermLauncher) Name() string {
	return "wezterm"
}

// IsAvailable checks if wezterm is installed
func (w *WeztermLauncher) IsAvailable() bool {
	_, err := exec.LookPath("wezterm")
	return err == nil
}

// Spawn implements TerminalLauncher interface - spawns a new tab
func (w *WeztermLauncher) Spawn(name string, workDir string, command string, args []string) error {
	_, err := w.SpawnTab(name, workDir, command, args)
	return err
}

// OpenWindow opens a new Wezterm window with a command
// Uses 'wezterm start' which launches the GUI
func (w *WeztermLauncher) OpenWindow(title string, workDir string, command string, args []string) (string, error) {
	cmdArgs := []string{"start"}
	
	if workDir != "" {
		cmdArgs = append(cmdArgs, "--cwd", workDir)
	}
	
	cmdArgs = append(cmdArgs, "--")
	cmdArgs = append(cmdArgs, command)
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command("wezterm", cmdArgs...)
	err := cmd.Start()
	if err != nil {
		return "", fmt.Errorf("failed to open wezterm window: %w", err)
	}

	// Wait for wezterm to start and create its socket
	time.Sleep(1500 * time.Millisecond)
	
	// Get the pane ID from the running instance
	listCmd := exec.Command("wezterm", "cli", "list", "--format", "json")
	output, err := listCmd.Output()
	if err != nil {
		// Still return success - window was opened
		w.paneIDs = append(w.paneIDs, "0")
		return "0", nil
	}

	// Parse to get pane ID (simplified - just track that we have one)
	paneID := "0"
	if len(output) > 0 {
		// Extract first pane_id from JSON if possible
		paneID = extractFirstPaneID(string(output))
	}
	w.paneIDs = append(w.paneIDs, paneID)
	w.firstPaneID = paneID // Store for use when spawning new tabs
	return paneID, nil
}

// extractFirstPaneID extracts the first pane_id from wezterm cli list JSON output
func extractFirstPaneID(jsonOutput string) string {
	// Simple extraction - look for "pane_id":N
	idx := strings.Index(jsonOutput, `"pane_id":`)
	if idx == -1 {
		return "0"
	}
	start := idx + len(`"pane_id":`)
	end := start
	for end < len(jsonOutput) && (jsonOutput[end] >= '0' && jsonOutput[end] <= '9') {
		end++
	}
	if end > start {
		return jsonOutput[start:end]
	}
	return "0"
}

// SpawnTab spawns a new tab in the existing Wezterm window
func (w *WeztermLauncher) SpawnTab(title string, workDir string, command string, args []string) (string, error) {
	cmdArgs := []string{"cli", "spawn"}
	
	// Must specify --pane-id when running from outside Wezterm
	if w.firstPaneID != "" {
		cmdArgs = append(cmdArgs, "--pane-id", w.firstPaneID)
	}
	
	if workDir != "" {
		cmdArgs = append(cmdArgs, "--cwd", workDir)
	}
	
	cmdArgs = append(cmdArgs, "--")
	cmdArgs = append(cmdArgs, command)
	cmdArgs = append(cmdArgs, args...)

	// Retry logic - wezterm might not be fully ready
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		cmd := exec.Command("wezterm", cmdArgs...)
		output, err := cmd.Output()
		if err == nil {
			paneID := strings.TrimSpace(string(output))
			w.paneIDs = append(w.paneIDs, paneID)
			return paneID, nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("failed to spawn wezterm tab after retries: %w", lastErr)
}

// SpawnInPane spawns a command in a specific pane
func (w *WeztermLauncher) SpawnInPane(paneID string, command string, args []string) error {
	fullCmd := command
	if len(args) > 0 {
		fullCmd += " " + strings.Join(args, " ")
	}

	cmd := exec.Command("wezterm", "cli", "send-text", "--pane-id", paneID, "--no-paste", fullCmd+"\n")
	return cmd.Run()
}

// FocusPane focuses a specific pane
func (w *WeztermLauncher) FocusPane(paneID string) error {
	cmd := exec.Command("wezterm", "cli", "activate-pane", "--pane-id", paneID)
	return cmd.Run()
}

// ListPanes lists all panes in the current window
func (w *WeztermLauncher) ListPanes() ([]string, error) {
	cmd := exec.Command("wezterm", "cli", "list", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	// For now just return raw output - can parse JSON if needed
	return []string{string(output)}, nil
}

// GetPaneIDs returns tracked pane IDs
func (w *WeztermLauncher) GetPaneIDs() []string {
	return w.paneIDs
}

// ActivateWindow brings the Wezterm window to front
func (w *WeztermLauncher) ActivateWindow() error {
	// On macOS, activate the Wezterm app
	cmd := exec.Command("osascript", "-e", `tell application "WezTerm" to activate`)
	return cmd.Run()
}

// StartWithTabs opens Wezterm with multiple tabs, each running a command
// This is the main entry point for crux
func (w *WeztermLauncher) StartWithTabs(services []ServiceDef) error {
	if len(services) == 0 {
		return fmt.Errorf("no services to start")
	}

	// Get current working directory for services without explicit workdir
	cwd, _ := os.Getwd()

	// First service opens a new window
	first := services[0]
	workDir := first.WorkDir
	if workDir == "" {
		workDir = cwd
	}
	
	paneID, err := w.OpenWindow(first.Name, workDir, first.Command, first.Args)
	if err != nil {
		return fmt.Errorf("failed to open window for %s: %w", first.Name, err)
	}
	fmt.Printf("  ✅ %s (pane %s)\n", first.Name, paneID)

	// Remaining services open as tabs
	for _, svc := range services[1:] {
		workDir := svc.WorkDir
		if workDir == "" {
			workDir = cwd
		}
		
		paneID, err := w.SpawnTab(svc.Name, workDir, svc.Command, svc.Args)
		if err != nil {
			return fmt.Errorf("failed to spawn tab for %s: %w", svc.Name, err)
		}
		fmt.Printf("  ✅ %s (pane %s)\n", svc.Name, paneID)
	}

	// Activate the window
	w.ActivateWindow()

	return nil
}

// ServiceDef defines a service to spawn
type ServiceDef struct {
	Name    string
	Command string
	Args    []string
	WorkDir string
}
