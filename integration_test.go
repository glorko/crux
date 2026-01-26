package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/glorko/crux/internal/config"
	"github.com/glorko/crux/internal/flutter"
	"github.com/glorko/crux/internal/process"
)

// Integration test that simulates the full workflow
func TestIntegration_FullWorkflow(t *testing.T) {
	// Setup test environment
	testDir := filepath.Join("testdata")
	
	// Build mock binaries
	mockBackend := filepath.Join(testDir, "mock_backend")
	mockFlutter := filepath.Join(testDir, "mock_flutter")

	buildBackend := exec.Command("go", "build", "-o", mockBackend, "mock_backend.go")
	buildBackend.Dir = testDir
	if err := buildBackend.Run(); err != nil {
		t.Fatalf("Failed to build mock backend: %v", err)
	}

	buildFlutter := exec.Command("go", "build", "-o", mockFlutter, "mock_flutter.go")
	buildFlutter.Dir = testDir
	if err := buildFlutter.Run(); err != nil {
		t.Fatalf("Failed to build mock Flutter: %v", err)
	}

	// Create test config
	cfg := &config.Config{
		Backend: config.BackendConfig{
			Path:        testDir,
			StartScript: "nonexistent.sh", // Will use go run fallback
			Env: map[string]string{
				"TEST_MODE": "true",
			},
		},
		Flutter: config.FlutterConfig{
			Path: testDir,
			Instances: []config.FlutterInstance{
				{
					Name:     "app1",
					DeviceID: "test-device-1",
					Platform: "ios",
				},
				{
					Name:     "app2",
					DeviceID: "test-device-2",
					Platform: "android",
				},
			},
		},
		Dependencies: config.DependenciesConfig{
			Postgres: config.PostgresConfig{
				Host:     "localhost",
				Port:     5433,
				Database: "test",
				User:     "test",
				Password: "test",
			},
			Redis: config.RedisConfig{
				Host: "localhost",
				Port: 6379,
			},
		},
	}

	// Create a simple main.go for backend testing
	mainGoPath := filepath.Join(testDir, "main.go")
	mainGoContent := `package main
import (
	"fmt"
	"time"
)
func main() {
	fmt.Println("Backend started")
	time.Sleep(2 * time.Second)
}
`
	if err := os.WriteFile(mainGoPath, []byte(mainGoContent), 0644); err != nil {
		t.Fatalf("Failed to create test main.go: %v", err)
	}
	defer os.Remove(mainGoPath)

	// Create process manager
	pm := process.NewProcessManager()

	// Test backend handler
	t.Run("Backend", func(t *testing.T) {
		bh := process.NewBackendHandler(pm, cfg)

		// Start
		if err := bh.Start(); err != nil {
			t.Fatalf("Failed to start backend: %v", err)
		}

		time.Sleep(300 * time.Millisecond)

		if !bh.IsRunning() {
			t.Error("Backend should be running")
		}

		// Restart
		if err := bh.Restart(); err != nil {
			t.Fatalf("Failed to restart backend: %v", err)
		}

		time.Sleep(300 * time.Millisecond)

		if !bh.IsRunning() {
			t.Error("Backend should be running after restart")
		}

		// Stop
		if err := bh.Stop(); err != nil {
			t.Fatalf("Failed to stop backend: %v", err)
		}
	})

	// Test Flutter runner
	t.Run("Flutter", func(t *testing.T) {
		// Create a wrapper script that uses our mock Flutter
		wrapperPath := filepath.Join(testDir, "flutter_wrapper.sh")
		wrapperContent := `#!/bin/bash
exec ` + mockFlutter + ` "$@"
`
		if err := os.WriteFile(wrapperPath, []byte(wrapperContent), 0755); err != nil {
			t.Fatalf("Failed to create wrapper: %v", err)
		}
		defer os.Remove(wrapperPath)

		// Temporarily override PATH to use our wrapper
		originalPath := os.Getenv("PATH")
		testPath := testDir + ":" + originalPath
		os.Setenv("PATH", testPath)
		defer os.Setenv("PATH", originalPath)

		// Create a symlink or wrapper for flutter command
		// For this test, we'll directly test the runner with mock processes
		runner := flutter.NewFlutterRunner(pm, cfg)

		// Start instances (will fail because flutter command doesn't exist, but we test the logic)
		// In a real scenario, we'd mock the flutter binary
		// For now, we test that the runner can be created and instances are registered
		instances := runner.GetInstances()
		if len(instances) != 2 {
			t.Errorf("Expected 2 instances, got %d", len(instances))
		}
	})

	// Cleanup
	pm.StopAll()
}

// Test command sending and receiving
func TestIntegration_CommandHandling(t *testing.T) {
	testDir := filepath.Join("testdata")
	mockFlutter := filepath.Join(testDir, "mock_flutter")

	// Build mock Flutter
	buildCmd := exec.Command("go", "build", "-o", mockFlutter, "mock_flutter.go")
	buildCmd.Dir = testDir
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build mock Flutter: %v", err)
	}

	pm := process.NewProcessManager()

	// Start mock Flutter process
	cmd := exec.Command(mockFlutter, "test-device")
	proc, err := pm.StartProcess("test-flutter", "test-flutter", cmd)
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Send hot reload command
	if err := proc.SendInput("r"); err != nil {
		t.Fatalf("Failed to send hot reload: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Send hot restart command
	if err := proc.SendInput("R"); err != nil {
		t.Fatalf("Failed to send hot restart: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Check output contains expected messages
	output := proc.GetOutput()
	foundReload := false
	foundRestart := false

	for _, line := range output {
		if contains(line, "hot reload") || contains(line, "Reloaded") {
			foundReload = true
		}
		if contains(line, "hot restart") || contains(line, "Restarted") {
			foundRestart = true
		}
	}

	if !foundReload {
		t.Error("Expected to find hot reload message in output")
	}

	// Cleanup
	proc.Stop()
	time.Sleep(100 * time.Millisecond)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		(len(s) > len(substr) && 
			(s[:len(substr)] == substr || 
			 s[len(s)-len(substr):] == substr ||
			 containsMiddle(s, substr))))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
