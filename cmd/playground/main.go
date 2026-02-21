package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
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
	// Parse config file path and positional args
	configPath := "config.yaml"
	args := os.Args[1:]
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
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
		case "-c", "--config":
			if i+1 < len(args) {
				configPath = args[i+1]
				i++
			} else {
				fmt.Println("❌ --config requires a file path")
				os.Exit(1)
			}
		default:
			if strings.HasPrefix(args[i], "-c=") {
				configPath = strings.TrimPrefix(args[i], "-c=")
			} else if strings.HasPrefix(args[i], "--config=") {
				configPath = strings.TrimPrefix(args[i], "--config=")
			} else {
				positional = append(positional, args[i])
			}
		}
	}

	fmt.Println("╔════════════════════════════════════════════════╗")
	fmt.Println("║              Crux - Dev Orchestrator           ║")
	fmt.Println("╚════════════════════════════════════════════════╝")
	fmt.Println()

	// Load config
	cfg, err := LoadPlaygroundConfig(configPath)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		fmt.Println()
		fmt.Printf("Create %s or run: crux init\n", configPath)
		fmt.Println()
		fmt.Println("Or specify a different config: crux -c config.test.yaml")
		os.Exit(1)
	}

	fmt.Printf("✅ Loaded %s (%d services", configPath, len(cfg.Services))
	if len(cfg.Dependencies) > 0 {
		fmt.Printf(", %d dependencies", len(cfg.Dependencies))
	}
	fmt.Println(")")
	fmt.Println()

	// Start only one service into existing Wezterm window (e.g. after a crash)
	if len(positional) >= 2 && positional[0] == "start-one" {
		serviceName := positional[1]
		if err := runStartOne(cfg, configPath, serviceName); err != nil {
			fmt.Printf("❌ %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Check and start dependencies first
	if err := cfg.CheckDependencies(); err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}

	// Validate service commands exist
	for _, svc := range cfg.Services {
		cmdPath := svc.Command
		
		// If command is a relative path (./something), resolve relative to workdir
		if strings.HasPrefix(cmdPath, "./") || strings.HasPrefix(cmdPath, "../") {
			if svc.WorkDir != "" {
				cmdPath = filepath.Join(svc.WorkDir, svc.Command)
			}
		}
		
		// Check if it's a file path
		if strings.Contains(cmdPath, "/") {
			if _, err := os.Stat(cmdPath); os.IsNotExist(err) {
				fmt.Printf("❌ Command not found: %s\n", cmdPath)
				fmt.Printf("   (workdir: %s)\n", svc.WorkDir)
				os.Exit(1)
			}
		} else {
			// It's a binary name - check if it's in PATH
			if _, err := exec.LookPath(cmdPath); err != nil {
				fmt.Printf("❌ Command not found in PATH: %s\n", cmdPath)
				os.Exit(1)
			}
		}
	}

	// Wezterm is the only supported terminal (native tabs, MCP, start-one).
	if cfg.Terminal.App != "" && cfg.Terminal.App != "wezterm" {
		fmt.Printf("⚠️  Only wezterm is supported; ignoring terminal.app=%q\n", cfg.Terminal.App)
	}
	fmt.Println("✅ Terminal: wezterm")
	fmt.Println()
	runWithWezterm(cfg)
}

// exitCodeInLogRe matches "Exited with code N" in the wrapper log tail
var exitCodeInLogRe = regexp.MustCompile(`Exited with code (\d+)`)

// checkServiceFailures reads each service's latest.log (last 2KB) and returns names that exited non-zero.
func checkServiceFailures(serviceNames []string) (failed []string) {
	const tailBytes = 2048
	for _, name := range serviceNames {
		logPath := filepath.Join("/tmp/crux-logs", name, "latest.log")
		data, err := os.ReadFile(logPath)
		if err != nil {
			continue // no log yet or missing
		}
		tail := data
		if len(tail) > tailBytes {
			tail = tail[len(tail)-tailBytes:]
		}
		matches := exitCodeInLogRe.FindSubmatch(tail)
		if len(matches) < 2 {
			continue
		}
		code, _ := strconv.Atoi(string(matches[1]))
		if code != 0 {
			failed = append(failed, name)
		}
	}
	return failed
}

// printFailedServicesWarning prints a big, bright warning listing failed services and their tab numbers.
func printFailedServicesWarning(serviceNames []string, failed []string) {
	if len(failed) == 0 {
		return
	}
	// Build tab number (1-based index in serviceNames) for each failed service
	var tabNums []string
	for _, f := range failed {
		for i, n := range serviceNames {
			if n == f {
				tabNums = append(tabNums, strconv.Itoa(i+1))
				break
			}
		}
	}
	tabList := strings.Join(tabNums, ", ")
	failedList := strings.Join(failed, ", ")
	// ANSI: bold, bright red background / white text for maximum visibility
	const bold = "\033[1m"
	const redBg = "\033[41m"
	const white = "\033[37m"
	const reset = "\033[0m"
	fmt.Println()
	fmt.Printf("%s%s%s", redBg, white, bold)
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║  ⚠️  WARNING: Your environment is started but some services FAILED with an error.  ║")
	fmt.Println("║                                                                              ║")
	fmt.Printf("║  Failed services: %-54s ║\n", failedList)
	fmt.Printf("║  Check logs in tabs: %-52s ║\n", tabList)
	fmt.Println("║                                                                              ║")
	fmt.Println("║  Or run: crux_logfile service=<name>  to read the log from here.              ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Print(reset)
	fmt.Println()
}

// runStartOne starts a single service in a new tab in the existing Wezterm window.
// Use after one service crashed: crux start-one backend (or crux -c other.yaml start-one backend).
func runStartOne(cfg *PlaygroundConfig, configPath string, serviceName string) error {
	var svc *ServiceConfig
	for i := range cfg.Services {
		if cfg.Services[i].Name == serviceName {
			svc = &cfg.Services[i]
			break
		}
	}
	if svc == nil {
		var names []string
		for _, s := range cfg.Services {
			names = append(names, s.Name)
		}
		return fmt.Errorf("service %q not found in config (available: %s)", serviceName, strings.Join(names, ", "))
	}

	cwd, _ := os.Getwd()
	workDir := svc.WorkDir
	if workDir == "" {
		workDir = cwd
	}

	// Only wezterm supports spawning into existing window via CLI
	wez := terminal.NewWeztermLauncher()
	if !wez.IsAvailable() {
		return fmt.Errorf("start-one requires wezterm (install from https://wezterm.org/)")
	}

	paneID, err := terminal.GetFirstPaneID()
	if err != nil {
		return fmt.Errorf("no Wezterm window open: %w (open Wezterm and run crux, or start the crashed tab manually)", err)
	}

	_, err = terminal.SpawnTabInPane(paneID, svc.Name, workDir, svc.Command, svc.ExpandArgs())
	if err != nil {
		return err
	}

	fmt.Printf("  ✅ %s started in new tab\n", svc.Name)
	wez.ActivateWindow()
	return nil
}

// runWithWezterm uses native Wezterm tabs (no tmux needed)
func runWithWezterm(cfg *PlaygroundConfig) {
	wez := terminal.NewWeztermLauncher()
	if !wez.IsAvailable() {
		fmt.Println("❌ wezterm is not installed!")
		fmt.Println("   Install from: https://wezterm.org/")
		os.Exit(1)
	}

	// Kill any previous crux session
	fmt.Println("🧹 Cleaning up previous session...")
	wez.KillPrevious()

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

	// Save pane IDs for cleanup on next run or Ctrl+C
	wez.SavePanes()

	// Start API server for MCP (MCP calls crux API, not wezterm)
	apiServer := api.NewServer(cfg.API.Port)
	tc := newWeztermTabController(wez)
	apiServer.SetTabController(tc)
	apiServer.SetStartOneHandler(func(serviceName string) (string, error) {
		var svc *ServiceConfig
		for i := range cfg.Services {
			if cfg.Services[i].Name == serviceName {
				svc = &cfg.Services[i]
				break
			}
		}
		if svc == nil {
			var names []string
			for _, s := range cfg.Services {
				names = append(names, s.Name)
			}
			return "", fmt.Errorf("service %q not found (available: %s)", serviceName, strings.Join(names, ", "))
		}
		cwd, _ := os.Getwd()
		workDir := svc.WorkDir
		if workDir == "" {
			workDir = cwd
		}
		if err := tc.SpawnTab(svc.Name, workDir, svc.Command, svc.ExpandArgs()); err != nil {
			return "", err
		}
		return fmt.Sprintf("Started %s in new tab", svc.Name), nil
	})
	apiServer.SetOnShutdown(func() {
		wez.Cleanup()
		os.Exit(0)
	})
	go apiServer.Start()

	fmt.Println()
	fmt.Println("✅ Services running in Wezterm tabs!")
	fmt.Printf("\n🌐 API: http://localhost:%d (MCP uses this)\n", cfg.API.Port)
	fmt.Println()
	fmt.Println("   Ctrl+C here = close all tabs and exit")
	fmt.Println("   Or just close this terminal - tabs stay running")
	fmt.Println()

	// After a short delay, check if any service already exited with error and warn loudly
	go func() {
		time.Sleep(15 * time.Second)
		names := make([]string, len(cfg.Services))
		for i := range cfg.Services {
			names[i] = cfg.Services[i].Name
		}
		failed := checkServiceFailures(names)
		if len(failed) > 0 {
			printFailedServicesWarning(names, failed)
		}
	}()

	// Handle Ctrl+C to cleanup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal
	<-sigChan
	fmt.Println("\n🛑 Shutting down...")
	wez.Cleanup()
	fmt.Println("✅ All tabs closed")
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

NOTE: crux is tested on macOS. May work on Linux (Wezterm is cross-platform).

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
   {\"mcpServers\":{\"crux\":{\"command\":\"${userHome}/bin/crux-mcp\",\"args\":[]}}}"

## Step 3: Analyze this project - USE EXISTING SCRIPTS, DO NOT GUESS

CRITICAL: Do NOT guess commands like "go run ./cmd/server" or "react-native run". 
You MUST find and use the project's actual run scripts. Read files, don't assume.

For each service, look for and USE (in order of priority):
1. scripts/ folder - start.sh, run.sh, dev.sh, scripts/start-backend.sh, etc.
2. Makefile - targets like "run", "start", "dev", "backend"
3. package.json "scripts" - "start", "dev", "run:ios", "run:android", "run:web", etc.
4. docker-compose.yml - services and how they're started
5. README or docs - often document the exact run commands

Backend: Look for scripts/run.sh, Make run, npm run dev - NOT "go run" unless that's what the script uses.
React Native: Almost always uses package.json scripts (npm run ios, yarn android, etc.) - NOT "react-native run" directly.
Flutter: Uses flutter run -d <device> but check if there's a wrapper script first.

WRONG: guessing "go run ./cmd/server" when project has scripts/start-backend.sh
RIGHT: command: ./scripts/start-backend.sh  workdir: ./
WRONG: guessing "react-native run" for a React Native app
RIGHT: command: npm  args: ["run", "ios"]  (or whatever package.json "scripts" actually defines)

Identify:
- Backend services (Go, Python, Node.js, etc.) - and which script starts each
- Mobile apps (Flutter, React Native) - and their actual run commands from package.json/scripts
- Web apps (React, Vue, etc.) - usually npm run dev / yarn dev

## Step 3a: Backend infrastructure dependencies
Check if project needs databases/caches. Ask user: "Are postgres/redis/mongo already running, or should crux start them?"

If crux should manage them, add to dependencies:
  - name: postgres
    check: pg_isready -h localhost -p 5432
    start: docker run -d --name crux-postgres -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:15
    timeout: 30
  - name: redis
    check: redis-cli ping
    start: docker run -d --name crux-redis -p 6379:6379 redis:7
    timeout: 15

## Step 3b: Mobile app dependencies (REQUIRED if project has mobile apps)
IMPORTANT: If project contains Flutter, React Native, iOS, or Android apps, you MUST add emulator dependencies.

For iOS apps - add this dependency:
  - name: ios-simulator
    check: xcrun simctl list devices | grep -q Booted
    start: open -a Simulator
    timeout: 60

For Android apps - first find AVD name with: emulator -list-avds
Then add this dependency (replace YOUR_AVD_NAME):
  - name: android-emulator
    check: adb devices | grep -q emulator
    start: nohup emulator -avd YOUR_AVD_NAME > /dev/null 2>&1 &
    timeout: 120

## Step 3c: Get device IDs for mobile services
For iOS: run "xcrun simctl list devices available"
- Use the UUID (e.g., "90266925-B62F-4741-A89E-EF11BFA0CC57")
- If no simulators, tell user to create one in Xcode

For Android: run "flutter devices" (after emulator starts)
- Use the device ID (e.g., "emulator-5554")
- If no emulators, tell user to create AVD in Android Studio

## Step 4: Create config.yaml
Create a config.yaml using the ACTUAL commands you found (from scripts, Makefile, package.json).
- One service entry per runnable component
- command/args = what the project's scripts use, e.g.:
  - Script: command: ./scripts/start-backend.sh  (or /bin/bash -c "./scripts/start-backend.sh")
  - Makefile: command: make  args: ["run"]
  - package.json: command: npm  args: ["run", "dev"]  (use the exact script name)
  - React Native: command: npm  args: ["run", "ios"]  (or "run:ios", whatever package.json has)
- Working directories relative to config.yaml (where the script/command runs from)
- terminal.app set to wezterm

Run: crux --help
to see the exact config format.

## Step 5: Run crux
Run: crux
This opens Wezterm with all services in separate tabs.

## Step 6: Use MCP to control services
Once running, you can use these MCP tools:

crux_status
  - No parameters
  - Returns: List of all tabs with numbers and titles

crux_send
  - tab: Tab number (1,2,3...) or partial name ("backend", "flutter")
  - text: Command to send ("r"=reload, "R"=restart, "q"=quit, or any text)

crux_logs
  - tab: Tab number or partial name
  - lines: Number of lines to get (default: 50)
  - Returns: Live terminal scrollback from the running tab

crux_focus
  - tab: Tab number or partial name
  - Action: Brings that tab to front in Wezterm

crux_start_one
  - service: Service name from config (e.g. backend, flutter-ios)
  - Action: Start that service in a new tab (or new window). Use when a service crashed.

crux_logfile
  - service: Service name ("backend") or "list" to see all services with logs
  - run: "latest" (default), "list" to show run history, or timestamp like "2024-02-11_143022"
  - lines: Number of lines from end (default: 100)
  - Returns: Log file content from /tmp/crux-logs/<service>/<timestamp>.log
  - USE WHEN: Tab crashed/closed, debugging failed startup, or viewing run history

Log structure:
  /tmp/crux-logs/<service>/<timestamp>.log (keeps last 10 runs per service)
  /tmp/crux-logs/<service>/latest.log -> symlink to most recent

If a command fails, the tab stays open with error message until Enter is pressed.

Examples:
- "What services are running?" -> crux_status
- "Hot reload Flutter" -> crux_send tab="flutter" text="r"
- "Show live backend logs" -> crux_logs tab="backend"
- "Backend crashed, what happened?" -> crux_logfile service="backend"
- "What services have logs?" -> crux_logfile service="list"
- "Show previous backend runs" -> crux_logfile service="backend" run="list"
- "Read this morning's run" -> crux_logfile service="backend" run="2024-02-11_090000"

## Notes
- NEVER guess: "go run", "react-native run", "python main.py" - always read scripts/package.json/Makefile first
- If a service has scripts/start.sh or similar, use it: command: ./scripts/start.sh
- For Python with venv, use the project's run script (./run.sh, Makefile, etc.)
- For React Native, use package.json scripts (npm run ios, yarn android) - not react-native CLI directly
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
    crux [options] [command]

OPTIONS:
    -c, --config FILE   Use specified config file (default: config.yaml)

COMMANDS:
    (none)      Start services from config file
    init        Generate example config.yaml
    prompt      Print AI agent prompt (for configuring crux via LLM)
    help        Show this help message
    version     Show version

    Examples:
        crux                        # Use config.yaml
        crux -c config.test.yaml    # Use test configuration
        crux --config=config.e2e.yaml
        crux start-one backend      # Start only one service in current Wezterm window (e.g. after crash)

CONFIGURATION:
    Create a config.yaml in your project root:

    # Dependencies are checked/started before services
    dependencies:
      - name: postgres
        check: pg_isready -h localhost -p 5432
        start: docker run -d --name crux-postgres -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:15
        timeout: 30

    services:
      - name: backend           # Display name for the tab
        command: go             # Executable to run
        args: ["run", "./cmd/server"]  # Command arguments (optional)
        workdir: ./backend      # Working directory (optional, relative to config)

      - name: flutter-ios
        command: flutter
        args: ["run", "-d", "iPhone 15 Pro"]
        workdir: ./mobile

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
      {"mcpServers":{"crux":{"command":"${userHome}/bin/crux-mcp","args":[]}}}

    Available MCP tools:
      crux_status   - List all terminal tabs
      crux_send     - Send commands to tabs (r=reload, R=restart, q=quit)
      crux_logs     - Get live terminal output from running tabs
      crux_focus    - Focus a specific tab
      crux_start_one - Start one service in new tab (same session, after crash)
      crux_logfile  - Read log history for crashed/closed tabs
                     Logs: /tmp/crux-logs/<service>/<timestamp>.log

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
# Requires: Wezterm, Docker (for dependencies)

# Dependencies are checked/started before services
# Each has: check (command to verify running), start (command to run if not), timeout (seconds)
dependencies:
  # PostgreSQL via Docker
  - name: postgres
    check: pg_isready -h localhost -p 5432
    start: docker run -d --name crux-postgres -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:15
    timeout: 30

  # Redis via Docker  
  - name: redis
    check: redis-cli ping
    start: docker run -d --name crux-redis -p 6379:6379 redis:7
    timeout: 15

  # iOS Simulator (uncomment if needed)
  # - name: ios-simulator
  #   check: xcrun simctl list devices | grep -q Booted
  #   start: open -a Simulator
  #   timeout: 60

  # Android Emulator (uncomment and set your AVD name)
  # - name: android-emulator
  #   check: adb devices | grep -q emulator
  #   start: nohup emulator -avd Pixel_7_API_34 > /dev/null 2>&1 &
  #   timeout: 120

services:
  # Backend service example
  - name: backend
    command: go
    args: ["run", "./cmd/server"]
    # workdir: ./backend  # optional working directory

  # Flutter iOS example (get UUID: xcrun simctl list devices)
  # - name: flutter-ios
  #   command: flutter
  #   args: ["run", "-d", "YOUR-IOS-SIMULATOR-UUID"]
  #   workdir: ./mobile

  # Flutter Android example (device ID from: flutter devices)
  # - name: flutter-android
  #   command: flutter
  #   args: ["run", "-d", "emulator-5554"]

  # Web app example
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
