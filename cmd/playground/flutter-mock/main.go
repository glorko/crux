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

// Mock Flutter runner that simulates realistic Flutter app output.
// Responds to commands via named pipe:
//   r -> Hot reload (like pressing 'r' in Flutter)
//   R -> Hot restart (like pressing 'R' in Flutter)
//   s -> Show status
//   q -> Quit
//
// Usage: flutter-mock <name> [device]
// Creates: /tmp/crux-<name>.pipe, /tmp/crux-<name>.pid, /tmp/crux-<name>.log

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
		fmt.Fprintf(os.Stderr, "Usage: %s <name> [device]\n", os.Args[0])
		os.Exit(1)
	}

	name := os.Args[1]
	device := "iPhone 15 Pro"
	if len(os.Args) >= 3 {
		device = os.Args[2]
	}

	startTime := time.Now()
	reloadCount := 0
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
		// Read single characters from stdin
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

	// Simulate Flutter startup
	printFlutterBanner(output)
	printStartup(output, name, device)

	// Periodic "frame" simulation
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	frameCount := 0
	for {
		select {
		case <-done:
			printShutdown(output, name)
			return

		case <-ticker.C:
			frameCount++
			// Simulate occasional rebuild/sync messages
			if frameCount%3 == 0 {
				fmt.Fprintf(output, "flutter: %d widgets rebuilt\n", rand.Intn(50)+10)
			}

		case cmd := <-cmdChan:
			switch cmd {
			case "r":
				reloadCount++
				printHotReload(output, reloadCount)

			case "R":
				restartCount++
				printHotRestart(output, restartCount)

			case "s":
				printStatus(output, name, device, startTime, reloadCount, restartCount)

			case "h":
				printHelp(output)

			case "q":
				printShutdown(output, name)
				return

			default:
				// Ignore unknown commands silently in interactive mode
			}
		}
	}
}

func printFlutterBanner(w io.Writer) {
	fmt.Fprintln(w, `
  ╔════════════════════════════════════════════════════════════╗
  ║                 Flutter Mock Runner                        ║
  ║            Simulating Flutter app execution                ║
  ╚════════════════════════════════════════════════════════════╝`)
}

func printStartup(w io.Writer, name, device string) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Launching lib/main.dart on %s in debug mode...\n", device)
	time.Sleep(200 * time.Millisecond)

	fmt.Fprintln(w, "Running Xcode build...")
	time.Sleep(100 * time.Millisecond)
	fmt.Fprintln(w, " └─Compiling, linking and signing...")
	time.Sleep(100 * time.Millisecond)

	fmt.Fprintln(w, "Xcode build done.                                           8.2s")
	fmt.Fprintf(w, "Installing and launching...                                  3.1s\n")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "🚀 %s app started!\n", name)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Flutter run key commands.")
	fmt.Fprintln(w, "r Hot reload. 🔥🔥🔥")
	fmt.Fprintln(w, "R Hot restart.")
	fmt.Fprintln(w, "h List all available interactive commands.")
	fmt.Fprintln(w, "d Detach (terminate \"flutter run\" but leave application running).")
	fmt.Fprintln(w, "c Clear the screen")
	fmt.Fprintln(w, "q Quit (terminate the application on the device).")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "An Observatory debugger and profiler on %s is available at: http://127.0.0.1:5%d/\n",
		device, 4000+rand.Intn(999))
	fmt.Fprintln(w, "The Flutter DevTools debugger and profiler is available at: http://127.0.0.1:9100")
	fmt.Fprintln(w)
}

func printHotReload(w io.Writer, count int) {
	reloadTime := 150 + rand.Intn(300)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Performing hot reload...")
	time.Sleep(time.Duration(reloadTime) * time.Millisecond)

	fmt.Fprintf(w, "Reloaded %d of %d libraries in %dms (compile: %d ms, reload: %d ms).\n",
		rand.Intn(5)+1, rand.Intn(10)+5, reloadTime, reloadTime/3, reloadTime*2/3)
	fmt.Fprintln(w, "🔥 Hot reload performed!")
	fmt.Fprintln(w)
}

func printHotRestart(w io.Writer, count int) {
	restartTime := 800 + rand.Intn(1200)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Performing hot restart...")
	time.Sleep(time.Duration(restartTime) * time.Millisecond)

	fmt.Fprintf(w, "Restarted application in %dms.\n", restartTime)
	fmt.Fprintln(w, "🔄 Hot restart complete!")
	fmt.Fprintln(w)
}

func printStatus(w io.Writer, name, device string, startTime time.Time, reloads, restarts int) {
	uptime := time.Since(startTime).Round(time.Second)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "═══════════════════════════════════════")
	fmt.Fprintf(w, " Flutter App Status: %s\n", name)
	fmt.Fprintln(w, "═══════════════════════════════════════")
	fmt.Fprintf(w, " Device:      %s\n", device)
	fmt.Fprintf(w, " PID:         %d\n", os.Getpid())
	fmt.Fprintf(w, " Uptime:      %s\n", uptime)
	fmt.Fprintf(w, " Hot reloads: %d\n", reloads)
	fmt.Fprintf(w, " Restarts:    %d\n", restarts)
	fmt.Fprintf(w, " State:       Running ✅\n")
	fmt.Fprintln(w, "═══════════════════════════════════════")
	fmt.Fprintln(w)
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flutter run key commands:")
	fmt.Fprintln(w, "  r  Hot reload 🔥")
	fmt.Fprintln(w, "  R  Hot restart")
	fmt.Fprintln(w, "  s  Show status")
	fmt.Fprintln(w, "  h  Show this help")
	fmt.Fprintln(w, "  q  Quit")
	fmt.Fprintln(w)
}

func printShutdown(w io.Writer, name string) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "🛑 %s app shutting down...\n", name)
	fmt.Fprintln(w, "Application finished.")
	fmt.Fprintln(w)
}
