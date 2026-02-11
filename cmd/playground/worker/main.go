package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// Worker is a test process that simulates a service for the playground.
// It responds to commands via a named pipe (FIFO):
//   r -> "Hot reload triggered!"
//   s -> status with uptime
//   q -> graceful exit
//
// Usage: worker <name>
// Creates: /tmp/crux-<name>.pipe and /tmp/crux-<name>.pid

// Logger that writes to both stdout and a log file
type dualWriter struct {
	file   *os.File
	stdout io.Writer
}

func (d *dualWriter) Write(p []byte) (n int, err error) {
	if d.file != nil {
		d.file.Write(p)
	}
	return d.stdout.Write(p)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <name>\n", os.Args[0])
		os.Exit(1)
	}

	name := os.Args[1]
	startTime := time.Now()

	// Create log file for tailing
	logFile := fmt.Sprintf("/tmp/crux-%s.log", name)
	logF, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not create log file: %v\n", err)
	}
	defer func() {
		if logF != nil {
			logF.Close()
		}
	}()

	// Create dual writer for stdout + file
	output := &dualWriter{file: logF, stdout: os.Stdout}
	log := func(format string, args ...interface{}) {
		timestamp := time.Now().Format("15:04:05")
		msg := fmt.Sprintf("[%s] [%s] %s\n", timestamp, name, fmt.Sprintf(format, args...))
		output.Write([]byte(msg))
	}

	// Write PID file so orchestrator can track us
	pidFile := fmt.Sprintf("/tmp/crux-%s.pid", name)
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write PID file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(pidFile)

	// Create named pipe (FIFO) for receiving commands
	pipePath := fmt.Sprintf("/tmp/crux-%s.pipe", name)
	
	// Remove existing pipe if any
	os.Remove(pipePath)
	
	if err := syscall.Mkfifo(pipePath, 0666); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create FIFO: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(pipePath)

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Channel to signal shutdown
	done := make(chan struct{})

	// Goroutine to handle signals
	go func() {
		sig := <-sigChan
		fmt.Printf("\n[%s] Received signal %v, shutting down...\n", name, sig)
		close(done)
	}()

	// Goroutine to read commands from FIFO
	cmdChan := make(chan string, 10)
	go func() {
		for {
			// Open pipe in read mode (blocks until writer connects)
			pipe, err := os.OpenFile(pipePath, os.O_RDONLY, 0)
			if err != nil {
				select {
				case <-done:
					return
				default:
					fmt.Printf("[%s] Error opening pipe: %v\n", name, err)
					time.Sleep(100 * time.Millisecond)
					continue
				}
			}

			scanner := bufio.NewScanner(pipe)
			for scanner.Scan() {
				cmd := scanner.Text()
				select {
				case cmdChan <- cmd:
				case <-done:
					pipe.Close()
					return
				}
			}
			pipe.Close()
		}
	}()

	// Print startup message
	fmt.Fprintln(output, "╔════════════════════════════════════════╗")
	fmt.Fprintf(output, "║  Worker: %-30s ║\n", name)
	fmt.Fprintln(output, "╚════════════════════════════════════════╝")
	log("Started (PID: %d)", os.Getpid())
	log("Listening on: %s", pipePath)
	log("Log file: %s", logFile)
	fmt.Fprintln(output)

	// Heartbeat ticker
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	heartbeat := 0
	for {
		select {
		case <-done:
			log("Goodbye!")
			return

		case <-ticker.C:
			heartbeat++
			uptime := time.Since(startTime).Round(time.Second)
			log("Heartbeat #%d (uptime: %s)", heartbeat, uptime)

		case cmd := <-cmdChan:
			switch cmd {
			case "r":
				fmt.Fprintln(output)
				log("⚡ Hot reload triggered!")
				log("Reloading configuration...")
				time.Sleep(200 * time.Millisecond)
				log("✅ Reload complete!")
				fmt.Fprintln(output)

			case "R":
				fmt.Fprintln(output)
				log("🔄 Hot restart triggered!")
				log("Stopping services...")
				time.Sleep(300 * time.Millisecond)
				log("Starting services...")
				time.Sleep(300 * time.Millisecond)
				log("✅ Restart complete!")
				fmt.Fprintln(output)

			case "s":
				uptime := time.Since(startTime).Round(time.Second)
				fmt.Fprintln(output)
				log("=== STATUS ===")
				log("PID: %d", os.Getpid())
				log("Uptime: %s", uptime)
				log("Heartbeats: %d", heartbeat)
				log("Log file: %s", logFile)
				log("State: Running")
				fmt.Fprintln(output)

			case "q":
				fmt.Fprintln(output)
				log("Quit command received")
				close(done)

			default:
				log("Unknown command: %q", cmd)
			}
		}
	}
}
