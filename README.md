# Crux - Dev Environment Orchestrator

**Local Development Only** - Orchestrates your dev services in native terminal tabs.

Crux spawns each service in its own terminal tab, giving you full interactive control (Ctrl+C, logs, commands) while managing everything from a single config file.

## Features

- **Native Terminal Tabs** - Each service runs in its own tab (Wezterm, Kitty, or tmux)
- **One Command Launch** - `crux` reads config.yaml and starts everything
- **Interactive Control** - Full terminal access to each service
- **MCP Integration** - Control services via AI assistants (Cursor, etc.)
- **Hot Reload/Restart** - Send commands to services via API

## Requirements

### Terminal (choose one)

| Terminal | Install | Tab Support | Recommended |
|----------|---------|-------------|-------------|
| **Wezterm** | `brew install --cask wezterm` | Native CLI | **Yes** |
| **Kitty** | `brew install --cask kitty` | Remote control | Yes |
| **tmux** | `brew install tmux` | Multiplexer | Fallback |

**Wezterm is recommended** - best CLI support for programmatic tab control.

### Other Requirements

- **Go 1.21+** - For building crux
- **Your services** - Backend, Flutter, web apps, etc.

## Installation

### 1. Install Wezterm (required)

```bash
brew install --cask wezterm
```

### 2. Build and install crux

```bash
# Clone the repo
git clone https://github.com/glorko/crux.git
cd crux

# Install both crux and crux-mcp
./install.sh
```

This installs:
- `crux` → `~/go/bin/crux`
- `crux-mcp` → `~/bin/crux-mcp`

### 3. Add to PATH

Add to your `~/.zshrc` or `~/.bashrc`:

```bash
export PATH="$PATH:$HOME/go/bin:$HOME/bin"
```

Then reload: `source ~/.zshrc`

### 4. Configure MCP for Cursor (optional but recommended)

Add to your Cursor MCP config (`~/.cursor/mcp.json` or Cursor Settings > MCP):

```json
{
  "mcpServers": {
    "crux": {
      "command": "${HOME}/bin/crux-mcp",
      "args": []
    }
  }
}
```

Restart Cursor after adding.

### Verify installation

```bash
crux --version        # Should print version
crux --help           # Should print help
which crux-mcp        # Should print ~/bin/crux-mcp
```

## Quick Start

### For AI Agents

```bash
crux --prompt
```

Copy the output and paste to your AI assistant. It will analyze your project and create the config.

### For Humans

1. Create a `config.yaml` in your project directory:

```yaml
services:
  - name: backend
    command: go
    args: ["run", "./cmd/server"]
    
  - name: flutter-ios
    command: flutter
    args: ["run", "-d", "YOUR-IOS-SIMULATOR-UUID"]  # Get UUID: xcrun simctl list
    
  - name: flutter-android
    command: flutter
    args: ["run", "-d", "emulator-5554"]  # Start emulator first, then check: flutter devices
    
  - name: web-admin
    command: npm
    args: ["run", "dev"]
    workdir: ./webapps/admin

terminal:
  app: wezterm  # Options: wezterm, kitty, tmux
```

> **Note:** For Flutter, you need device IDs not names:
> - **iOS**: Get UUID with `xcrun simctl list devices` (e.g., `90266925-B62F-4741-A89E-EF11BFA0CC57`)
> - **Android**: Start emulator first (`emulator -avd Pixel_9a`), then use the ID from `flutter devices` (e.g., `emulator-5554`)

2. Run crux:

```bash
crux
```

3. A Wezterm window opens with 3 tabs - one for each service!

## Configuration

### config.yaml

```yaml
# Services to run (each gets its own terminal tab)
services:
  - name: backend           # Display name
    command: go             # Command to run
    args: ["run", "./cmd/server"]  # Arguments
    workdir: ./backend      # Optional working directory
    
  - name: flutter-ios
    command: flutter
    args: ["run", "-d", "YOUR-IOS-UUID"]  # xcrun simctl list
    
  - name: flutter-android
    command: flutter
    args: ["run", "-d", "emulator-5554"]  # flutter devices
    
  - name: web-admin
    command: npm
    args: ["run", "dev"]
    workdir: ./webapps/admin

# Terminal application
terminal:
  app: wezterm  # wezterm (recommended), kitty, or tmux

# Only used if terminal.app is tmux
tmux:
  session_name: crux
```

### Terminal Options

#### Wezterm (Recommended)

```yaml
terminal:
  app: wezterm
```

Keybindings:
- `Ctrl+Shift+T` - New tab
- `Ctrl+Tab` / `Ctrl+Shift+Tab` - Switch tabs
- `Ctrl+Shift+W` - Close tab

#### Kitty

```yaml
terminal:
  app: kitty
```

Keybindings:
- `Ctrl+Shift+T` - New tab
- `Ctrl+Shift+Right/Left` - Switch tabs

#### tmux (Fallback)

