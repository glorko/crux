package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mitchellh/go-homedir"
)

// Installer handles system-wide installation
type Installer struct {
	binaryPath string
	installDir string
}

// NewInstaller creates a new installer
func NewInstaller() (*Installer, error) {
	homeDir, err := homedir.Dir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Determine install directory
	installDir := filepath.Join(homeDir, "go", "bin")
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		installDir = filepath.Join(gopath, "bin")
	}

	return &Installer{
		installDir: installDir,
	}, nil
}

// Install installs the binary and updates PATH if needed
func (i *Installer) Install(sourcePath string) error {
	// Ensure install directory exists
	if err := os.MkdirAll(i.installDir, 0755); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	binaryName := filepath.Base(sourcePath)
	i.binaryPath = filepath.Join(i.installDir, binaryName)

	// Copy binary to install directory
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to read source binary: %w", err)
	}

	if err := os.WriteFile(i.binaryPath, data, 0755); err != nil {
		return fmt.Errorf("failed to write binary: %w", err)
	}

	// Check if PATH needs updating
	if !i.isInPath() {
		if err := i.updatePath(); err != nil {
			return fmt.Errorf("failed to update PATH: %w", err)
		}
	}

	return nil
}

// InstallViaGoInstall uses go install for installation
func (i *Installer) InstallViaGoInstall(modulePath string) error {
	cmd := exec.Command("go", "install", modulePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go install failed: %w", err)
	}

	// Check if PATH needs updating
	if !i.isInPath() {
		if err := i.updatePath(); err != nil {
			return fmt.Errorf("failed to update PATH: %w", err)
		}
	}

	return nil
}

// isInPath checks if install directory is in PATH
func (i *Installer) isInPath() bool {
	path := os.Getenv("PATH")
	paths := strings.Split(path, string(os.PathListSeparator))
	
	for _, p := range paths {
		if p == i.installDir {
			return true
		}
	}
	return false
}

// updatePath adds install directory to shell config
func (i *Installer) updatePath() error {
	homeDir, err := homedir.Dir()
	if err != nil {
		return err
	}

	// Detect shell
	shell := os.Getenv("SHELL")
	var configFile string
	var pathLine string

	if strings.Contains(shell, "zsh") {
		configFile = filepath.Join(homeDir, ".zshrc")
		pathLine = fmt.Sprintf(`export PATH="$PATH:%s"`, i.installDir)
	} else if strings.Contains(shell, "bash") {
		configFile = filepath.Join(homeDir, ".bashrc")
		pathLine = fmt.Sprintf(`export PATH="$PATH:%s"`, i.installDir)
	} else {
		// Default to .zshrc for macOS
		configFile = filepath.Join(homeDir, ".zshrc")
		pathLine = fmt.Sprintf(`export PATH="$PATH:%s"`, i.installDir)
	}

	// Check if already added
	configData, err := os.ReadFile(configFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if strings.Contains(string(configData), i.installDir) {
		return nil // Already added
	}

	// Append to config file
	f, err := os.OpenFile(configFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "\n# Added by crux installer\n%s\n", pathLine); err != nil {
		return fmt.Errorf("failed to write to config file: %w", err)
	}

	return nil
}

// GetInstallPath returns the installation path
func (i *Installer) GetInstallPath() string {
	return i.installDir
}

// GetBinaryPath returns the full binary path
func (i *Installer) GetBinaryPath() string {
	return i.binaryPath
}
