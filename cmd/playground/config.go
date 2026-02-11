package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// PlaygroundConfig is the configuration for the playground
type PlaygroundConfig struct {
	Services []ServiceConfig `yaml:"services"`
	API      APIConfig       `yaml:"api"`
	Tmux     TmuxConfig      `yaml:"tmux"`
	Terminal TerminalConfig  `yaml:"terminal"`
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

// LoadPlaygroundConfig loads configuration from config.yaml
func LoadPlaygroundConfig() (*PlaygroundConfig, error) {
	// Look for config.yaml in current directory
	configPath := "config.yaml"
	
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config.yaml not found in current directory")
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
