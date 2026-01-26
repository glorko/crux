package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/glorko/crux/internal/installer"
)

func main() {
	useGoInstall := flag.Bool("go-install", false, "Use 'go install' instead of copying binary")
	flag.Parse()

	inst, err := installer.NewInstaller()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to create installer: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("🚀 Installing crux...")
	fmt.Printf("Install directory: %s\n", inst.GetInstallPath())

	if *useGoInstall {
		// Use go install
		if err := inst.InstallViaGoInstall("./cmd/crux"); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Installation failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Build and copy binary
		wd, _ := os.Getwd()
		sourceBinary := filepath.Join(wd, "crux")
		
		// Build if doesn't exist
		if _, err := os.Stat(sourceBinary); os.IsNotExist(err) {
			fmt.Println("Building crux...")
			cmd := exec.Command("go", "build", "-o", sourceBinary, "./cmd/crux")
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "❌ Build failed: %v\n", err)
				os.Exit(1)
			}
		}

		if err := inst.Install(sourceBinary); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Installation failed: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("✅ Crux installed successfully!")
	fmt.Printf("Binary location: %s\n", inst.GetBinaryPath())
	fmt.Println("\nYou may need to restart your terminal or run:")
	fmt.Println("  source ~/.zshrc  # or source ~/.bashrc")
}
