package terminal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const cruxPanesFile = "/tmp/crux-panes.txt"

// weztermListEntry matches one item from wezterm cli list --format json
type weztermListEntry struct {
	WindowID float64 `json:"window_id"`
	PaneID   float64 `json:"pane_id"`
}

// WeztermLauncher manages services in Wezterm tabs
type WeztermLauncher struct {
	paneIDs       []string          // Track pane IDs for each spawned tab
	firstPaneID   string            // Anchor pane ID (used for spawning new tabs)
	firstWindowID string            // Window ID of our crux window (preferred for spawn)
	servicePanes  map[string]string // service name -> pane ID (for API/MCP)
}

// NewWeztermLauncher creates a new Wezterm launcher
func NewWeztermLauncher() *WeztermLauncher {
	return &WeztermLauncher{
		paneIDs:      make([]string, 0),
		servicePanes: make(map[string]string),
	}
}

// KillPrevious kills any previous crux Wezterm window
func (w *WeztermLauncher) KillPrevious() {
	// Read saved pane IDs from previous run
	data, err := os.ReadFile(cruxPanesFile)
	if err != nil {
		return // No previous session
	}

	paneIDs := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, paneID := range paneIDs {
		paneID = strings.TrimSpace(paneID)
		if paneID == "" {
			continue
		}
		// Kill each pane - this closes tabs
		exec.Command("wezterm", "cli", "kill-pane", "--pane-id", paneID).Run()
	}

	os.Remove(cruxPanesFile)
}

// SavePanes saves current pane IDs for cleanup on next run
func (w *WeztermLauncher) SavePanes() error {
	if len(w.paneIDs) == 0 {
		return nil
	}

	content := strings.Join(w.paneIDs, "\n")
	return os.WriteFile(cruxPanesFile, []byte(content), 0644)
}

// Cleanup kills all panes from this session
func (w *WeztermLauncher) Cleanup() {
	for _, paneID := range w.paneIDs {
		exec.Command("wezterm", "cli", "kill-pane", "--pane-id", paneID).Run()
	}
	os.Remove(cruxPanesFile)
}

// Keep for potential future use
var _ = filepath.Join
var _ = bufio.NewReader

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

// wrapCommand wraps a command to log output and keep terminal open on failure
// Logs to /tmp/crux-logs/<service>/<timestamp>.log with symlink to latest.log
func wrapCommand(name string, command string, args []string) (string, []string) {
	// Build the original command string
	fullCmd := command
	for _, arg := range args {
		// Quote args with spaces
		if strings.Contains(arg, " ") {
			fullCmd += fmt.Sprintf(" '%s'", arg)
		} else {
			fullCmd += " " + arg
		}
	}
	
	// Log directory structure: /tmp/crux-logs/<service>/
	logDir := fmt.Sprintf("/tmp/crux-logs/%s", name)
	
	// Wrapper script:
	// 1. Create log directory
	// 2. Create timestamped log file
	// 3. Symlink latest.log to current log
	// 4. Clean up old logs (keep last 10)
	// 5. Run command with tee
	// 6. Keep open on failure
	wrapper := fmt.Sprintf(`
# Setup logging
LOG_DIR="%s"
mkdir -p "$LOG_DIR"
TIMESTAMP=$(date +%%Y-%%m-%%d_%%H%%M%%S)
LOG_FILE="$LOG_DIR/$TIMESTAMP.log"

# Symlink latest.log
rm -f "$LOG_DIR/latest.log"
ln -s "$LOG_FILE" "$LOG_DIR/latest.log"

# Clean old logs (keep last 10)
ls -t "$LOG_DIR"/*.log 2>/dev/null | grep -v latest.log | tail -n +11 | xargs rm -f 2>/dev/null

# Run with logging
echo "=== crux: %s ===" | tee "$LOG_FILE"
echo "Command: %s" | tee -a "$LOG_FILE"
echo "Started: $(date)" | tee -a "$LOG_FILE"
echo "Log: $LOG_FILE" | tee -a "$LOG_FILE"
echo "================================" | tee -a "$LOG_FILE"
%s 2>&1 | tee -a "$LOG_FILE"
EXIT_CODE=${PIPESTATUS[0]}
echo "" | tee -a "$LOG_FILE"
echo "=== Exited with code $EXIT_CODE at $(date) ===" | tee -a "$LOG_FILE"
if [ $EXIT_CODE -ne 0 ]; then
  echo ""
  echo "⚠️  Command failed! Log saved to: $LOG_FILE"
  echo "Press Enter to close this tab..."
  read
fi
`, logDir, name, fullCmd, fullCmd)
	
	return "/bin/bash", []string{"-c", wrapper}
}

