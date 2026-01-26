package flutter

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/glorko/crux/internal/config"
	"github.com/glorko/crux/internal/process"
)

// FlutterRunner manages Flutter app instances
type FlutterRunner struct {
	manager  *process.ProcessManager
	config   *config.Config
	instances map[string]*FlutterInstance
}

// FlutterInstance represents a running Flutter app instance
type FlutterInstance struct {
	Name     string
	DeviceID string
	Platform string
	AppPath  string // Optional: subdirectory for Flutter app
	ProcID   string
	Handler  *process.ManagedProcess
}

// NewFlutterRunner creates a new Flutter runner
func NewFlutterRunner(manager *process.ProcessManager, cfg *config.Config) *FlutterRunner {
	runner := &FlutterRunner{
		manager:   manager,
		config:    cfg,
		instances: make(map[string]*FlutterInstance),
	}

	// Initialize instances from config
	for _, instanceConfig := range cfg.Flutter.Instances {
		procID := fmt.Sprintf("flutter-%s", instanceConfig.Name)
		runner.instances[instanceConfig.Name] = &FlutterInstance{
			Name:     instanceConfig.Name,
			DeviceID: instanceConfig.DeviceID,
			Platform: instanceConfig.Platform,
			AppPath:  instanceConfig.AppPath,
			ProcID:   procID,
		}
	}

	return runner
}

// StartInstance starts a Flutter app instance
func (fr *FlutterRunner) StartInstance(name string) error {
	instance, exists := fr.instances[name]
	if !exists {
		return fmt.Errorf("Flutter instance %s not found in config", name)
	}

	// Check if already running
	if proc, err := fr.manager.GetProcess(instance.ProcID); err == nil && proc.IsRunning() {
		return fmt.Errorf("Flutter instance %s is already running", name)
	}

	// Start emulator/simulator if needed
	if err := fr.ensureDeviceReady(instance); err != nil {
		return fmt.Errorf("failed to prepare device: %w", err)
	}

	// Determine Flutter app path
	flutterPath := fr.config.Flutter.Path
	if instance.AppPath != "" {
		// Use app-specific path if provided
		flutterPath = fmt.Sprintf("%s/%s", flutterPath, instance.AppPath)
	}

	// Generate protobuf files if needed (before running)
	if err := fr.generateProtobuf(flutterPath); err != nil {
		fmt.Printf("⚠️  Warning: Failed to generate protobuf: %v\n", err)
		// Continue anyway - might already be generated
	}

	// Build flutter run command
	cmd := exec.Command("flutter", "run", "-d", instance.DeviceID)
	cmd.Dir = flutterPath
	cmd.Env = os.Environ()

	// Create descriptive name: "Flutter iOS" or "Flutter Android"
	var displayName string
	if instance.Platform == "ios" {
		displayName = "Flutter iOS"
	} else if instance.Platform == "android" {
		displayName = "Flutter Android"
	} else {
		// Fallback: capitalize first letter
		displayName = fmt.Sprintf("Flutter %s%s", strings.ToUpper(string(instance.Platform[0])), instance.Platform[1:])
	}
	
	proc, err := fr.manager.StartProcess(instance.ProcID, displayName, cmd)
	if err != nil {
		return fmt.Errorf("failed to start Flutter instance %s: %w", name, err)
	}

	// Update instance handler
	fr.instances[name].Handler = proc
	return nil
}

// StopInstance stops a Flutter app instance
func (fr *FlutterRunner) StopInstance(name string) error {
	instance, exists := fr.instances[name]
	if !exists {
		return fmt.Errorf("Flutter instance %s not found", name)
	}

	return fr.manager.StopProcess(instance.ProcID)
}

// HotReload triggers hot reload on all running Flutter instances
func (fr *FlutterRunner) HotReload() error {
	var lastErr error
	for name, instance := range fr.instances {
		if instance.Handler != nil && instance.Handler.IsRunning() {
			if err := instance.Handler.SendInput("r"); err != nil {
				lastErr = fmt.Errorf("failed to hot reload %s: %w", name, err)
			} else {
				fmt.Printf("🔄 Hot reload triggered for %s\n", name)
			}
		}
	}
	return lastErr
}

