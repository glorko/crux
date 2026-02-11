package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to get home directory: %v\n", err)
		os.Exit(1)
	}

	// Install directories
	goBin := filepath.Join(home, "go", "bin")
	homeBin := filepath.Join(home, "bin")

	// Ensure directories exist
	os.MkdirAll(goBin, 0755)
	os.MkdirAll(homeBin, 0755)

	fmt.Println("🚀 Installing crux...")

	// Build and install crux (main binary)
	cruxPath := filepath.Join(goBin, "crux")
	fmt.Printf("Building crux -> %s\n", cruxPath)
	cmd := exec.Command("go", "build", "-o", cruxPath, "./cmd/playground")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to build crux: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ crux installed")

	// Build and install crux-mcp (MCP server for Cursor)
	mcpPath := filepath.Join(homeBin, "crux-mcp")
	fmt.Printf("Building crux-mcp -> %s\n", mcpPath)
	cmd = exec.Command("go", "build", "-o", mcpPath, "./cmd/mcp")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to build crux-mcp: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ crux-mcp installed")

	// Build mock binaries for testing (optional, in homeBin)
	fmt.Println("\nBuilding mock binaries for testing...")
	mocks := []struct {
		name string
		path string
	}{
		{"crux-backend-mock", "./cmd/playground/backend-mock"},
		{"crux-flutter-mock", "./cmd/playground/flutter-mock"},
	}
	for _, mock := range mocks {
		mockPath := filepath.Join(homeBin, mock.name)
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
	fmt.Println("\nMake sure these are in your PATH:")
	fmt.Printf("  export PATH=\"$PATH:%s:%s\"\n", goBin, homeBin)
	fmt.Println("\nFor Cursor MCP integration, add to your MCP config:")
	fmt.Println(`  {"mcpServers":{"crux":{"command":"` + mcpPath + `","args":[]}}}`)
	fmt.Println("\nVerify installation:")
	fmt.Println("  crux --version")
	fmt.Println("  which crux-mcp")
}