// OpenWindow opens a new Wezterm window with a command
// Uses 'wezterm start' which launches the GUI
func (w *WeztermLauncher) OpenWindow(title string, workDir string, command string, args []string) (string, error) {
	// Wrap command to log and keep open on failure
	wrappedCmd, wrappedArgs := wrapCommand(title, command, args)
	
	cmdArgs := []string{"start"}
	
	if workDir != "" {
		cmdArgs = append(cmdArgs, "--cwd", workDir)
	}
	
	cmdArgs = append(cmdArgs, "--")
	cmdArgs = append(cmdArgs, wrappedCmd)
	cmdArgs = append(cmdArgs, wrappedArgs...)

	cmd := exec.Command("wezterm", cmdArgs...)
	err := cmd.Start()
	if err != nil {
		return "", fmt.Errorf("failed to open wezterm window: %w", err)
	}

	// Wait for wezterm to start and create its socket
	time.Sleep(1500 * time.Millisecond)
	
	// Get pane + window ID from our NEW window (wezterm start just created it).
	// Use LAST entry - our new window is typically last in the list.
	// Using "first" can pick a pane from an existing window (e.g. where user ran crux).
	listCmd := exec.Command("wezterm", "cli", "list", "--format", "json")
	output, err := listCmd.Output()
	paneID := "0"
	windowID := "0"
	if err == nil && len(output) > 0 {
		windowID, paneID = parseListLastWindowAndPane(string(output))
		if paneID == "0" {
			paneID = extractLastPaneID(string(output))
		}
		if paneID == "0" {
			paneID = extractFirstPaneID(string(output))
		}
	}
	w.firstWindowID = windowID
	w.firstPaneID = paneID
	w.paneIDs = append(w.paneIDs, paneID)

	setTabTitle(paneID, title)
	w.servicePanes[title] = paneID
	return paneID, nil
}

