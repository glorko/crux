package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func buildMockBinary(t *testing.T, binaryName, sourceFile string) string {
	// Get absolute path to testdata
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	
	// Navigate from internal/process to project root
	projectRoot := filepath.Join(wd, "..", "..")
	testDir := filepath.Join(projectRoot, "testdata")
	mockBinary := filepath.Join(testDir, binaryName)
	
	// Build the mock binary if it doesn't exist or is older than source
	sourcePath := filepath.Join(testDir, sourceFile)
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("Source file not found: %v (looking in %s)", err, sourcePath)
	}
	
	binaryInfo, err := os.Stat(mockBinary)
	if err != nil || binaryInfo.ModTime().Before(sourceInfo.ModTime()) {
		buildCmd := exec.Command("go", "build", "-o", mockBinary, sourceFile)
		buildCmd.Dir = testDir
		if err := buildCmd.Run(); err != nil {
			t.Fatalf("Failed to build %s: %v", binaryName, err)
		}
	}
	
	// Return absolute path
	absPath, err := filepath.Abs(mockBinary)
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}
	return absPath
}

func TestProcessManager_StartStop(t *testing.T) {
	pm := NewProcessManager()

	// Build mock backend for testing
	mockBackend := buildMockBinary(t, "mock_backend", "mock_backend.go")

	// Start process
	cmd := exec.Command(mockBackend)
	proc, err := pm.StartProcess("test-backend", "test-backend", cmd)
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Check if running
	if !proc.IsRunning() {
		t.Error("Process should be running")
	}

	// Stop process
	if err := proc.Stop(); err != nil {
		t.Fatalf("Failed to stop process: %v", err)
	}

	// Give it a moment to stop
	time.Sleep(100 * time.Millisecond)

	if proc.IsRunning() {
		t.Error("Process should be stopped")
	}
}

func TestProcessManager_SendInput(t *testing.T) {
	pm := NewProcessManager()

	// Build mock Flutter for testing
	mockFlutter := buildMockBinary(t, "mock_flutter", "mock_flutter.go")

	// Start process
	cmd := exec.Command(mockFlutter, "test-device")
	proc, err := pm.StartProcess("test-flutter", "test-flutter", cmd)
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	// Give it a moment to start
	time.Sleep(200 * time.Millisecond)

	// Send hot reload command
	if err := proc.SendInput("r"); err != nil {
		t.Fatalf("Failed to send input: %v", err)
	}

	// Give it a moment to process
	time.Sleep(200 * time.Millisecond)

	// Send hot restart command
	if err := proc.SendInput("R"); err != nil {
		t.Fatalf("Failed to send input: %v", err)
	}

	// Give it a moment to process
	time.Sleep(200 * time.Millisecond)

	// Check output
	output := proc.GetOutput()
	if len(output) == 0 {
		t.Error("Expected output from process")
	}

	// Cleanup
	proc.Stop()
}

func TestProcessManager_StopAll(t *testing.T) {
	pm := NewProcessManager()

	mockBackend := buildMockBinary(t, "mock_backend", "mock_backend.go")
	mockFlutter := buildMockBinary(t, "mock_flutter", "mock_flutter.go")

	// Start multiple processes
	cmd1 := exec.Command(mockBackend)
	proc1, err := pm.StartProcess("backend", "backend", cmd1)
	if err != nil {
		t.Fatalf("Failed to start backend: %v", err)
	}

	cmd2 := exec.Command(mockFlutter, "device1")
	proc2, err := pm.StartProcess("flutter1", "flutter1", cmd2)
	if err != nil {
		t.Fatalf("Failed to start flutter1: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Stop all
	if err := pm.StopAll(); err != nil {
		t.Fatalf("Failed to stop all: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if proc1.IsRunning() || proc2.IsRunning() {
		t.Error("All processes should be stopped")
	}
}
