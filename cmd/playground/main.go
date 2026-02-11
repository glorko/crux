package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/glorko/crux/internal/api"
	"github.com/glorko/crux/internal/terminal"
)

const version = "0.9.0"

// Worker represents a spawned worker process
type Worker struct {
	Name     string
	PipePath string
	PIDFile  string
	PID      int
}

// Implement api.Worker interface
func (w *Worker) GetName() string     { return w.Name }
func (w *Worker) GetPID() int         { return w.PID }
func (w *Worker) GetPipePath() string { return w.PipePath }
func (w *Worker) SendCommand(cmd string) error {
	return sendCommand(w.PipePath, cmd)
}

func main() {
	// Handle CLI flags
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			printHelp()
			return
		case "-v", "--version", "version":
			fmt.Printf("crux version %s\n", version)
			return
		case "init", "--init":
			generateExampleConfig()
			return
		case "prompt", "--prompt":
			printAgentPrompt()
			return
		}
	}

	fmt.Println("╔════════════════════════════════════════════════╗")
	fmt.Println("║              Crux - Dev Orchestrator           ║")
	fmt.Println("╚════════════════════════════════════════════════╝")
	fmt.Println()

	// Load config from current directory
	cfg, err := LoadPlaygroundConfig()
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		fmt.Println()
		fmt.Println("Create a config.yaml in the current directory with:")
		fmt.Println(`
services:
  - name: backend
    command: crux-backend-mock
    args: ["backend", "50051"]
  - name: flutter-ios
    command: crux-flutter-mock
    args: ["flutter-ios", "iPhone 15 Pro"]

terminal:
  app: wezterm  # Options: wezterm, kitty, tmux
`)
		os.Exit(1)
	}

	fmt.Printf("✅ Loaded config.yaml (%d services)\n", len(cfg.Services))

	// Validate service commands exist
	for _, svc := range cfg.Services {
		if _, err := os.Stat(svc.Command); os.IsNotExist(err) {
			fmt.Printf("❌ Command not found: %s\n", svc.Command)
			os.Exit(1)
		}
	}

	// Check terminal mode
	terminalApp := cfg.Terminal.App
	if terminalApp == "" {
		terminalApp = "wezterm" // default to wezterm (best CLI support)
	}

	fmt.Printf("✅ Terminal: %s\n", terminalApp)
	fmt.Println()

	switch terminalApp {
	case "wezterm":
		runWithWezterm(cfg)
	case "kitty":
		runWithKitty(cfg)
	default:
		// Fallback to tmux mode for other terminals
		runWithTmux(cfg)
	}
}