```yaml
terminal:
  app: tmux

tmux:
  session_name: my-project
```

Opens Ghostty/iTerm2/Terminal with tmux attached. Keybindings:
- `Ctrl+B` then `0/1/2` - Switch windows
- `Ctrl+B` then `d` - Detach

## MCP Server (AI/LLM Integration)

Control crux services via AI assistants like Cursor. The MCP server communicates directly with Wezterm's CLI to manage terminal tabs.

> **Note:** See [Installation](#installation) for setup instructions.

### Available Tools

| Tool | Description |
|------|-------------|
| `crux_status` | List all terminal tabs with their numbers and titles |
| `crux_send` | Send text/commands to a tab (e.g., `r` for hot reload, `R` for restart, `q` for quit) |
| `crux_logs` | Get terminal scrollback from a tab (last N lines) |
| `crux_focus` | Focus/activate a specific tab in Wezterm |

### Tool Parameters

**crux_send**
- `tab` - Tab number (1, 2, 3...) or partial title match ("backend", "flutter")
- `text` - Text to send (e.g., "r", "R", "q")

**crux_logs**
- `tab` - Tab number or partial title match
- `lines` - Number of lines to retrieve (default: 50)

**crux_focus**
- `tab` - Tab number or partial title match

### Usage Examples

Ask Cursor:
- "What services are running?" (uses crux_status)
- "Hot reload Flutter" (sends "r" to flutter tab)
- "Hot restart the consumer app" (sends "R")
- "Show me the backend logs" (uses crux_logs)
- "Focus the backend tab" (activates tab in Wezterm)

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                     Wezterm Window                        │
├──────────────┬──────────────┬──────────────┬─────────────┤
│   Tab 1      │    Tab 2     │    Tab 3     │   Tab 4     │
│  backend     │  flutter-ios │flutter-android│  web-admin  │
└──────────────┴──────────────┴──────────────┴─────────────┘
        ▲              ▲              ▲             ▲
        │              │              │             │
        └──────────────┴──────────────┴─────────────┘
                              │
                     wezterm cli (spawn, send-text, get-text)
                              │
                      ┌───────┴───────┐
                      │   crux-mcp    │
                      │    server     │
                      └───────┬───────┘
                              │
                        JSON-RPC (stdio)
                              │
                      ┌───────┴───────┐
                      │    Cursor     │
                      │   (LLM/AI)    │
                      └───────────────┘
```

**How it works:**
1. `crux` reads `config.yaml` and spawns Wezterm with tabs for each service
2. `crux-mcp` uses `wezterm cli` to control tabs (no separate API needed)
3. Cursor calls MCP tools which execute `wezterm cli` commands

## Logs

Logs are read directly from terminal scrollback via `wezterm cli get-text`. No log files needed!

Access via MCP: "show me the backend logs" or "get logs from tab 2"

## Examples

### Full Stack App

```yaml
services:
  - name: postgres
    command: docker
    args: ["run", "--rm", "-p", "5432:5432", "-e", "POSTGRES_PASSWORD=dev", "postgres:15"]
    
  - name: backend
    command: go
    args: ["run", "./cmd/server"]
    workdir: ./backend
    
  - name: flutter-ios
    command: flutter
    args: ["run", "-d", "iPhone 15 Pro"]
    workdir: ./mobile
    
  - name: flutter-android
    command: flutter
    args: ["run", "-d", "emulator-5554"]
    workdir: ./mobile
    
  - name: admin-web
    command: npm
    args: ["run", "dev"]
    workdir: ./webapps/admin

terminal:
  app: wezterm
```

### Python Backend

```yaml
services:
  - name: api
    command: uvicorn
    args: ["app.main:app", "--reload", "--port", "8000"]
    workdir: ./backend
    
  - name: worker
    command: celery
    args: ["-A", "app.worker", "worker", "--loglevel=info"]
    workdir: ./backend

terminal:
  app: wezterm
```

### Node.js Microservices

```yaml
services:
  - name: gateway
    command: npm
    args: ["run", "dev"]
    workdir: ./services/gateway
    
  - name: users-api
    command: npm
    args: ["run", "dev"]
    workdir: ./services/users
    
  - name: orders-api
    command: npm
    args: ["run", "dev"]
    workdir: ./services/orders

terminal:
  app: wezterm
```

## Troubleshooting

### Wezterm not found

```bash
brew install --cask wezterm
```

### Tabs not opening

Make sure Wezterm is running or crux can start it:

```bash
wezterm --version
```

### Services not starting

Check the command works manually:

```bash
cd ./backend && go run ./cmd/server
```

### MCP not connecting

1. Ensure Wezterm is running with crux tabs (run `crux` first)
2. Check crux-mcp is built: `ls ~/bin/crux-mcp`
3. Restart Cursor after updating mcp.json
4. Test manually: `echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ~/bin/crux-mcp`

## Version

Current version: **v0.8.0**

## License

MIT License
