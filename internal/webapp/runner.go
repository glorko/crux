package webapp

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/glorko/crux/internal/config"
	"github.com/glorko/crux/internal/process"
)

// WebAppRunner manages web app instances
type WebAppRunner struct {
	config        *config.Config
	manager       *process.ProcessManager
	instances     map[string]*WebAppInstance
	processIDs    map[string]string
}

// WebAppInstance represents a running web app instance
type WebAppInstance struct {
	Config *config.WebAppInstance
}

// NewWebAppRunner creates a new web app runner
func NewWebAppRunner(cfg *config.Config, manager *process.ProcessManager) *WebAppRunner {
	return &WebAppRunner{
		config:     cfg,
		manager:    manager,
		instances:  make(map[string]*WebAppInstance),
		processIDs: make(map[string]string),
	}
}

// StartInstance starts a web app instance
func (wr *WebAppRunner) StartInstance(name string) error {
	// Find the instance config
	var instanceConfig *config.WebAppInstance
	for i := range wr.config.WebApps.Instances {
		if wr.config.WebApps.Instances[i].Name == name {
			instanceConfig = &wr.config.WebApps.Instances[i]
			break
		}
	}

	if instanceConfig == nil {
		return fmt.Errorf("web app instance '%s' not found in config", name)
	}

	appPath := instanceConfig.Path
	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		return fmt.Errorf("web app path does not exist: %s", appPath)
	}

	// Parse start script (e.g., "npm run dev" -> ["npm", "run", "dev"])
	scriptParts := strings.Fields(instanceConfig.StartScript)
	if len(scriptParts) == 0 {
		return fmt.Errorf("start_script is required for web app '%s'", name)
	}

	// Create command
	cmd := exec.Command(scriptParts[0], scriptParts[1:]...)
	cmd.Dir = appPath

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range instanceConfig.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Generate process ID
	procID := fmt.Sprintf("webapp-%s", name)

	// Start the process
	_, err := wr.manager.StartProcess(procID, fmt.Sprintf("WebApp %s", name), cmd)
	if err != nil {
		return fmt.Errorf("failed to start web app '%s': %w", name, err)
	}

	wr.processIDs[name] = procID
	wr.instances[name] = &WebAppInstance{Config: instanceConfig}

	return nil
}

// StopInstance stops a web app instance
func (wr *WebAppRunner) StopInstance(name string) error {
	procID, exists := wr.processIDs[name]
	if !exists {
		return fmt.Errorf("web app instance '%s' is not running", name)
	}

	proc, err := wr.manager.GetProcess(procID)
	if err != nil {
		return fmt.Errorf("failed to get process for '%s': %w", name, err)
	}

	if err := proc.Stop(); err != nil {
		return fmt.Errorf("failed to stop web app '%s': %w", name, err)
	}

	delete(wr.processIDs, name)
	delete(wr.instances, name)

	return nil
}

// GetInstances returns all configured web app instances
func (wr *WebAppRunner) GetInstances() []string {
	var names []string
	for _, instance := range wr.config.WebApps.Instances {
		names = append(names, instance.Name)
	}
	return names
}

// IsRunning checks if a web app instance is running
func (wr *WebAppRunner) IsRunning(name string) bool {
	procID, exists := wr.processIDs[name]
	if !exists {
		return false
	}

	proc, err := wr.manager.GetProcess(procID)
	if err != nil {
		return false
	}

	return proc.IsRunning()
}