// setTabTitle sets the tab title so it shows the service name instead of "bash"
func setTabTitle(paneID string, title string) {
	cmd := exec.Command("wezterm", "cli", "set-tab-title", "--pane-id", paneID, title)
	_ = cmd.Run() // Best effort - don't fail if title can't be set
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

// extractLastPaneID extracts the LAST pane_id - use after wezterm start to get OUR new window's pane.
// wezterm list order is undefined; the window we just created is often last.
func extractLastPaneID(jsonOutput string) string {
	var last string
	search := jsonOutput
	for {
		idx := strings.Index(search, `"pane_id":`)
		if idx == -1 {
			break
		}
		start := idx + len(`"pane_id":`)
		end := start
		for end < len(search) && (search[end] >= '0' && search[end] <= '9') {
			end++
		}
		if end > start {
			last = search[start:end]
		}
		search = search[end:]
	}
	if last != "" {
		return last
	}
	return "0"
}

// parseListLastWindowAndPane parses wezterm list JSON and returns (windowID, paneID) of the LAST entry.
// The last entry is typically our newly created window from wezterm start.
func parseListLastWindowAndPane(jsonOutput string) (windowID, paneID string) {
	var entries []weztermListEntry
	if err := json.Unmarshal([]byte(jsonOutput), &entries); err != nil || len(entries) == 0 {
		return "0", "0"
	}
	last := entries[len(entries)-1]
	return fmt.Sprintf("%.0f", last.WindowID), fmt.Sprintf("%.0f", last.PaneID)
}

// GetFirstPaneID returns the first pane ID from any existing Wezterm window.
// Used to spawn a new tab into the user's current window (e.g. after one service crashed).
func GetFirstPaneID() (string, error) {
	cmd := exec.Command("wezterm", "cli", "list", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("wezterm not running or not available: %w", err)
	}
	paneID := extractFirstPaneID(string(output))
	if paneID == "0" {
		return "", fmt.Errorf("no wezterm panes found")
	}
	return paneID, nil
}

// SpawnTabInWindow spawns a new tab in a specific window. Prefer over SpawnTabInPane when we know the window.
func SpawnTabInWindow(windowID string, title string, workDir string, command string, args []string) (string, error) {
	wrappedCmd, wrappedArgs := wrapCommand(title, command, args)
	cmdArgs := []string{"cli", "spawn", "--window-id", windowID}
	if workDir != "" {
		cmdArgs = append(cmdArgs, "--cwd", workDir)
	}
	cmdArgs = append(cmdArgs, "--")
	cmdArgs = append(cmdArgs, wrappedCmd)
	cmdArgs = append(cmdArgs, wrappedArgs...)

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		cmd := exec.Command("wezterm", cmdArgs...)
		output, err := cmd.Output()
		if err == nil {
			newPaneID := strings.TrimSpace(string(output))
			setTabTitle(newPaneID, title)
			return newPaneID, nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("failed to spawn wezterm tab: %w", lastErr)
}

// SpawnTabInPane spawns a new tab in an existing Wezterm window by pane ID.
// Use GetFirstPaneID() to get a pane when attaching to the current window.
func SpawnTabInPane(anchorPaneID string, title string, workDir string, command string, args []string) (string, error) {
	wrappedCmd, wrappedArgs := wrapCommand(title, command, args)
	cmdArgs := []string{"cli", "spawn", "--pane-id", anchorPaneID}
	if workDir != "" {
		cmdArgs = append(cmdArgs, "--cwd", workDir)
	}
	cmdArgs = append(cmdArgs, "--")
	cmdArgs = append(cmdArgs, wrappedCmd)
	cmdArgs = append(cmdArgs, wrappedArgs...)

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		cmd := exec.Command("wezterm", cmdArgs...)
		output, err := cmd.Output()
		if err == nil {
			newPaneID := strings.TrimSpace(string(output))
			setTabTitle(newPaneID, title)
			return newPaneID, nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("failed to spawn wezterm tab: %w", lastErr)
}

// SpawnTab spawns a new tab in the existing Wezterm window
func (w *WeztermLauncher) SpawnTab(title string, workDir string, command string, args []string) (string, error) {
	if w.firstPaneID == "" && w.firstWindowID == "" {
		return "", fmt.Errorf("no anchor pane (start crux normally first, or use crux start-one with wezterm already open)")
	}
	var newPaneID string
	var err error
	if w.firstWindowID != "" && w.firstWindowID != "0" {
		newPaneID, err = SpawnTabInWindow(w.firstWindowID, title, workDir, command, args)
	} else {
		newPaneID, err = SpawnTabInPane(w.firstPaneID, title, workDir, command, args)
	}
	if err != nil {
		return "", err
	}
	w.paneIDs = append(w.paneIDs, newPaneID)
	w.servicePanes[title] = newPaneID
	return newPaneID, nil
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

// SendTextToPane sends text to a pane (for API /send)
func (w *WeztermLauncher) SendTextToPane(paneID string, text string) error {
	cmd := exec.Command("wezterm", "cli", "send-text", "--pane-id", paneID, "--no-paste", text+"\n")
	return cmd.Run()
}

// GetPaneScrollback returns the last N lines of scrollback from a pane
func (w *WeztermLauncher) GetPaneScrollback(paneID string, lines int) (string, error) {
	cmd := exec.Command("wezterm", "cli", "get-text", "--pane-id", paneID, "--start-line", fmt.Sprintf("%d", -lines))
	output, err := cmd.Output()
	return string(output), err
}

// GetServicePane returns pane ID for a service name (from session state)
func (w *WeztermLauncher) GetServicePane(service string) string {
	return w.servicePanes[service]
}

// FocusPane focuses a specific pane
func (w *WeztermLauncher) FocusPane(paneID string) error {
	cmd := exec.Command("wezterm", "cli", "activate-pane", "--pane-id", paneID)
	return cmd.Run()
}

// PaneInfo holds pane data from wezterm list (for API/TabController)
type PaneInfo struct {
	Title   string
	PaneID  string
	LogDir  string
	LogPath string
}

// ListPanesWithTitles returns panes with titles (refreshes from wezterm - no stale state)
func (w *WeztermLauncher) ListPanesWithTitles() ([]PaneInfo, error) {
	cmd := exec.Command("wezterm", "cli", "list", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var entries []struct {
		PaneID    int    `json:"pane_id"`
		Title     string `json:"title"`      // pane title (e.g. bash)
		TabTitle  string `json:"tab_title"`  // tab title (service name from set-tab-title)
	}
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, err
	}
	result := make([]PaneInfo, 0, len(entries))
	for _, e := range entries {
		name := e.TabTitle
		if name == "" {
			name = e.Title
		}
		if name == "" {
			name = "unknown"
		}
		result = append(result, PaneInfo{
			Title:   name,
			PaneID:  fmt.Sprintf("%d", e.PaneID),
			LogDir:  fmt.Sprintf("/tmp/crux-logs/%s", name),
			LogPath: fmt.Sprintf("/tmp/crux-logs/%s/latest.log", name),
		})
	}
	return result, nil
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
