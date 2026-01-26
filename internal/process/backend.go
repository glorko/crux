package process

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/glorko/crux/internal/config"
)

// BackendHandler manages the backend process
type BackendHandler struct {
	manager *ProcessManager
	config  *config.Config
	procID  string
}

// NewBackendHandler creates a new backend handler
func NewBackendHandler(manager *ProcessManager, cfg *config.Config) *BackendHandler {
	return &BackendHandler{
		manager: manager,
		config:  cfg,
		procID:  "backend",
	}
}

// Start starts the backend process
func (bh *BackendHandler) Start() error {
	backendPath := bh.config.Backend.Path
	startScript := bh.config.Backend.StartScript

	// Check if start script exists
	scriptPath := filepath.Join(backendPath, startScript)
	if _, err := os.Stat(scriptPath); err == nil {
		// Use start script (works for both Go and Python)
		cmd := exec.Command("/bin/bash", scriptPath)
		cmd.Dir = backendPath
		
		// Set environment variables
		cmd.Env = os.Environ()
		for k, v := range bh.config.Backend.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}

		_, err := bh.manager.StartProcess(bh.procID, "Backend", cmd)
		return err
	}

	// Check if it's a Python project (has main.py or uvicorn command)
	mainPyPath := filepath.Join(backendPath, "services", "api", "app", "main.py")
	if _, err := os.Stat(mainPyPath); err == nil {
		// Python/FastAPI backend - run uvicorn
		apiPath := filepath.Join(backendPath, "services", "api")
		cmd := exec.Command("uvicorn", "app.main:app", "--reload", "--host", "0.0.0.0", "--port", "8000")
		cmd.Dir = apiPath
		
		// Set environment variables
		cmd.Env = os.Environ()
		for k, v := range bh.config.Backend.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}

		_, err := bh.manager.StartProcess(bh.procID, "Backend", cmd)
		return err
	}

	// Fallback: run go run directly (for Go backends)
	cmd := exec.Command("go", "run", "cmd/server/main.go")
	cmd.Dir = backendPath
	
	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range bh.config.Backend.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	_, err := bh.manager.StartProcess(bh.procID, "Backend", cmd)
	return err
}

// Restart stops, rebuilds, and starts the backend
func (bh *BackendHandler) Restart() error {
	// Stop the process if running
	proc, err := bh.manager.GetProcess(bh.procID)
	if err == nil && proc.IsRunning() {
		if err := proc.Stop(); err != nil {
			return fmt.Errorf("failed to stop backend: %w", err)
		}
		// Give it a moment to fully stop
		os.Stdout.WriteString("Stopping backend...\n")
	}

	backendPath := bh.config.Backend.Path

	// Check if Python backend (no build needed, just restart)
	mainPyPath := filepath.Join(backendPath, "services", "api", "app", "main.py")
	if _, err := os.Stat(mainPyPath); err == nil {
		// Python backend - no build needed, uvicorn --reload handles it
		os.Stdout.WriteString("🔄 Restarting Python backend (uvicorn auto-reloads)...\n")
	} else {
		// Go backend - build/compile
		os.Stdout.WriteString("Building backend...\n")
		buildCmd := exec.Command("go", "build", "-o", filepath.Join(backendPath, "server"), "cmd/server/main.go")
		buildCmd.Dir = backendPath
		buildCmd.Env = os.Environ()
		for k, v := range bh.config.Backend.Env {
			buildCmd.Env = append(buildCmd.Env, fmt.Sprintf("%s=%s", k, v))
		}

		buildOutput, err := buildCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to build backend: %w\nOutput: %s", err, string(buildOutput))
		}

		os.Stdout.WriteString("✅ Backend built successfully\n")
	}

	// Start the backend using the built binary, script, or uvicorn
	startScript := bh.config.Backend.StartScript
	scriptPath := filepath.Join(backendPath, startScript)
	
	var cmd *exec.Cmd
	if _, err := os.Stat(scriptPath); err == nil {
		// Use start script
		cmd = exec.Command("/bin/bash", scriptPath)
	} else {
		// Check if Python backend
		mainPyPath := filepath.Join(backendPath, "services", "api", "app", "main.py")
		if _, err := os.Stat(mainPyPath); err == nil {
			// Python/FastAPI backend
			apiPath := filepath.Join(backendPath, "services", "api")
			cmd = exec.Command("uvicorn", "app.main:app", "--reload", "--host", "0.0.0.0", "--port", "8000")
			cmd.Dir = apiPath
		} else {
			// Use go run (will use cached build)
			cmd = exec.Command("go", "run", "cmd/server/main.go")
		}
	}
	
	cmd.Dir = backendPath
	cmd.Env = os.Environ()
	for k, v := range bh.config.Backend.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	os.Stdout.WriteString("Starting backend...\n")
	_, err = bh.manager.StartProcess(bh.procID, "Backend", cmd)
	return err
}

// Stop stops the backend process
func (bh *BackendHandler) Stop() error {
	return bh.manager.StopProcess(bh.procID)
}

// IsRunning returns whether the backend is running
func (bh *BackendHandler) IsRunning() bool {
	proc, err := bh.manager.GetProcess(bh.procID)
	if err != nil {
		return false
	}
	return proc.IsRunning()
}
