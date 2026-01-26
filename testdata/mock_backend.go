package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// mock_backend.go - A simple mock backend for testing
// This simulates a Go backend that:
// - Prints startup messages
// - Responds to stdin commands
// - Handles graceful shutdown

func main() {
	fmt.Println("Starting mock backend server...")
	fmt.Println("Server listening on port 50051")
	fmt.Println("Ready to accept connections")

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Simple command loop
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				break
			}
			if buf[0] == 'r' || buf[0] == 'R' {
				fmt.Println("Received reload command")
			}
		}
	}()

	// Wait for signal
	<-sigChan
	fmt.Println("Shutting down gracefully...")
	time.Sleep(100 * time.Millisecond)
	fmt.Println("Server stopped")
}
