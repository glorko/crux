package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// mock_flutter.go - A simple mock Flutter app for testing
// This simulates a Flutter app that:
// - Prints startup messages
// - Responds to hot reload (r) and hot restart (R) commands
// - Handles graceful shutdown

func main() {
	deviceID := os.Args[1]
	if deviceID == "" {
		deviceID = "test-device"
	}

	fmt.Printf("Launching lib/main.dart on %s in debug mode...\n", deviceID)
	fmt.Println("Flutter run key commands.")
	fmt.Println("r Hot reload.")
	fmt.Println("R Hot restart.")
	fmt.Println("Flutter DevTools, a Flutter debugger and profiler, is available at:")
	fmt.Println("http://127.0.0.1:9100?uri=http://127.0.0.1:54321/")
	fmt.Println("An Observatory debugger and profiler is available at:")
	fmt.Println("http://127.0.0.1:54321/")
	fmt.Println("The Flutter DevTools debugger and profiler is available at:")
	fmt.Println("http://127.0.0.1:9100?uri=http://127.0.0.1:54321/")

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Command handler
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			switch line {
			case "r":
				fmt.Println("Performing hot reload...")
				time.Sleep(50 * time.Millisecond)
				fmt.Println("Reloaded 1 of 1234 libraries")
				fmt.Println("Hot reload performed in 234ms")
			case "R":
				fmt.Println("Performing hot restart...")
				time.Sleep(100 * time.Millisecond)
				fmt.Println("Restarted application in 456ms")
			default:
				// Echo other commands
				if line != "" {
					fmt.Printf("Received: %s\n", line)
				}
			}
		}
	}()

	// Wait for signal
	<-sigChan
	fmt.Println("Application finished.")
}
