package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to get home directory: %v\n", err)
		os.Exit(1)
	}

	// Single install dir: ~/bin (same idea as Homebrew's single bin dir)
	binDir := filepath.Join(home, "bin")
	os.MkdirAll(binDir, 0755)

	fmt.Println("🚀 Installing crux...")

	cruxPath := filepath.Join(binDir, "crux")
	fmt.Printf("Building crux -> %s\n", cruxPath)
	cmd := exec.Command("go", "build", "-o", cruxPath, "./cmd/playground")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to build crux: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ crux installed")

	mcpPath := filepath.Join(binDir, "crux-mcp")
	fmt.Printf("Building crux-mcp -> %s\n", mcpPath)
	cmd = exec.Command("go", "build", "-o", mcpPath, "./cmd/mcp")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to build crux-mcp: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ crux-mcp installed")

	// Optional: mock binaries for testing
	fmt.Println("\nBuilding mock binaries for testing...")
	mocks := []struct {
		name string
		path string
	}{
		{"crux-backend-mock", "./cmd/playground/backend-mock"},
		{"crux-flutter-mock", "./cmd/playground/flutter-mock"},
	}
	for _, mock := range mocks {
		mockPath := filepath.Join(binDir, mock.name)
		cmd = exec.Command("go", "build", "-o", mockPath, mock.path)
		if err := cmd.Run(); err != nil {
			fmt.Printf("⚠️  Failed to build %s (optional): %v\n", mock.name, err)
		} else {
			fmt.Printf("✅ %s installed\n", mock.name)
		}
	}

	fmt.Println("\n" + "═══════════════════════════════════════════")
	fmt.Println("Installation complete!")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("\nBinaries installed:")
	fmt.Printf("  crux:      %s\n", cruxPath)
	fmt.Printf("  crux-mcp:  %s\n", mcpPath)

	ensurePathInShellConfig(home, binDir)

	fmt.Println("\nCursor MCP: in ~/.cursor/mcp.json set \"command\" to the path above for crux-mcp (Cursor needs full path; use ${userHome}/bin/crux-mcp so it works on any machine).")
	fmt.Println("\nVerify (new terminal): crux --version && which crux-mcp")
}

func ensurePathInShellConfig(home, binDir string) {
	pathLine := fmt.Sprintf("export PATH=\"$PATH:%s\"", binDir)
	var rcPath string
	for _, name := range []string{".zshrc", ".bashrc"} {
		p := filepath.Join(home, name)
		if _, err := os.Stat(p); err == nil {
			rcPath = p
			break
		}
	}
	if rcPath == "" {
		rcPath = filepath.Join(home, ".zshrc")
		_, _ = os.Create(rcPath)
	}

	f, err := os.Open(rcPath)
	if err != nil {
		fmt.Printf("\nAdd to your PATH manually:\n  %s\n", pathLine)
		return
	}
	scanner := bufio.NewScanner(f)
	hasPath := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, binDir) {
			hasPath = true
			break
		}
	}
	f.Close()
	if hasPath {
		fmt.Println("\nPATH already includes install dirs in " + filepath.Base(rcPath))
		return
	}

	rc, err := os.OpenFile(rcPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("\nAdd to your PATH manually:\n  %s\n", pathLine)
		return
	}
	defer rc.Close()
	if _, err := rc.WriteString("\n# crux\n" + pathLine + "\n"); err != nil {
		fmt.Printf("\nAdd to your PATH manually:\n  %s\n", pathLine)
		return
	}
	fmt.Printf("\n✅ Added PATH to %s (open a new terminal or run source %s)\n", rcPath, rcPath)
}
