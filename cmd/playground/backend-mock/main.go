package main

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// Mock Backend runner that simulates a Go/gRPC backend.
// Responds to commands via named pipe:
//   r -> Reload config
//   R -> Full restart (rebuild + restart)
//   s -> Show status
//   q -> Quit
//
// Usage: backend-mock <name> [port]
// Creates: /tmp/crux-<name>.pipe, /tmp/crux-<name>.pid, /tmp/crux-<name>.log

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
		fmt.Fprintf(os.Stderr, "Usage: %s <name> [port]\n", os.Args[0])
		os.Exit(1)
	}

	name := os.Args[1]
	port := "50051"
	if len(os.Args) >= 3 {
		port = os.Args[2]
	}

	startTime := time.Now()
	requestCount := 0
	restartCount := 0

	// Create log file
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

	output := &dualWriter{file: logF, stdout: os.Stdout}

	log := func(level, format string, args ...interface{}) {
		ts := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(output, "{\"time\":\"%s\",\"level\":\"%s\",\"msg\":\"%s\"}\n", ts, level, msg)
	}

	// Write PID file
	pidFile := fmt.Sprintf("/tmp/crux-%s.pid", name)
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write PID file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(pidFile)

	// Create named pipe
	pipePath := fmt.Sprintf("/tmp/crux-%s.pipe", name)
	os.Remove(pipePath)
	if err := syscall.Mkfifo(pipePath, 0666); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create FIFO: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(pipePath)

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	done := make(chan struct{})

	go func() {
		<-sigChan
		close(done)
	}()

	// Command reader - reads from both named pipe AND stdin
	cmdChan := make(chan string, 10)
	
	// Read from named pipe (for orchestrator/API control)
	go func() {
		for {
			pipe, err := os.OpenFile(pipePath, os.O_RDONLY, 0)
			if err != nil {
				select {
				case <-done:
					return
				default:
					time.Sleep(100 * time.Millisecond)
					continue
				}
			}

			scanner := bufio.NewScanner(pipe)
			for scanner.Scan() {
				select {
				case cmdChan <- scanner.Text():
				case <-done:
					pipe.Close()
					return
				}
			}
			pipe.Close()
		}
	}()

	// Read from stdin (for interactive keyboard input)
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				select {
				case <-done:
					return
				default:
					time.Sleep(100 * time.Millisecond)
					continue
				}
			}
			char := string(buf[0])
			// Only process recognized commands
			if char == "r" || char == "R" || char == "s" || char == "q" || char == "h" {
				select {
				case cmdChan <- char:
				case <-done:
					return
				}
			}
		}
	}()

	// Startup sequence
	printBanner(output, name)
	printStartup(output, log, name, port)

	// Simulate occasional requests
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			log("INFO", "Shutting down server...")
			log("INFO", "Server stopped")
			return

		case <-ticker.C:
			// Simulate incoming request
			requestCount++
			methods := []string{"GetUser", "CreatePost", "SyncData", "UpdateProfile", "ListItems"}
			method := methods[rand.Intn(len(methods))]
			latency := rand.Intn(50) + 5
			log("INFO", "gRPC call %s completed in %dms", method, latency)

		case cmd := <-cmdChan:
			switch cmd {
			case "r":
				log("INFO", "Reloading configuration...")
				time.Sleep(100 * time.Millisecond)
				log("INFO", "Configuration reloaded successfully")

			case "R":
				restartCount++
				log("INFO", "=== RESTART TRIGGERED ===")
				log("INFO", "Stopping server...")
				time.Sleep(200 * time.Millisecond)
				log("INFO", "Rebuilding application...")
				time.Sleep(500 * time.Millisecond)
				log("INFO", "Build completed in 1.2s")
				log("INFO", "Starting server on port %s...", port)
				time.Sleep(100 * time.Millisecond)
				log("INFO", "Server restarted successfully")
				log("INFO", "=== RESTART COMPLETE ===")

			case "s":
				uptime := time.Since(startTime).Round(time.Second)
				fmt.Fprintln(output)
				fmt.Fprintln(output, "═══════════════════════════════════════")
				fmt.Fprintf(output, " Backend Status: %s\n", name)
				fmt.Fprintln(output, "═══════════════════════════════════════")
				fmt.Fprintf(output, " Port:          %s\n", port)
				fmt.Fprintf(output, " PID:           %d\n", os.Getpid())
				fmt.Fprintf(output, " Uptime:        %s\n", uptime)
				fmt.Fprintf(output, " Requests:      %d\n", requestCount)
				fmt.Fprintf(output, " Restarts:      %d\n", restartCount)
				fmt.Fprintf(output, " State:         Running ✅\n")
				fmt.Fprintln(output, "═══════════════════════════════════════")
				fmt.Fprintln(output)

			case "q":
				log("INFO", "Quit command received")
				log("INFO", "Shutting down gracefully...")
				close(done)

			default:
				log("WARN", "Unknown command: %s", cmd)
			}
		}
	}
}

func printBanner(w io.Writer, name string) {
	fmt.Fprintln(w, `
  ╔════════════════════════════════════════════════════════════╗
  ║                 Backend Mock Server                        ║
  ║            Simulating Go/gRPC backend                      ║
  ╚════════════════════════════════════════════════════════════╝`)
}

func printStartup(w io.Writer, log func(string, string, ...interface{}), name, port string) {
	fmt.Fprintln(w)
	log("INFO", "Starting %s backend server...", name)
	log("INFO", "Loading configuration...")
	time.Sleep(50 * time.Millisecond)
	log("INFO", "Connecting to PostgreSQL...")
	time.Sleep(50 * time.Millisecond)
	log("INFO", "Database connection established")
	log("INFO", "Connecting to Redis...")
	time.Sleep(30 * time.Millisecond)
	log("INFO", "Redis connection established")
	log("INFO", "Initializing gRPC server on port %s...", port)
	time.Sleep(50 * time.Millisecond)

	fmt.Fprintln(w)
	fmt.Fprintf(w, "🚀 Backend server started on port %s\n", port)
	fmt.Fprintln(w)

	log("INFO", "Server ready to accept connections")
}
