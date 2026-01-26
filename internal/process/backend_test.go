package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/glorko/crux/internal/config"
)

func TestBackendHandler_StartStop(t *testing.T) {
	pm := NewProcessManager()

	// Create a test config
	testDir := filepath.Join("..", "..", "testdata")
	cfg := &config.Config{
		Backend: config.BackendConfig{
			Path:        testDir,
			StartScript: "nonexistent.sh", // Will fall back to go run
			Env: map[string]string{
				"TEST_ENV": "test_value",
			},
		},
	}

	bh := NewBackendHandler(pm, cfg)

	// Build mock backend first
	mockBackend := filepath.Join(testDir, "mock_backend")
	buildCmd := exec.Command("go", "build", "-o", mockBackend, "mock_backend.go")
	buildCmd.Dir = testDir
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build mock backend: %v", err)
	}

	// Create a simple main.go in test directory for go run
	mainGoPath := filepath.Join(testDir, "main.go")
	mainGoContent := `package main
import "fmt"
func main() {
	fmt.Println("Mock backend started")
	select {}
}
`
	if err := os.WriteFile(mainGoPath, []byte(mainGoContent), 0644); err != nil {
		t.Fatalf("Failed to create test main.go: %v", err)
	}
	defer os.Remove(mainGoPath)

	// Start backend
	if err := bh.Start(); err != nil {
		t.Fatalf("Failed to start backend: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if !bh.IsRunning() {
		t.Error("Backend should be running")
	}

	// Stop backend
	if err := bh.Stop(); err != nil {
		t.Fatalf("Failed to stop backend: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if bh.IsRunning() {
		t.Error("Backend should be stopped")
	}
}

func TestBackendHandler_Restart(t *testing.T) {
	pm := NewProcessManager()

	testDir := filepath.Join("..", "..", "testdata")
	cfg := &config.Config{
		Backend: config.BackendConfig{
			Path:        testDir,
			StartScript: "nonexistent.sh",
			Env:         map[string]string{},
		},
	}

	bh := NewBackendHandler(pm, cfg)

	// Create test main.go
	mainGoPath := filepath.Join(testDir, "main.go")
	mainGoContent := `package main
import "fmt"
func main() {
	fmt.Println("Mock backend v1")
	select {}
}
`
	if err := os.WriteFile(mainGoPath, []byte(mainGoContent), 0644); err != nil {
		t.Fatalf("Failed to create test main.go: %v", err)
	}
	defer os.Remove(mainGoPath)

	// Start backend
	if err := bh.Start(); err != nil {
		t.Fatalf("Failed to start backend: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Update main.go to simulate code change
	mainGoContentV2 := `package main
import "fmt"
func main() {
	fmt.Println("Mock backend v2")
	select {}
}
`
	if err := os.WriteFile(mainGoPath, []byte(mainGoContentV2), 0644); err != nil {
		t.Fatalf("Failed to update test main.go: %v", err)
	}

	// Restart (should rebuild)
	if err := bh.Restart(); err != nil {
		t.Fatalf("Failed to restart backend: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	if !bh.IsRunning() {
		t.Error("Backend should be running after restart")
	}

	// Cleanup
	bh.Stop()
}