// runWithWezterm uses native Wezterm tabs (no tmux needed)
func runWithWezterm(cfg *PlaygroundConfig) {
	wez := terminal.NewWeztermLauncher()
	if !wez.IsAvailable() {
		fmt.Println("❌ wezterm is not installed!")
		fmt.Println("   Install from: https://wezterm.org/")
		os.Exit(1)
	}

	fmt.Println("📺 Opening Wezterm with service tabs...")

	// Convert services to ServiceDef
	services := make([]terminal.ServiceDef, len(cfg.Services))
	for i, svc := range cfg.Services {
		services[i] = terminal.ServiceDef{
			Name:    svc.Name,
			Command: svc.Command,
			Args:    svc.ExpandArgs(),
			WorkDir: svc.WorkDir,
		}
	}

	if err := wez.StartWithTabs(services); err != nil {
		fmt.Printf("❌ Failed to start services: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("✅ Services running in Wezterm tabs!")
	fmt.Println("   Ctrl+Shift+T = new tab")
	fmt.Println("   Ctrl+Tab / Ctrl+Shift+Tab = switch tabs")
	fmt.Println("   Ctrl+Shift+W = close tab")
	fmt.Println()
}

// runWithKitty uses native Kitty tabs with remote control
func runWithKitty(cfg *PlaygroundConfig) {
	// Check if kitty is available
	if _, err := exec.LookPath("kitty"); err != nil {
		fmt.Println("❌ kitty is not installed!")
		fmt.Println("   Install from: https://sw.kovidgoyal.net/kitty/")
		os.Exit(1)
	}

	fmt.Println("📺 Opening Kitty with service tabs...")

	// Start kitty with remote control and first service
	first := cfg.Services[0]
	cmd := exec.Command("kitty",
		"-o", "allow_remote_control=yes",
		"--listen-on", "unix:/tmp/crux-kitty.sock",
		"--title", first.Name,
		first.Command,
	)
	cmd.Args = append(cmd.Args, first.ExpandArgs()...)
	if err := cmd.Start(); err != nil {
		fmt.Printf("❌ Failed to start kitty: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  ✅ %s\n", first.Name)

	// Wait for kitty to start
	time.Sleep(500 * time.Millisecond)

	// Spawn remaining services as tabs
	for _, svc := range cfg.Services[1:] {
		args := []string{
			"@", "--to", "unix:/tmp/crux-kitty.sock",
			"launch", "--type", "tab", "--title", svc.Name,
			svc.Command,
		}
		args = append(args, svc.ExpandArgs()...)

		tabCmd := exec.Command("kitty", args...)
		if err := tabCmd.Run(); err != nil {
			fmt.Printf("  ⚠️  %s: failed to create tab: %v\n", svc.Name, err)
		} else {
			fmt.Printf("  ✅ %s\n", svc.Name)
		}
	}

	fmt.Println()
	fmt.Println("✅ Services running in Kitty tabs!")
	fmt.Println("   Ctrl+Shift+T = new tab")
	fmt.Println("   Ctrl+Shift+Right/Left = switch tabs")
	fmt.Println()
}

// runWithTmux uses tmux inside a terminal app
func runWithTmux(cfg *PlaygroundConfig) {
	sessionName := cfg.Tmux.SessionName
	if sessionName == "" {
		sessionName = "crux"
	}

	tmux := terminal.NewTmuxLauncher(sessionName)
	if !tmux.IsAvailable() {
		fmt.Println("❌ tmux is not installed!")
		fmt.Println("   Install with: brew install tmux")
		os.Exit(1)
	}

	// Create tmux session
	if err := tmux.CreateSession(); err != nil {
		fmt.Printf("❌ Failed to create tmux session: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Created tmux session: %s\n", tmux.SessionName())

	workers := make([]*Worker, 0, len(cfg.Services))

	// Cleanup function
	cleanup := func() {
		fmt.Println("\n🛑 Shutting down...")
		for _, w := range workers {
			sendCommand(w.PipePath, "q")
			if w.PID > 0 {
				syscall.Kill(w.PID, syscall.SIGTERM)
			}
		}
		time.Sleep(500 * time.Millisecond)
		tmux.KillSession()
	}

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigChan
		cleanup()
		os.Exit(0)
	}()

	// Spawn workers
	fmt.Println("Spawning services in tmux...")
	for _, svc := range cfg.Services {
		w := &Worker{
			Name:     svc.Name,
			PipePath: fmt.Sprintf("/tmp/crux-%s.pipe", svc.Name),
			PIDFile:  fmt.Sprintf("/tmp/crux-%s.pid", svc.Name),
		}
		os.Remove(w.PipePath)
		os.Remove(w.PIDFile)

		if err := tmux.Spawn(svc.Name, svc.WorkDir, svc.Command, svc.ExpandArgs()); err != nil {
			fmt.Printf("❌ Failed to spawn %s: %v\n", svc.Name, err)
			cleanup()
			os.Exit(1)
		}
		workers = append(workers, w)
		fmt.Printf("  ✅ %s\n", svc.Name)
	}

	// Wait for PIDs
	time.Sleep(2 * time.Second)
	for _, w := range workers {
		if pid, err := readPIDFile(w.PIDFile); err == nil {
			w.PID = pid
		}
	}

	// Start API server
	apiServer := api.NewServer(cfg.API.Port)
	apiWorkers := make([]api.Worker, len(workers))
	for i, w := range workers {
		apiWorkers[i] = w
	}
	apiServer.SetWorkers(apiWorkers)
	apiServer.SetOnShutdown(func() {
		cleanup()
		os.Exit(0)
	})
	go apiServer.Start()

	fmt.Printf("\n🌐 API: http://localhost:%d\n", cfg.API.Port)

	// Open terminal with tmux
	termApp := cfg.Terminal.App
	if termApp == "" || termApp == "tmux" {
		termApp = "ghostty" // default terminal for tmux mode
	}

	fmt.Printf("📺 Opening %s with tmux...\n", termApp)
	openTerminalWithTmux(termApp, sessionName)

	fmt.Println()
	fmt.Println("✅ Services running in tmux: " + sessionName)
	fmt.Println("   Reattach: tmux attach -t " + sessionName)
	fmt.Println("   Stop all: tmux kill-session -t " + sessionName)
}

func openTerminalWithTmux(app string, sessionName string) error {
	tmuxCmd := fmt.Sprintf("tmux attach -t %s", sessionName)

	switch app {
	case "ghostty":
		cmd := exec.Command("open", "-na", "Ghostty", "--args", "-e", "/bin/sh", "-c", tmuxCmd)
		return cmd.Start()
	case "iterm", "iterm2":
		script := fmt.Sprintf(`tell application "iTerm" to create window with default profile command "%s"`, tmuxCmd)
		return exec.Command("osascript", "-e", script).Start()
	case "terminal", "terminal.app":
		script := fmt.Sprintf(`tell application "Terminal" to do script "%s"`, tmuxCmd)
		return exec.Command("osascript", "-e", script).Start()
	case "wezterm":
		return exec.Command("wezterm", "start", "--", "/bin/sh", "-c", tmuxCmd).Start()
	case "kitty":
		return exec.Command("kitty", "/bin/sh", "-c", tmuxCmd).Start()
	default:
		return fmt.Errorf("unknown terminal: %s", app)
	}
}

func readPIDFile(path string) (int, error) {
	for i := 0; i < 10; i++ {
		data, err := os.ReadFile(path)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		return strconv.Atoi(strings.TrimSpace(string(data)))
	}
	return 0, fmt.Errorf("PID file not found")
}

func sendCommand(pipePath string, cmd string) error {
	done := make(chan error, 1)
	go func() {
		pipe, err := os.OpenFile(pipePath, os.O_WRONLY, 0)
		if err != nil {
			done <- err
			return
		}
		defer pipe.Close()
		_, err = pipe.WriteString(cmd + "\n")
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		return fmt.Errorf("timeout")
	}
}

func printAgentPrompt() {
	fmt.Print(`# Crux Agent Prompt
# Copy everything below this line and paste to your AI assistant
# ================================================================

I want you to set up and run my development environment using crux.

NOTE: crux currently supports macOS only.

## Step 1: Verify crux is installed
Run: crux --help
If not found, tell the user to install crux:
  git clone https://github.com/glorko/crux.git
  cd crux
  ./install.sh
  # Add ~/go/bin and ~/bin to PATH

## Step 2: Check MCP integration
Run: which crux-mcp
If not found, tell the user:
  "crux-mcp is not installed. Run ./install.sh in the crux repo.
   
   Then add to Cursor MCP config:
   {\"mcpServers\":{\"crux\":{\"command\":\"${HOME}/bin/crux-mcp\",\"args\":[]}}}"

## Step 3: Analyze this project
Look at the project structure to identify:
- Backend services (Go, Python, Node.js, etc.)
- Mobile apps (Flutter, React Native, iOS, Android)
- Web apps (React, Vue, etc.)
- Required infrastructure (Docker, databases)
- Existing run scripts (run.sh, Makefile, package.json scripts)

## Step 3b: For mobile projects, detect available devices
If Flutter/React Native/mobile projects found:

For iOS simulators, run: xcrun simctl list devices available
- Look for "Booted" or available simulators
- Use the UUID (e.g., "90266925-B62F-4741-A89E-EF11BFA0CC57")
- If none available, tell user: "No iOS simulators found. Create one in Xcode > Window > Devices and Simulators"

For Android emulators, run: emulator -list-avds
- If AVDs exist, tell user to start one: "emulator -avd AVD_NAME &"
- Then get the device ID with: flutter devices (look for "emulator-5554" or similar)
- If no AVDs, tell user: "No Android emulators found. Create one in Android Studio > Device Manager"

Use the actual device IDs in config.yaml, not device names.

## Step 4: Create config.yaml
Create a config.yaml in the project root with:
- One service entry per runnable component
- Correct commands and arguments
- Working directories relative to config.yaml
- terminal.app set to wezterm

Run: crux --help
to see the exact config format and examples.

## Step 5: Run crux
Run: crux
This opens Wezterm with all services in separate tabs.

## Step 6: Use MCP to control services
Once running, you can use these MCP tools:
- crux_status: List all tabs
- crux_send: Send commands (r=reload, R=restart, q=quit)
- crux_logs: Get terminal output
- crux_focus: Focus a tab

Examples:
- "What services are running?"
- "Hot reload the Flutter app"
- "Show backend logs"

## Notes
- For Python projects with venv, use a shell script (./run.sh) as the command
- For Flutter, you need actual device IDs:
  - iOS: Get UUID with "xcrun simctl list devices"
  - Android: Start emulator first, then get ID from "flutter devices"
- Services run in interactive terminals - Ctrl+C, keyboard input all work
- Wezterm must be installed: brew install --cask wezterm

`)
}

func printHelp() {
	fmt.Printf(`crux - Dev Environment Orchestrator (v%s)

USAGE:
    crux [command]

COMMANDS:
    (none)      Start services from config.yaml in current directory
    init        Generate example config.yaml
    prompt      Print AI agent prompt (for configuring crux via LLM)
    help        Show this help message
    version     Show version

CONFIGURATION:
    Create a config.yaml in your project root:

    services:
      - name: backend           # Display name for the tab
        command: go             # Executable to run
        args: ["run", "./cmd/server"]  # Command arguments (optional)
        workdir: ./backend      # Working directory (optional, relative to config)

      - name: flutter-ios
        command: flutter
        args: ["run", "-d", "iPhone 15 Pro"]
        workdir: ./mobile

      - name: web-admin
        command: npm
        args: ["run", "dev"]
        workdir: ./webapps/admin

    terminal:
      app: wezterm    # Options: wezterm (recommended), kitty, tmux

EXAMPLES:
    # Go backend
    - name: api
      command: go
      args: ["run", "./cmd/server"]

    # Python with shell script (for venv activation)
    - name: backend
      command: ./run.sh
      workdir: backend

    # Flutter iOS (use UUID from: xcrun simctl list)
    - name: consumer-ios
      command: flutter
      args: ["run", "-d", "YOUR-IOS-SIMULATOR-UUID"]

    # Flutter Android (start emulator first, get ID from: flutter devices)
    - name: vendor-android
      command: flutter
      args: ["run", "-d", "emulator-5554"]

    # npm/Node.js
    - name: admin-web
      command: npm
      args: ["run", "dev"]

    # Docker
    - name: postgres
      command: docker
      args: ["compose", "up", "postgres"]

REQUIREMENTS:
    - Wezterm terminal: brew install --cask wezterm
    - Or Kitty: brew install --cask kitty
    - Or tmux: brew install tmux

MCP INTEGRATION:
    crux has an MCP server for AI assistant control (Cursor, etc.)

    Install:
      go build -o ~/bin/crux-mcp ./cmd/mcp

    Add to Cursor MCP config:
      {"mcpServers":{"crux":{"command":"${HOME}/bin/crux-mcp","args":[]}}}

    Available MCP tools:
      crux_status  - List all terminal tabs
      crux_send    - Send commands to tabs (r=reload, R=restart, q=quit)
      crux_logs    - Get terminal output from tabs
      crux_focus   - Focus a specific tab

MORE INFO:
    https://github.com/glorko/crux
`, version)
}

func generateExampleConfig() {
	configPath := "config.yaml"
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("❌ %s already exists\n", configPath)
		return
	}

	example := `# Crux Configuration
# Run with: crux

services:
  # Backend service example
  - name: backend
    command: go
    args: ["run", "./cmd/server"]
    # workdir: ./backend  # optional working directory

  # Flutter iOS example (get UUID: xcrun simctl list devices)
  - name: flutter-ios
    command: flutter
    args: ["run", "-d", "YOUR-IOS-SIMULATOR-UUID"]
    # workdir: ./mobile

  # Flutter Android example (start emulator first, get ID: flutter devices)
  - name: flutter-android
    command: flutter
    args: ["run", "-d", "emulator-5554"]

  # Web app example (uncomment to use)
  # - name: web-admin
  #   command: npm
  #   args: ["run", "dev"]
  #   workdir: ./webapps/admin

# Terminal to use for tabs
terminal:
  app: wezterm  # Options: wezterm (recommended), kitty, tmux
`

	if err := os.WriteFile(configPath, []byte(example), 0644); err != nil {
		fmt.Printf("❌ Failed to write %s: %v\n", configPath, err)
		return
	}

	fmt.Printf("✅ Created %s\n", configPath)
	fmt.Println("   Edit the file to match your project, then run: crux")
}

// Keep for potential future use
var _ = filepath.Join
