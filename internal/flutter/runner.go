package flutter

import (
	"context"
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
	AVDName  string // Android AVD name (for starting emulator)
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
			AVDName:  instanceConfig.AVDName,
			Platform: instanceConfig.Platform,
			AppPath:  instanceConfig.AppPath,
			ProcID:   procID,
		}
	}

	return runner
}

// StartInstance starts a Flutter app instance
func (fr *FlutterRunner) StartInstance(ctx context.Context, name string) error {
	instance, exists := fr.instances[name]
	if !exists {
		return fmt.Errorf("Flutter instance %s not found in config", name)
	}

	// Check if already running
	if proc, err := fr.manager.GetProcess(instance.ProcID); err == nil && proc.IsRunning() {
		return fmt.Errorf("Flutter instance %s is already running", name)
	}

	// Start emulator/simulator if needed
	if err := fr.ensureDeviceReady(ctx, instance); err != nil {
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
// This function is called in parallel for multiple instances, so device checks are independent
func (fr *FlutterRunner) ensureDeviceReady(ctx context.Context, instance *FlutterInstance) error {
	// For Android emulator IDs, we need to check if they're running
	// Don't fail immediately - give it a chance to appear
	if instance.Platform == "android" && strings.HasPrefix(instance.DeviceID, "emulator-") {
		// Emulator ID provided (e.g., "emulator-5554")
		// Check if it's already running - if not, we can't auto-start it (need AVD name)
		// But give it a few seconds to appear (might be starting up)
		maxQuickRetries := 5
		for i := 0; i < maxQuickRetries; i++ {
			select {
			case <-ctx.Done():
				return fmt.Errorf("device check cancelled: %w", ctx.Err())
			default:
			}
			
			checkCmd := exec.Command("flutter", "devices", "--machine")
			output, err := checkCmd.Output()
			if err == nil && strings.Contains(string(output), instance.DeviceID) {
				// Found it!
				return nil
			}
			
			// Also try adb directly as a fallback
			adbCmd := exec.Command("adb", "devices")
			adbOutput, err := adbCmd.Output()
			if err == nil && strings.Contains(string(adbOutput), instance.DeviceID) {
				// Found via adb, but wait a moment for Flutter to see it
				time.Sleep(1 * time.Second)
				// Check again with Flutter
				checkCmd := exec.Command("flutter", "devices", "--machine")
				output, err := checkCmd.Output()
				if err == nil && strings.Contains(string(output), instance.DeviceID) {
					return nil
				}
			}
			
			if i < maxQuickRetries-1 {
				time.Sleep(500 * time.Millisecond)
			}
		}
		
		// Not found after quick check - try to start the emulator using AVD name
		avdName := instance.AVDName
		if avdName == "" {
			// No AVD name provided - try to find one
			fmt.Printf("⚠️  Emulator %s not found and no AVD name provided. Attempting to find an Android emulator...\n", instance.DeviceID)
			
			// List available emulators using emulator command directly (more reliable)
			listCmd := exec.Command("emulator", "-list-avds")
			output, err := listCmd.Output()
			if err == nil && len(output) > 0 {
				// Parse AVD names (one per line)
				lines := strings.Split(strings.TrimSpace(string(output)), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line != "" {
						avdName = line
						break // Use first available AVD
					}
				}
			}
			
			if avdName == "" {
				return fmt.Errorf("emulator %s is not running and no AVD name provided in config. Please add 'avd_name' to your config", instance.DeviceID)
			}
		}
		
		// Start the emulator using AVD name
		fmt.Printf("🚀 Starting Android emulator: %s (will appear as %s)...\n", avdName, instance.DeviceID)
		launchCmd := exec.Command("flutter", "emulators", "--launch", avdName)
		if err := launchCmd.Start(); err != nil {
			// Try alternative: use emulator command directly
			emulatorCmd := exec.Command("emulator", "-avd", avdName)
			emulatorCmd.Start() // Start in background
		}
		time.Sleep(3 * time.Second)
		
		// Now continue with the normal wait logic below - it will detect any Android emulator
	}

	// Quick check if device is already available (non-blocking)
	checkCmd := exec.Command("flutter", "devices", "--machine")
	output, err := checkCmd.Output()
	if err == nil {
		// Check if device ID appears in the output
		if strings.Contains(string(output), instance.DeviceID) {
			// Device is already available
			return nil
		}
	}

	// Device not available, need to start it
	// Start device in background (non-blocking) so multiple devices can start in parallel
	if instance.Platform == "android" {
		// AVD name provided (e.g., "Pixel_9a") - we can start it
		fmt.Printf("🚀 Starting Android emulator: %s (this may take 30-60 seconds)...\n", instance.DeviceID)
		launchCmd := exec.Command("flutter", "emulators", "--launch", instance.DeviceID)
		// Start in background, don't wait
		if err := launchCmd.Start(); err != nil {
			// Try alternative: use emulator command directly
			emulatorCmd := exec.Command("emulator", "-avd", instance.DeviceID)
			if err := emulatorCmd.Start(); err != nil {
				return fmt.Errorf("failed to start Android emulator: %w", err)
			}
		}
		// Give it a moment to start (but don't block other instances)
		time.Sleep(3 * time.Second)
	} else if instance.Platform == "ios" {
		// For iOS, boot the simulator
		fmt.Printf("🚀 Starting iOS simulator: %s\n", instance.DeviceID)
		bootCmd := exec.Command("xcrun", "simctl", "boot", instance.DeviceID)
		// Run boot command (it's fast, but we don't wait for full boot)
		if err := bootCmd.Run(); err != nil {
			// Simulator might already be booted, that's okay
			if !strings.Contains(err.Error(), "already booted") && !strings.Contains(err.Error(), "Unable to boot") {
				// Only return error if it's not a "already booted" error
				// Some simulators return non-zero exit code even when booted
			}
		}
		// Give it a moment (but don't block other instances)
		time.Sleep(1 * time.Second)
	}

	// Verify device is now available (with retries, but each instance does this independently)
	// Android emulators take much longer to boot (30-60s), iOS is faster (5-10s)
	maxRetries := 60 // 60 retries
	retryInterval := 1 * time.Second // 1 second between retries = up to 60 seconds for Android
	
	if instance.Platform == "ios" {
		maxRetries = 15 // 15 seconds for iOS is usually enough
		retryInterval = 1 * time.Second
	}
	
	fmt.Printf("⏳ Waiting for %s device %s to be ready...\n", instance.Platform, instance.DeviceID)
	
	for i := 0; i < maxRetries; i++ {
		// Check if context is cancelled (Ctrl+C pressed)
		select {
		case <-ctx.Done():
			return fmt.Errorf("device readiness check cancelled: %w", ctx.Err())
		default:
			// Continue checking
		}
		
		checkCmd := exec.Command("flutter", "devices", "--machine")
		output, err := checkCmd.Output()
		if err == nil {
			outputStr := string(output)
			
			if instance.Platform == "android" {
				// For Android: handle both AVD names and emulator IDs
				if strings.HasPrefix(instance.DeviceID, "emulator-") {
					// Specific emulator ID provided - check for exact match
					if strings.Contains(outputStr, instance.DeviceID) {
						fmt.Printf("✅ Android emulator %s is ready\n", instance.DeviceID)
						return nil
					}
				} else {
					// AVD name provided - check if any Android emulator is available
					// Flutter will map the AVD name to the actual emulator ID when running
					if strings.Contains(outputStr, "emulator") && strings.Contains(outputStr, "android") {
						// Found an Android emulator - it's ready
						fmt.Printf("✅ Android emulator is ready (AVD: %s)\n", instance.DeviceID)
						return nil
					}
				}
			} else {
				// For iOS, check if device ID appears in the output
				if strings.Contains(outputStr, instance.DeviceID) {
					fmt.Printf("✅ iOS device %s is ready\n", instance.DeviceID)
					return nil
				}
			}
		}
		
		// Show progress every 5 seconds
		if i > 0 && i%5 == 0 {
			fmt.Printf("⏳ Still waiting for %s device %s... (%d/%d)\n", instance.Platform, instance.DeviceID, i, maxRetries)
		}
		
		// Use context-aware sleep
		select {
		case <-ctx.Done():
			return fmt.Errorf("device readiness check cancelled: %w", ctx.Err())
		case <-time.After(retryInterval):
			// Continue loop
		}
	}

	return fmt.Errorf("device %s not available after %d seconds - emulator may still be booting", instance.DeviceID, maxRetries)
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
