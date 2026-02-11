package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// PlaygroundConfig is the configuration for the playground
type PlaygroundConfig struct {
	Dependencies []DependencyConfig `yaml:"dependencies"`
	Services     []ServiceConfig    `yaml:"services"`
	API          APIConfig          `yaml:"api"`
	Tmux         TmuxConfig         `yaml:"tmux"`
	Terminal     TerminalConfig     `yaml:"terminal"`
}

// DependencyConfig defines a dependency to check/start before services
type DependencyConfig struct {
	Name    string `yaml:"name"`              // Display name (e.g., "postgres", "redis")
	Check   string `yaml:"check"`             // Command to check if running (exit 0 = running)
	Start   string `yaml:"start,omitempty"`   // Command to start if not running (optional)
	Timeout int    `yaml:"timeout,omitempty"` // Seconds to wait for check to pass after start (default: 30)
}

// TerminalConfig defines the terminal app to use
type TerminalConfig struct {
	App string `yaml:"app"` // ghostty, iterm2, terminal, wezterm, kitty
}

// ServiceConfig defines a service to run
type ServiceConfig struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	WorkDir string   `yaml:"workdir,omitempty"`
}

// APIConfig defines the API server configuration
type APIConfig struct {
	Port int `yaml:"port"`
}

// TmuxConfig defines tmux session configuration
type TmuxConfig struct {
	SessionName string `yaml:"session_name"`
}

// LoadPlaygroundConfig loads configuration from the specified file
func LoadPlaygroundConfig(configPath string) (*PlaygroundConfig, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%s not found", configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config.yaml: %w", err)
	}

	var cfg PlaygroundConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config.yaml: %w", err)
	}

	// Set defaults
	if cfg.API.Port == 0 {
		cfg.API.Port = 9876
	}
	if cfg.Tmux.SessionName == "" {
		cfg.Tmux.SessionName = "crux"
	}

	// Resolve command paths
	for i := range cfg.Services {
		cfg.Services[i].Command = resolveCommand(cfg.Services[i].Command)
	}

	return &cfg, nil
}

// resolveCommand resolves command path, checking ~/bin first
func resolveCommand(cmd string) string {
	// If it's already an absolute path and exists, use it
	if filepath.IsAbs(cmd) {
		if _, err := os.Stat(cmd); err == nil {
			return cmd
		}
	}

	// Extract just the binary name
	baseName := filepath.Base(cmd)

	// Check common locations
	home := os.Getenv("HOME")
	locations := []string{}

	if home != "" {
		locations = append(locations, filepath.Join(home, "bin", baseName))
	}
	locations = append(locations, "/tmp/"+baseName)
	locations = append(locations, "./"+baseName)

	gopath := os.Getenv("GOPATH")
	if gopath == "" && home != "" {
		gopath = filepath.Join(home, "go")
	}
	if gopath != "" {
		locations = append(locations, filepath.Join(gopath, "bin", baseName))
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	// Return original if not found (will fail later with clear error)
	return cmd
}

// ExpandArgs expands any environment variables in args
func (s *ServiceConfig) ExpandArgs() []string {
	expanded := make([]string, len(s.Args))
	for i, arg := range s.Args {
		expanded[i] = os.ExpandEnv(arg)
	}
	return expanded
}

// String returns a readable representation
func (c *PlaygroundConfig) String() string {
	var sb strings.Builder
	sb.WriteString("Playground Config:\n")
	sb.WriteString(fmt.Sprintf("  API Port: %d\n", c.API.Port))
	sb.WriteString(fmt.Sprintf("  Tmux Session: %s\n", c.Tmux.SessionName))
	sb.WriteString("  Services:\n")
	for _, svc := range c.Services {
		sb.WriteString(fmt.Sprintf("    - %s: %s %v\n", svc.Name, svc.Command, svc.Args))
	}
	return sb.String()
}

// CheckDependencies checks and starts all dependencies
// Returns nil if all dependencies are ready, error otherwise
func (c *PlaygroundConfig) CheckDependencies() error {
	if len(c.Dependencies) == 0 {
		return nil
	}

	fmt.Println("🔍 Checking dependencies...")
	fmt.Println()

	for _, dep := range c.Dependencies {
		if err := checkDependency(dep); err != nil {
			return err
		}
	}

	fmt.Println()
	return nil
}

// checkDependency checks a single dependency and starts it if needed
func checkDependency(dep DependencyConfig) error {
	timeout := dep.Timeout
	if timeout == 0 {
		timeout = 30 // Default 30 seconds
	}

	fmt.Printf("  📦 %s\n", dep.Name)
	fmt.Printf("     check: %s\n", dep.Check)

	// Run check command
	if runCheck(dep.Check) {
		fmt.Printf("     ✅ already running\n")
		return nil
	}

	// Not running - try to start
	if dep.Start == "" {
		fmt.Printf("     ❌ not running (no start command)\n")
		return fmt.Errorf("dependency %s is not running and no start command provided", dep.Name)
	}

	fmt.Printf("     start: %s\n", dep.Start)

	// Run start command and capture output
	startCmd := exec.Command("sh", "-c", dep.Start)
	output, err := startCmd.CombinedOutput()
	if err != nil {
		// For background commands (ending with &), Start() is used instead
		if strings.HasSuffix(strings.TrimSpace(dep.Start), "&") {
			startCmd = exec.Command("sh", "-c", dep.Start)
			if err := startCmd.Start(); err != nil {
				fmt.Printf("     ❌ failed to start: %v\n", err)
				return fmt.Errorf("failed to start %s: %v", dep.Name, err)
			}
		} else {
			fmt.Printf("     ❌ start failed: %v\n", err)
			if len(output) > 0 {
				// Show first few lines of output
				lines := strings.Split(string(output), "\n")
				for i, line := range lines {
					if i >= 3 {
						fmt.Printf("        ... (truncated)\n")
						break
					}
					if line != "" {
						fmt.Printf("        %s\n", line)
					}
				}
			}
			return fmt.Errorf("failed to start %s", dep.Name)
		}
	} else if len(output) > 0 {
		// Show container ID or similar short output
		outStr := strings.TrimSpace(string(output))
		if len(outStr) > 60 {
			outStr = outStr[:60] + "..."
		}
		if outStr != "" && !strings.Contains(outStr, "\n") {
			fmt.Printf("     → %s\n", outStr)
		}
	}

	fmt.Printf("     ⏳ waiting for ready (timeout: %ds)...\n", timeout)

	// Poll check command until timeout
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	attempts := 0
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		attempts++
		if runCheck(dep.Check) {
			fmt.Printf("     ✅ ready (after %ds)\n", attempts*2)
			return nil
		}
	}

	fmt.Printf("     ❌ timeout after %ds\n", timeout)
	return fmt.Errorf("dependency %s failed to become ready within %ds", dep.Name, timeout)
}

// runCheck runs a check command and returns true if it exits with 0
func runCheck(checkCmd string) bool {
	cmd := exec.Command("sh", "-c", checkCmd)
	cmd.Stdout = nil
	cmd.Stderr = nil
	err := cmd.Run()
	return err == nil
}