// HotRestart triggers hot restart on all running Flutter instances
func (fr *FlutterRunner) HotRestart() error {
	var lastErr error
	for name, instance := range fr.instances {
		if instance.Handler != nil && instance.Handler.IsRunning() {
			if err := instance.Handler.SendInput("R"); err != nil {
				lastErr = fmt.Errorf("failed to hot restart %s: %w", name, err)
			} else {
				fmt.Printf("🔄 Hot restart triggered for %s\n", name)
			}
		}
	}
	return lastErr
}

// GetInstances returns all Flutter instances
func (fr *FlutterRunner) GetInstances() map[string]*FlutterInstance {
	return fr.instances
}

// IsInstanceRunning returns whether a specific instance is running
func (fr *FlutterRunner) IsInstanceRunning(name string) bool {
	instance, exists := fr.instances[name]
	if !exists {
		return false
	}
	if instance.Handler == nil {
		return false
	}
	return instance.Handler.IsRunning()
}

// StopAll stops all Flutter instances
func (fr *FlutterRunner) StopAll() error {
	var lastErr error
	for name := range fr.instances {
		if err := fr.StopInstance(name); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// ensureDeviceReady starts emulator/simulator if needed
func (fr *FlutterRunner) ensureDeviceReady(instance *FlutterInstance) error {
	// Check if device is already available
	checkCmd := exec.Command("flutter", "devices")
	output, err := checkCmd.Output()
	if err == nil {
		if strings.Contains(string(output), instance.DeviceID) {
			// Device is already available
			return nil
		}
	}

	// Device not available, need to start it
	if instance.Platform == "android" {
		// For Android, try to start emulator by AVD name
		// If device_id looks like an emulator name (not emulator-5554 format)
		if !strings.HasPrefix(instance.DeviceID, "emulator-") {
			fmt.Printf("🚀 Starting Android emulator: %s\n", instance.DeviceID)
			launchCmd := exec.Command("flutter", "emulators", "--launch", instance.DeviceID)
			if err := launchCmd.Run(); err != nil {
				// Try alternative: use emulator command directly
				emulatorCmd := exec.Command("emulator", "-avd", instance.DeviceID)
				emulatorCmd.Start() // Start in background
				time.Sleep(5 * time.Second) // Wait for emulator to boot
			} else {
				time.Sleep(5 * time.Second) // Wait for emulator to boot
			}
		}
	} else if instance.Platform == "ios" {
		// For iOS, boot the simulator
		fmt.Printf("🚀 Starting iOS simulator: %s\n", instance.DeviceID)
		bootCmd := exec.Command("xcrun", "simctl", "boot", instance.DeviceID)
		if err := bootCmd.Run(); err != nil {
			// Simulator might already be booted, that's okay
			if !strings.Contains(err.Error(), "already booted") {
				return fmt.Errorf("failed to boot iOS simulator: %w", err)
			}
		}
		time.Sleep(2 * time.Second) // Wait for simulator to be ready
	}

	// Verify device is now available
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		checkCmd := exec.Command("flutter", "devices")
		output, err := checkCmd.Output()
		if err == nil && strings.Contains(string(output), instance.DeviceID) {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("device %s not available after starting", instance.DeviceID)
}

// generateProtobuf generates protobuf files for Flutter
func (fr *FlutterRunner) generateProtobuf(flutterPath string) error {
	// Check if generate script exists
	generateScript := fmt.Sprintf("%s/scripts/generate_protobuf.sh", flutterPath)
	if _, err := os.Stat(generateScript); err == nil {
		fmt.Printf("🔨 Generating protobuf files...\n")
		cmd := exec.Command("/bin/bash", generateScript)
		cmd.Dir = flutterPath
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("protobuf generation failed: %w\nOutput: %s", err, string(output))
		}
		fmt.Printf("✅ Protobuf files generated\n")
		return nil
	}

	// Try direct protoc if script doesn't exist
	protoDir := fmt.Sprintf("%s/proto", flutterPath)
	if _, err := os.Stat(protoDir); err == nil {
		fmt.Printf("🔨 Generating protobuf files (direct)...\n")
		// Try to find and generate proto files
		// This is a fallback - ideally the script should handle it
		return nil // Skip if no script, let Flutter handle it
	}

	return nil // No proto files to generate
}
