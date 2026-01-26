package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
	"github.com/glorko/crux/internal/flutter"
	"github.com/glorko/crux/internal/process"
	"github.com/glorko/crux/internal/webapp"
)

// readSingleChar reads a single character from stdin without requiring Enter
func readSingleChar() (string, error) {
	// Check if stdin is a terminal
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		// Fallback to scanner if not a terminal
		return "", fmt.Errorf("not a terminal")
	}

	// Save current terminal state
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer term.Restore(fd, oldState)

	// Read a single byte
	var buf [1]byte
	n, err := os.Stdin.Read(buf[:])
	if err != nil || n == 0 {
		return "", fmt.Errorf("failed to read input")
	}

	char := string(buf[0])
	// Handle Enter key (newline or carriage return)
	if char == "\n" || char == "\r" {
		return "", fmt.Errorf("empty input")
	}

	return char, nil
}

// Menu handles interactive menu and runtime controls
type Menu struct {
	backendHandler *process.BackendHandler
	flutterRunner  *flutter.FlutterRunner
	webAppRunner   *webapp.WebAppRunner
	scanner        *bufio.Scanner
}

// NewMenu creates a new menu instance
func NewMenu(backendHandler *process.BackendHandler, flutterRunner *flutter.FlutterRunner, webAppRunner *webapp.WebAppRunner) *Menu {
	return &Menu{
		backendHandler: backendHandler,
		flutterRunner: flutterRunner,
		webAppRunner: webAppRunner,
		scanner:        bufio.NewScanner(os.Stdin),
	}
}

// ShowStartupMenu displays the initial menu for selecting what to start
func (m *Menu) ShowStartupMenu() ([]string, error) {
	fmt.Println("\n╔════════════════════════════════════════╗")
	fmt.Println("║     Crux - Dev Environment Controller  ║")
	fmt.Println("║     ⚠️  Local Development Only          ║")
	fmt.Println("╚════════════════════════════════════════╝")
	fmt.Println("\nWhat would you like to start?")
	
	instances := m.flutterRunner.GetInstances()
	webAppInstances := m.webAppRunner.GetInstances()
	optionNum := 1
	
	fmt.Printf("  [%d] Backend\n", optionNum)
	optionNum++
	
	instanceNames := make([]string, 0, len(instances))
	for name := range instances {
		instanceNames = append(instanceNames, name)
		fmt.Printf("  [%d] Flutter %s\n", optionNum, name)
		optionNum++
	}
	
	webAppNames := make([]string, 0, len(webAppInstances))
	for _, name := range webAppInstances {
		webAppNames = append(webAppNames, name)
		fmt.Printf("  [%d] WebApp %s\n", optionNum, name)
		optionNum++
	}
	
	allOptionNum := optionNum
	fmt.Printf("  [%d] All (Backend + All Flutter Apps + All WebApps)\n", allOptionNum)
	fmt.Println("  [q] Quit")
	fmt.Print("\nSelect option: ")

	// Try to read single character without requiring Enter
	input, err := readSingleChar()
	if err != nil {
		// Fallback to scanner if single char read fails (e.g., not a terminal)
		if !m.scanner.Scan() {
			return nil, fmt.Errorf("failed to read input")
		}
		input = strings.TrimSpace(m.scanner.Text())
	} else {
		// Echo the character and newline for better UX
		fmt.Printf("%s\n", input)
	}
	
	selected := []string{}

	if input == "q" || input == "Q" {
		return nil, nil
	}

	// Parse numeric input (single digit)
	var choice int
	if _, err := fmt.Sscanf(input, "%d", &choice); err != nil {
		return nil, fmt.Errorf("invalid option: %s", input)
	}

	if choice == 1 {
		selected = append(selected, "backend")
	} else if choice >= 2 && choice <= 1+len(instanceNames) {
		instanceIndex := choice - 2
		selected = append(selected, instanceNames[instanceIndex])
	} else if choice > 1+len(instanceNames) && choice <= 1+len(instanceNames)+len(webAppNames) {
		webAppIndex := choice - 2 - len(instanceNames)
		selected = append(selected, webAppNames[webAppIndex])
	} else if choice == allOptionNum {
		selected = append(selected, "backend")
		selected = append(selected, instanceNames...)
		selected = append(selected, webAppNames...)
	} else {
		return nil, fmt.Errorf("invalid option: %d", choice)
	}

	return selected, nil
}

// ShowRuntimeMenu displays runtime controls and handles user input
func (m *Menu) ShowRuntimeMenu() error {
	fmt.Println("\n╔════════════════════════════════════════╗")
	fmt.Println("║         Runtime Controls                ║")
	fmt.Println("╚════════════════════════════════════════╝")
	m.PrintStatus()
	fmt.Println("\nCommands:")
	fmt.Println("  [r]     - Hot reload Flutter (all instances)")
	fmt.Println("  [R]     - Hot restart Flutter (all instances)")
	fmt.Println("  [r b]   - Restart backend (rebuild + start)")
	fmt.Println("  [q]     - Quit all")
	fmt.Print("\n> ")

	if !m.scanner.Scan() {
		return fmt.Errorf("failed to read input")
	}

	input := strings.TrimSpace(m.scanner.Text())

	switch input {
	case "r":
		if err := m.flutterRunner.HotReload(); err != nil {
			fmt.Printf("❌ Error: %v\n", err)
		}
	case "R":
		if err := m.flutterRunner.HotRestart(); err != nil {
			fmt.Printf("❌ Error: %v\n", err)
		}
	case "r b", "rb":
		fmt.Println("🔄 Restarting backend (rebuilding)...")
		if err := m.backendHandler.Restart(); err != nil {
			fmt.Printf("❌ Error restarting backend: %v\n", err)
		} else {
			fmt.Println("✅ Backend restarted successfully")
		}
	case "q", "Q":
		return fmt.Errorf("quit requested")
	default:
		fmt.Printf("Unknown command: %s\n", input)
	}

	return nil
}

// PrintStatus prints the current status of all processes
func (m *Menu) PrintStatus() {
	fmt.Println("\nStatus:")
	
	// Backend status
	if m.backendHandler.IsRunning() {
		fmt.Println("  ✅ Backend: Running")
	} else {
		fmt.Println("  ⏸️  Backend: Stopped")
	}

	// Flutter instances status
	instances := m.flutterRunner.GetInstances()
	for name, instance := range instances {
		if m.flutterRunner.IsInstanceRunning(name) {
			fmt.Printf("  ✅ Flutter %s (%s): Running\n", name, instance.DeviceID)
		} else {
			fmt.Printf("  ⏸️  Flutter %s (%s): Stopped\n", name, instance.DeviceID)
		}
	}

	// WebApp instances status
	webAppInstances := m.webAppRunner.GetInstances()
	for _, name := range webAppInstances {
		if m.webAppRunner.IsRunning(name) {
			fmt.Printf("  ✅ WebApp %s: Running\n", name)
		} else {
			fmt.Printf("  ⏸️  WebApp %s: Stopped\n", name)
		}
	}
}

// RunInteractiveLoop runs the interactive loop for runtime controls
// Processes run in background threads, this just handles commands
func (m *Menu) RunInteractiveLoop() error {
	for {
		if err := m.ShowRuntimeMenu(); err != nil {
			if err.Error() == "quit requested" {
				return nil
			}
			// Continue on other errors
		}
	}
}
