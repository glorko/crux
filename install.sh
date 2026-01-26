#!/bin/bash

# Crux Installation Script
# Uses Go installer library for proper PATH management

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

# Use the Go installer
go run ./cmd/install/main.go --go-install
