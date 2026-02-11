#!/bin/bash

# Crux Installation Script
# Builds and installs crux and crux-mcp

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

# Run the Go installer
go run ./cmd/install/main.go
