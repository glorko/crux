# Crux - AI-Native Dev Environment Orchestrator

**For agentic engineers and vibe coders** — one command to run your local dev stack and see what’s going on.

Holding a local environment together with agentic tools is still painful: when you drive execution you waste time on terminal commands; when Cursor or Claude drive it you can’t read logs or see what’s happening. Crux is an AI-optimized launcher for your local environment. You get an LLM to generate a `config.yaml`, run `crux`, and your services (backend, frontend, workers, whatever you run) and dependencies start in Wezterm tabs — with logs you can read. Crux has built-in MCP, so your AI can read logs, restart a service, or send input to any tab.

> **Tested on macOS.** Wezterm is cross-platform; contributions welcome to make it work smoothly on Linux.

## The Dream

1. Install crux
2. Run `crux --prompt` and paste the output to your AI
3. AI analyzes your project and generates `config.yaml`
4. Run `crux`
5. Your stack is running — all services and dependencies in one place

**That's it.** The prompt teaches your AI how crux works, it reads your project, generates the config, and crux handles the rest. If LLM misunderstood your setup - you may ask to make tweaks in the config. 

## Reality Check

This works great for many setups, but if something doesn't work for your architecture:
- **Create an issue** - [github.com/glorko/crux/issues](https://github.com/glorko/crux/issues)
- **Ping me directly** - [t.me/glorfindeil](https://t.me/glorfindeil)

More architectures and platforms coming soon!

## What Crux Does

Crux spawns each service in its own terminal tab, giving you full interactive control (Ctrl+C, logs, commands) while managing everything from a single config file. AI agents can monitor and control services via MCP.

## Features

- **AI-Native** - Built for LLM agents to configure and control via MCP
- **Native Terminal Tabs** - Each service runs in its own Wezterm tab
- **One Command Launch** - `crux` reads config.yaml and starts everything
- **Dependency Management** - Auto-start databases, queues, emulators — anything with a check/start
- **Crash Recovery** - Logs persisted, tabs stay open on failure
- **Interactive Control** - Full terminal access, hot reload, keyboard input

## Requirements

### Terminal (Wezterm only)

| Terminal | Install | Tab support |
|----------|---------|-------------|
| **Wezterm** | `brew install --cask wezterm` | Native CLI |

Wezterm is the only supported terminal — native tabs, MCP integration, and `start-one` for crash recovery.

### Other Requirements

- **Go 1.21+** - For building crux
- **Docker** - For running dependencies (postgres, redis, mongo, etc.)
- **Your services** - Any process: backends, frontends, workers, scripts — any tech stack

## Installation

### 1. Install Wezterm (required)

macOS: `brew install --cask wezterm`. Linux: [wezterm install](https://wezfurlong.org/wezterm/install.html).

### 2. Install crux

**macOS** — Homebrew (use `--HEAD` until there’s a stable release):

```bash
brew tap glorko/crux
brew install --HEAD glorko/crux/crux
```

**Linux (or build from source)** — manual:

```bash
git clone https://github.com/glorko/crux.git
cd crux
./install.sh
```

Manual install puts `crux` and `crux-mcp` in `~/bin` and adds it to your shell PATH.

### 3. MCP (Cursor)

In `~/.cursor/mcp.json` add crux under `mcpServers`. Use the path for your platform (see table). Restart Cursor after changing.

| Platform / install | `"command"` |
|--------------------|-------------|
| macOS (Homebrew)   | `"/opt/homebrew/bin/crux-mcp"` (Apple Silicon) or `"/usr/local/bin/crux-mcp"` (Intel) |
| Linux / manual     | `"${userHome}/bin/crux-mcp"` |

Example (macOS Homebrew; merge into your existing `mcpServers` if you have other servers):

```json
{
  "mcpServers": {
    "crux": {
      "command": "/opt/homebrew/bin/crux-mcp",
      "args": []
    }
  }
}
```

### Verify

`crux --version` · `which crux-mcp`

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
    workdir: ./backend

  - name: frontend
    command: npm
    args: ["run", "dev"]
    workdir: ./frontend

  - name: worker
    command: python
    args: ["-m", "worker"]
    workdir: ./worker

terminal:
  app: wezterm
```

2. Run crux:

```bash
crux                        # Uses config.yaml
crux -c config.test.yaml    # Use different config
```

3. A Wezterm window opens with tabs - one for each service!

### Multiple Configurations

You can have different configs for different scenarios:

```bash
config.yaml           # Default dev setup
config.test.yaml      # For running tests
config.minimal.yaml   # Minimal set (e.g. backend only)
```

```bash
crux                        # Uses config.yaml
crux -c config.test.yaml    # Uses test config
crux --config=config.e2e.yaml
```

## Dependencies

Crux can automatically check and start dependencies before running your services. **Any dependency works** - the pattern is simple:

- `check` - Command that exits 0 if dependency is ready
- `start` - Command to run if check fails
- `timeout` - How long to wait for it to become ready

```yaml
dependencies:
  - name: my-dependency
    check: some-command-that-exits-0-if-ready
    start: command-to-start-it
    timeout: 30
```

### Common Examples

```yaml
dependencies:
  # PostgreSQL via Docker
  - name: postgres
    check: pg_isready -h localhost -p 5432
    start: docker run -d --name crux-postgres -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:15
    timeout: 30

  # Redis via Docker
  - name: redis
    check: redis-cli ping
    start: docker run -d --name crux-redis -p 6379:6379 redis:7
    timeout: 15

  # MongoDB via Docker
  - name: mongo
    check: mongosh --eval "db.runCommand('ping')" --quiet
    start: docker run -d --name crux-mongo -p 27017:27017 mongo:7
    timeout: 30

  # MinIO via Docker
  - name: minio
    check: curl -sf http://localhost:9000/minio/health/live >/dev/null
    start: docker run -d --name crux-minio -p 9000:9000 -p 9001:9001 minio/minio server /data --console-address ":9001"
    timeout: 20

  # Optional: mobile simulators (e.g. for Flutter, React Native)
  # iOS Simulator
  - name: ios-simulator
    check: xcrun simctl list devices | grep -q Booted
    start: open -a Simulator
    timeout: 60
  # Android Emulator
  - name: android-emulator
    check: adb devices | grep -q emulator
    start: sh -c 'nohup emulator -avd YOUR_AVD_NAME > /tmp/crux-emulator.log 2>&1 &'
    timeout: 120

  # Any custom service
  - name: my-custom-api
    check: curl -sf http://localhost:8080/health
    start: docker-compose up -d my-api
    timeout: 45
```

**The pattern works for anything** - Docker containers, brew services, emulators, custom scripts. If you can write a check command and a start command, crux can manage it.

When you run `crux`:
1. Each dependency's `check` is run
2. If check fails and `start` is provided, it runs the start command
3. Polls `check` until it passes or timeout expires
4. Once all pass → services start! 🎉

## Configuration

### config.yaml

```yaml
# Services to run (each gets its own terminal tab). Any command, any stack.
services:
  - name: backend
    command: go
    args: ["run", "./cmd/server"]
    workdir: ./backend

  - name: frontend
    command: npm
    args: ["run", "dev"]
    workdir: ./frontend

  - name: worker
    command: python
    args: ["-m", "worker"]
    workdir: ./worker

terminal:
  app: wezterm
```

#### Wezterm keybindings

- `Ctrl+Shift+T` - New tab
- `Ctrl+Tab` / `Ctrl+Shift+Tab` - Switch tabs
- `Ctrl+Shift+W` - Close tab

## MCP Server (AI/LLM Integration)

**Your AI can see everything.** Ask Cursor/Claude about what's happening in your running services - it has full access to terminal output, logs, and can send commands.

The MCP server lets AI assistants:
- **Read live logs** from any running terminal tab
- **See crash logs** even after a tab closes
- **Send commands** like hot reload (`r`), restart (`R`), or any keystroke
- **Monitor status** of all services

> **Note:** See [Installation](#installation) for setup instructions.

### Available Tools

| Tool | Description |
|------|-------------|
| `crux_status` | List all terminal tabs with their numbers and titles |
| `crux_send` | Send text/commands to a tab (e.g., `r` for hot reload, `R` for restart, `q` for quit) |
| `crux_logs` | Get terminal scrollback from a tab (last N lines) |
| `crux_focus` | Focus/activate a specific tab in Wezterm |
| `crux_start_one` | Start a single service (new tab, or new window in some cases). Use when a service crashed |
| `crux_logfile` | Read log files for crashed/closed tabs. Each run creates timestamped log in `/tmp/crux-logs/<service>/` |

### Tool Parameters

**crux_send**
- `tab` - Tab number (1, 2, 3...) or partial title match ("backend", "frontend")
- `text` - Text to send (e.g., "r", "R", "q")

**crux_logs**
- `tab` - Tab number or partial title match
- `lines` - Number of lines to retrieve (default: 50)

**crux_focus**
- `tab` - Tab number or partial title match

**crux_start_one**
- `service` - Service name from config (e.g. "backend", "frontend")
- May open a new tab in same window, or a new window, depending on session state
- Note: May open a new Wezterm window depending on session state; new tab in same window when possible

**crux_logfile**
- `service` - Service name (e.g., "backend") or "list" to show all services with logs
- `run` - Which run: "latest" (default), "list" to show all runs, or timestamp like "2024-02-11_143022"
- `lines` - Number of lines to read from end (default: 100)

### crux_logs vs crux_logfile

| Tool | When to Use |
|------|-------------|
| `crux_logs` | Tab is still **running** - reads live terminal scrollback |
| `crux_logfile` | Tab **crashed/closed** - reads persistent log file from /tmp |

### Log Files

All service output is automatically logged to `/tmp/crux-logs/<service>/<timestamp>.log`:

```
/tmp/crux-logs/
├── backend/
│   ├── 2024-02-11_143022.log
│   ├── 2024-02-11_150105.log
│   └── latest.log -> 2024-02-11_150105.log
└── frontend/
    ├── 2024-02-11_143025.log
    └── latest.log -> 2024-02-11_143025.log
```

Features:
1. **Run History** - Each `crux` run creates a new timestamped log (keeps last 10)
2. **Crash Recovery** - If a tab dies, you can still read the logs
3. **Quick Access** - `latest.log` symlink always points to most recent run
4. **Failed Startup** - Tab stays open with error message until Enter is pressed

When a service fails:
```
⚠️  Command failed! Log saved to: /tmp/crux-logs/backend/2024-02-11_143022.log
Press Enter to close this tab...
```

### Usage Examples

Ask Cursor:
- "What services are running?" (uses crux_status)
- "Send hot-reload to the frontend tab" (sends "r" to that tab)
- "Show me the backend logs" (uses crux_logs for live output)
- "Focus the backend tab" (activates tab in Wezterm)
- "Restart the backend" (uses crux_start_one to spawn in new tab)
- "The backend crashed, what happened?" (uses crux_logfile for crash logs)
- "What services have logs?" (crux_logfile with service="list")
- "Show me previous backend runs" (crux_logfile with service="backend", run="list")
- "Read the run from this morning" (crux_logfile with run="2024-02-11_090000")

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                     Wezterm Window                        │
├──────────────┬──────────────┬──────────────┬─────────────┤
│   Tab 1      │    Tab 2     │    Tab 3     │   Tab 4     │
│   backend    │   frontend   │    worker    │    ...      │
└──────────────┴──────────────┴──────────────┴─────────────┘
        ▲              ▲              ▲             ▲
        │              │              │             │
        └──────────────┴──────────────┴─────────────┘
                              │
                     wezterm cli (crux owns this)
                              │
                      ┌───────┴───────┐
                      │     crux      │  ← API server (localhost:9876)
                      │  + API server │
                      └───────┬───────┘
                              │ HTTP (tabs, send, logs, focus)
                              │
                      ┌───────┴───────┐
                      │   crux-mcp    │  ← MCP server (no wezterm knowledge)
                      └───────┬───────┘
                              │ JSON-RPC (stdio)
                      ┌───────┴───────┐
                      │    Cursor     │
                      │   (LLM/AI)    │
                      └───────────────┘
```

**How it works:**
1. `crux` reads `config.yaml`, spawns Wezterm tabs, and runs an HTTP API server
2. `crux-mcp` talks to crux's API only (no direct wezterm access)
3. Cursor calls MCP tools → crux-mcp → crux API → wezterm

## Examples

### Go + Node + Docker

```yaml
services:
  - name: postgres
    command: docker
    args: ["run", "--rm", "-p", "5432:5432", "-e", "POSTGRES_PASSWORD=dev", "postgres:15"]

  - name: backend
    command: go
    args: ["run", "./cmd/server"]
    workdir: ./backend

  - name: frontend
    command: npm
    args: ["run", "dev"]
    workdir: ./frontend

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

### Mobile (e.g. Flutter)

Use device IDs, not names — see [Notes](#notes) below. Add iOS/Android emulator as [dependencies](#dependencies) if you want crux to start them.

```yaml
services:
  - name: app-ios
    command: flutter
    args: ["run", "-d", "YOUR-IOS-UUID"]
    workdir: ./app
  - name: app-android
    command: flutter
    args: ["run", "-d", "emulator-5554"]
    workdir: ./app
terminal:
  app: wezterm
```

## Notes

**Flutter device IDs** — Use IDs, not device names, in your config:
- **iOS:** Get UUID with `xcrun simctl list devices` (e.g. `90266925-B62F-4741-A89E-EF11BFA0CC57`).
- **Android:** Start the emulator first (e.g. `emulator -avd Pixel_9a`), then get the ID from `flutter devices` (e.g. `emulator-5554`).

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

1. Ensure crux is running with tabs (run `crux` first - it starts the API server)
2. Check crux-mcp is built: `ls ~/bin/crux-mcp`
3. Use `${userHome}/bin/crux-mcp` in mcp.json - Cursor does **not** expand `${HOME}` (ENOENT error)
4. Restart Cursor after updating mcp.json
5. Test manually: `echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ~/bin/crux-mcp`

## Version

Current version: **v0.9.0**

## License

MIT License
