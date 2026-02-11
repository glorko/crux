package terminal

import (
	"fmt"
	"os/exec"
)

// KittyLauncher implements TerminalLauncher for Kitty terminal
type KittyLauncher struct{}

func (k *KittyLauncher) Name() string {
	return "Kitty"
}

func (k *KittyLauncher) IsAvailable() bool {
	return commandExists("kitty")
}

func (k *KittyLauncher) Spawn(name string, workDir string, command string, args []string) error {
	// Kitty CLI: kitty --title "Name" --directory /path command args...
	kittyArgs := []string{
		"--title", name,
	}

	if workDir != "" {
		kittyArgs = append(kittyArgs, "--directory", workDir)
	}

	// Add command and args
	kittyArgs = append(kittyArgs, command)
	kittyArgs = append(kittyArgs, args...)

	cmd := exec.Command("kitty", kittyArgs...)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start kitty: %w", err)
	}

	go cmd.Wait()

	return nil
}
