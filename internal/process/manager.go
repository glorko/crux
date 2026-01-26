package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// ProcessState represents the state of a managed process
type ProcessState string

const (
	StateStopped ProcessState = "stopped"
	StateStarting ProcessState = "starting"
	StateRunning  ProcessState = "running"
	StateStopping ProcessState = "stopping"
	StateError    ProcessState = "error"
)

// ManagedProcess represents a process managed by the process manager
type ManagedProcess struct {
	ID          string
	Name        string
	State       ProcessState
	Cmd         *exec.Cmd
	Cancel      context.CancelFunc
	Stdout      io.ReadCloser
	Stderr      io.ReadCloser
	Stdin       io.WriteCloser
	mu          sync.RWMutex
	outputLines []string
	maxLines    int
}

// ProcessManager manages multiple processes
type ProcessManager struct {
	processes map[string]*ManagedProcess
	mu        sync.RWMutex
}

// NewProcessManager creates a new process manager
func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		processes: make(map[string]*ManagedProcess),
	}
}

// StartProcess starts a new process
func (pm *ProcessManager) StartProcess(id, name string, cmd *exec.Cmd) (*ManagedProcess, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if process already exists
	if _, exists := pm.processes[id]; exists {
		return nil, fmt.Errorf("process %s already exists", id)
	}

	ctx, cancel := context.WithCancel(context.Background())
	newCmd := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	newCmd.Dir = cmd.Dir
	newCmd.Env = cmd.Env
	if len(newCmd.Env) == 0 {
		newCmd.Env = os.Environ()
	}
	cmd = newCmd

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		stdout.Close()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		stdout.Close()
		stderr.Close()
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	proc := &ManagedProcess{
		ID:          id,
		Name:        name,
		State:       StateStarting,
		Cmd:         cmd,
		Cancel:      cancel,
		Stdout:      stdout,
		Stderr:      stderr,
		Stdin:       stdin,
		outputLines: make([]string, 0),
		maxLines:    1000,
	}

	pm.processes[id] = proc

	// Start the command
	if err := cmd.Start(); err != nil {
		proc.State = StateError
		cancel()
		stdout.Close()
		stderr.Close()
		stdin.Close()
		delete(pm.processes, id)
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	proc.State = StateRunning

	// Start goroutines to capture output
	go proc.captureOutput(stdout, "stdout")
	go proc.captureOutput(stderr, "stderr")

	// Monitor process exit
	go proc.monitorExit()

	return proc, nil
}

// StopProcess stops a process
func (pm *ProcessManager) StopProcess(id string) error {
	pm.mu.RLock()
	proc, exists := pm.processes[id]
	pm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("process %s not found", id)
	}

	return proc.Stop()
}

// GetProcess returns a process by ID
func (pm *ProcessManager) GetProcess(id string) (*ManagedProcess, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	proc, exists := pm.processes[id]
	if !exists {
		return nil, fmt.Errorf("process %s not found", id)
	}

	return proc, nil
}

// ListProcesses returns all managed processes
func (pm *ProcessManager) ListProcesses() []*ManagedProcess {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	processes := make([]*ManagedProcess, 0, len(pm.processes))
	for _, proc := range pm.processes {
		processes = append(processes, proc)
	}

	return processes
}

// StopAll stops all managed processes
func (pm *ProcessManager) StopAll() error {
	pm.mu.RLock()
	processes := make([]*ManagedProcess, 0, len(pm.processes))
	for _, proc := range pm.processes {
		processes = append(processes, proc)
	}
	pm.mu.RUnlock()

	var lastErr error
	for _, proc := range processes {
		if err := proc.Stop(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// Stop stops the process
func (p *ManagedProcess) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.State == StateStopped || p.State == StateStopping {
		return nil
	}

	p.State = StateStopping

	// Cancel context to stop the process
	if p.Cancel != nil {
		p.Cancel()
	}

	// Wait for process to exit
	if p.Cmd != nil && p.Cmd.Process != nil {
		// Give it a moment to exit gracefully
		done := make(chan error, 1)
		go func() {
			done <- p.Cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited
		case <-time.After(5 * time.Second):
			// Force kill
			p.Cmd.Process.Kill()
			<-done
		}
	}

	if p.Stdout != nil {
		p.Stdout.Close()
	}
	if p.Stderr != nil {
		p.Stderr.Close()
	}
	if p.Stdin != nil {
		p.Stdin.Close()
	}

	p.State = StateStopped
	return nil
}

// SendInput sends input to the process stdin
func (p *ManagedProcess) SendInput(input string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.State != StateRunning || p.Stdin == nil {
		return fmt.Errorf("process is not running")
	}

	_, err := p.Stdin.Write([]byte(input + "\n"))
	return err
}

// GetOutput returns recent output lines
func (p *ManagedProcess) GetOutput() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	output := make([]string, len(p.outputLines))
	copy(output, p.outputLines)
	return output
}

// captureOutput captures output from a reader
func (p *ManagedProcess) captureOutput(reader io.ReadCloser, source string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		timestamp := time.Now().Format("15:04:05")
		
		// Format: [HH:MM:SS] [Service Name] log line
		formattedLine := fmt.Sprintf("[%s] [%s] %s", timestamp, p.Name, line)

		p.mu.Lock()
		p.outputLines = append(p.outputLines, formattedLine)
		if len(p.outputLines) > p.maxLines {
			p.outputLines = p.outputLines[1:]
		}
		p.mu.Unlock()

		// Print to console - all logs go to main thread stdout
		// Format makes it clear which service each log line belongs to
		fmt.Println(formattedLine)
	}
}

// monitorExit monitors the process and updates state on exit
func (p *ManagedProcess) monitorExit() {
	if p.Cmd != nil {
		p.Cmd.Wait()
		p.mu.Lock()
		if p.State == StateRunning {
			p.State = StateStopped
		}
		p.mu.Unlock()
	}
}

// IsRunning returns whether the process is running
func (p *ManagedProcess) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.State == StateRunning
}
