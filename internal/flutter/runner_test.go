package flutter

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/glorko/crux/internal/config"
	"github.com/glorko/crux/internal/process"
)

func buildMockBinary(t *testing.T, binaryName, sourceFile string) string {
	// Get absolute path to testdata
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	
	// Navigate from internal/flutter to project root
	projectRoot := filepath.Join(wd, "..", "..")
	testDir := filepath.Join(projectRoot, "testdata")
	mockBinary := filepath.Join(testDir, binaryName)
	
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

func TestFlutterRunner_StartStop(t *testing.T) {
	pm := process.NewProcessManager()

	testDir := filepath.Join("..", "..", "testdata")
	cfg := &config.Config{
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
	}

	runner := NewFlutterRunner(pm, cfg)

	// Build mock Flutter
	mockFlutter := buildMockBinary(t, "mock_flutter", "mock_flutter.go")

	// Override flutter command for testing by creating a wrapper script
	// For this test, we'll directly test with the mock
	// In real usage, we'd need to mock the flutter binary

	// Start instance
	if err := runner.StartInstance("app1"); err != nil {
		t.Fatalf("Failed to start app1: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if !runner.IsInstanceRunning("app1") {
		t.Error("app1 should be running")
	}

	// Stop instance
	if err := runner.StopInstance("app1"); err != nil {
		t.Fatalf("Failed to stop app1: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if runner.IsInstanceRunning("app1") {
		t.Error("app1 should be stopped")
	}
}

func TestFlutterRunner_HotReload(t *testing.T) {
	pm := process.NewProcessManager()

	testDir := filepath.Join("..", "..", "testdata")
	cfg := &config.Config{
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
	}

	runner := NewFlutterRunner(pm, cfg)

	// Build mock Flutter
	mockFlutter := buildMockBinary(t, "mock_flutter", "mock_flutter.go")

	// We need to create a wrapper that intercepts flutter commands
	// For this test, we'll use a simpler approach: test the command sending directly
	// by starting processes manually and testing hot reload

	// Start both instances
	if err := runner.StartInstance("app1"); err != nil {
		t.Fatalf("Failed to start app1: %v", err)
	}
	if err := runner.StartInstance("app2"); err != nil {
		t.Fatalf("Failed to start app2: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Test hot reload
	if err := runner.HotReload(); err != nil {
		t.Fatalf("Failed to hot reload: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Test hot restart
	if err := runner.HotRestart(); err != nil {
		t.Fatalf("Failed to hot restart: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Cleanup
	runner.StopAll()
}

func TestFlutterRunner_MultipleInstances(t *testing.T) {
	pm := process.NewProcessManager()

	testDir := filepath.Join("..", "..", "testdata")
	cfg := &config.Config{
		Flutter: config.FlutterConfig{
			Path: testDir,
			Instances: []config.FlutterInstance{
				{
					Name:     "app1",
					DeviceID: "device1",
					Platform: "ios",
				},
				{
					Name:     "app2",
					DeviceID: "device2",
					Platform: "android",
				},
			},
		},
	}

	runner := NewFlutterRunner(pm, cfg)

	instances := runner.GetInstances()
	if len(instances) != 2 {
		t.Errorf("Expected 2 instances, got %d", len(instances))
	}

	if instances["app1"] == nil || instances["app2"] == nil {
		t.Error("Expected app1 and app2 instances")
	}

	if instances["app1"].DeviceID != "device1" {
		t.Errorf("Expected device1, got %s", instances["app1"].DeviceID)
	}
}
