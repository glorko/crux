package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// GhosttyLauncher implements TerminalLauncher for Ghostty terminal
type GhosttyLauncher struct{}

func (g *GhosttyLauncher) Name() string {
	return "Ghostty"
}

func (g *GhosttyLauncher) IsAvailable() bool {
	// Check if ghostty CLI is in PATH
	if commandExists("ghostty") {
		return true
	}
	// On macOS, check if Ghostty.app exists
	if runtime.GOOS == "darwin" {
		return appExists("Ghostty")
	}
	return false
}

func (g *GhosttyLauncher) Spawn(name string, workDir string, command string, args []string) error {
	// Build the full command to execute
	fullCmd := command
	if len(args) > 0 {
		fullCmd = command + " " + strings.Join(args, " ")
	}

	// On macOS, we must use 'open -na Ghostty.app --args ...'
	// because direct CLI execution is not supported
	if runtime.GOOS == "darwin" {
		return g.spawnMacOS(name, workDir, fullCmd)
	}

	// On Linux/other, use direct ghostty CLI
	return g.spawnDirect(name, workDir, fullCmd)
}

func (g *GhosttyLauncher) spawnMacOS(name string, workDir string, fullCmd string) error {
	// On macOS, Ghostty wraps commands with login, so we need to use
	// a shell wrapper to execute our command properly.
	// The trick is that 'open --args' passes all remaining args to the app,
	// and Ghostty's -e flag takes the rest of the args as the command.
	
	// Build the shell command
	shellCmd := fmt.Sprintf("exec %s", fullCmd)
	if workDir != "" {
		shellCmd = fmt.Sprintf("cd %q && exec %s", workDir, fullCmd)
	}

	// Pass to Ghostty: -e /bin/sh -c "the shell command"
	ghosttyArgs := []string{
		"-na", "Ghostty.app",
		"--args",
		fmt.Sprintf("--title=%s", name),
		"-e",
		"/bin/sh",
		"-c",
		shellCmd,
	}

	cmd := exec.Command("open", ghosttyArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open Ghostty.app: %w", err)
	}

	return nil
}

func (g *GhosttyLauncher) spawnDirect(name string, workDir string, fullCmd string) error {
	ghosttyArgs := []string{
		fmt.Sprintf("--title=%s", name),
	}

	if workDir != "" {
		ghosttyArgs = append(ghosttyArgs, fmt.Sprintf("--working-directory=%s", workDir))
	}

	ghosttyArgs = append(ghosttyArgs, "-e", fullCmd)

	cmd := exec.Command("ghostty", ghosttyArgs...)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ghostty: %w", err)
	}

	go cmd.Wait()
	return nil
}
