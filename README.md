# Crux - Dev Environment Controller

**⚠️ Local Development Only** - This tool is designed exclusively for local development workflows and should never be used in production or staging environments.

Crux is a Go-based CLI tool to orchestrate your local development environment. It manages backend services, multiple Flutter app instances, and web apps with interactive controls and hot reload capabilities.

## Features

- 🚀 **Multi-Service Orchestration** - Start/stop backend, Flutter apps, and web apps in parallel
- 🔄 **Hot Reload/Restart** - Hot reload/restart for Flutter apps
- 🔨 **Backend Management** - Backend restart with automatic rebuild (Go) or reload (Python/Node.js)
- ✅ **Dependency Validation** - Validates PostgreSQL, Redis, S3/MinIO, and required binaries
- 📋 **Interactive Menu** - Easy-to-use console menu for service selection
- 🎨 **Color-Coded Logs** - Timestamped, color-coded process output with service prefixes
- ⚙️ **Flexible Configuration** - YAML-based config with .env file support
- 🌐 **Web App Support** - Run Node.js/Vite/React apps alongside backend and Flutter
- 🔍 **Smart Validation** - Reads database and S3 URLs from .env files automatically

## Supported Technologies

### ✅ Fully Supported

#### Backend Languages
- **Go** - Automatic detection, uses `go run` or custom start script, rebuilds on restart
- **Python** - Automatic detection (FastAPI/Uvicorn), uses `uvicorn` with `--reload` or custom start script
- **Node.js** - Via custom start script (e.g., `"node server.js"`, `"npm start"`)

#### Mobile Frameworks
- **Flutter** - Full support with hot reload, hot restart, automatic emulator/simulator launch
  - iOS Simulators (automatic boot)
  - Android Emulators (automatic launch)
  - Physical devices (iOS and Android)

#### Web Frameworks
- **Vite** - Via `npm run dev`, `yarn dev`, `pnpm dev`
- **React** - Via Vite, Create React App, or custom scripts
- **Vue** - Via Vite or custom scripts
- **Next.js** - Via `npm run dev` or custom scripts
- **Any Node.js app** - Via custom start script

#### Databases & Services
- **PostgreSQL** - Connection validation, reads from `DATABASE_URL` in .env
- **Redis** - Connection validation
- **MinIO** - S3-compatible storage, connection validation, reads from `MINIO_*` env vars
- **S3** - Any S3-compatible service (MinIO, LocalStack, AWS S3, etc.)

### ⚠️ Partially Supported / Workarounds

#### Backend Languages
- **Java** - Use custom start script (e.g., `"mvn spring-boot:run"` or `"gradle bootRun"`)
- **Ruby** - Use custom start script (e.g., `"rails server"` or `"bundle exec rackup"`)
- **PHP** - Use custom start script (e.g., `"php -S localhost:8000"`)
- **Rust** - Use custom start script (e.g., `"cargo run"`)

#### Mobile Frameworks
- **React Native** - Not directly supported (use `react-native run-ios`/`run-android` in start script)
- **Native iOS/Android** - Not supported (use Xcode/Android Studio directly)

#### Web Frameworks
- **Django** - Use custom start script (e.g., `"python manage.py runserver"`)
- **Flask** - Use custom start script (e.g., `"flask run"`)
- **Rails** - Use custom start script (e.g., `"rails server"`)

### ❌ Not Supported

- **Docker Compose** - Use `docker-compose up` directly (crux doesn't manage Docker containers)
- **Kubernetes** - Not supported (use `kubectl` directly)
- **Terraform/Infrastructure** - Not supported (crux is for application services only)
- **Database Migrations** - Not managed by crux (run migrations in your start script or separately)
- **CI/CD Pipelines** - Not supported (crux is local development only)
- **Production Deployments** - Not supported (crux is local development only)

### Notes

- **Custom Start Scripts**: Any technology can be supported via custom `start_script` in config.yaml
- **Hot Reload**: Only Flutter apps support hot reload via crux commands. Other frameworks use their own reload mechanisms (e.g., Vite HMR, uvicorn --reload)
- **Build Tools**: Crux doesn't manage build tools (webpack, rollup, etc.) - they should be part of your start script
- **Package Managers**: Crux doesn't install dependencies - ensure `npm install`, `go mod download`, etc. are run before starting crux

## Installation

### Option 1: Go Install (Recommended)

```bash
cd crux
go install ./cmd/crux
```

The binary will be installed to `$GOPATH/bin/crux` or `$HOME/go/bin/crux`. Ensure this directory is in your PATH:

```bash
# Add to ~/.zshrc or ~/.bashrc
export PATH="$PATH:$HOME/go/bin"
```

### Option 2: Install Script

```bash
cd crux
./install.sh
```

The script will check your PATH, install the binary, and provide instructions.

### Option 3: Manual Build

```bash
cd crux
go build -o /usr/local/bin/crux ./cmd/crux
```

## Configuration

Crux searches for `config.yaml` in the following order:

1. Current directory
2. Parent directories (up to 5 levels)
3. `~/.crux/config.yaml` (global config)

### Quick Start

1. Copy the example config:
   ```bash
   cp config.example.yaml config.yaml
   ```

2. Customize `config.yaml` for your project (see example below)

3. Run crux:
   ```bash
   crux
   ```

### Configuration Structure

See `config.example.yaml` for a complete example. Here's a minimal example:

```yaml
backend:
  path: "backend"
  start_script: "run.sh"  # Optional for Go, required for Python/Node.js

flutter:
  path: "."
  instances:
    - name: "consumer"
      device_id: "90266925-B62F-4741-A89E-EF11BFA0CC57"
      platform: "ios"
      app_path: "flutter-consumer"
    - name: "vendor"
      device_id: "Pixel_9a"
      platform: "android"
      app_path: "flutter-vendor"

webapps:
  instances:
    - name: "admin"
      path: "webapps/admin"
      start_script: "npm run dev"
      port: 5173

dependencies:
  postgres:
    host: "localhost"
    port: 5432
    database: "mydb"
    user: "postgres"
    password: "postgres"
  redis:
    host: "localhost"
    port: 6379
  s3:
    endpoint: "http://localhost:9000"
    access_key_id: "minioadmin"
    secret_access_key: "minioadmin"
    region: "us-east-1"
```

### Environment Variables (.env Files)

Crux automatically reads environment variables from `.env` files in your backend directory:

- **DATABASE_URL** - Overrides PostgreSQL config from config.yaml
- **MINIO_ENDPOINT_URL** - Overrides S3 endpoint from config.yaml
- **MINIO_ACCESS_KEY_ID** - Overrides S3 access key from config.yaml
- **MINIO_SECRET_ACCESS_KEY** - Overrides S3 secret key from config.yaml

Crux searches for `.env` files in:
1. `backend/.env`
2. `backend/services/api/.env`
3. `backend/services/api/app/.env`

This allows you to keep sensitive credentials in `.env` files (which should be in `.gitignore`) while keeping non-sensitive config in `config.yaml`.

## Usage

### Basic Usage

Run crux from your project root (where `config.yaml` is located):

```bash
crux
```

Or specify a config file:

```bash
crux --config /path/to/config.yaml
```

### Startup Menu

When you run crux, you'll see an interactive menu:

```
╔════════════════════════════════════════╗
║     Crux - Dev Environment Controller  ║
║     ⚠️  Local Development Only          ║
╚════════════════════════════════════════╝

What would you like to start?
  [1] Backend
  [2] Flutter consumer
  [3] Flutter vendor
  [4] WebApp admin
  [5] All (Backend + All Flutter Apps + All WebApps)
  [q] Quit
```

### Runtime Controls

Once services are running, you can use these commands:

- **`r`** - Hot reload Flutter (all instances)
- **`R`** - Hot restart Flutter (all instances)
- **`r b`** or **`rb`** - Restart backend (rebuilds Go backends, reloads Python/Node.js)
- **`q`** - Quit all services and exit crux

Press `Ctrl+C` to gracefully shut down all services.

### Process Output

All process output is displayed with timestamps and color-coded by service:

```
[15:04:05] [Backend] Starting server...
[15:04:06] [Flutter consumer] Hot reload performed in 234ms
[15:04:07] [WebApp admin] VITE v5.0.0 ready in 123 ms
```

## Backend Support

### Go Backends

- Automatically detects Go backends
- Uses `go run cmd/server/main.go` or runs `start_script` if specified
- Restart performs `go build` before starting

### Python Backends

- Detects Python backends (looks for `services/api/app/main.py`)
- Runs `uvicorn app.main:app --reload` automatically
- Or uses `start_script` if specified (e.g., `run.sh`)

### Node.js/Other Backends

- Use `start_script` to specify the command (e.g., `"node server.js"`, `"npm start"`)

## Dependencies

Crux validates the following dependencies before starting:

### Required

- **Go** - Required for backend compilation (Go backends only)
- **Flutter** - Required for Flutter app execution
- **PostgreSQL** - Backend database (connection validated)
- **Redis** - Backend cache/queue (connection validated)
- **S3/MinIO** - Object storage (connection validated)

### Optional

- **psql** - PostgreSQL client (warning if not found)
- **redis-cli** - Redis client (warning if not found)

### Dependency Validation

Crux validates dependencies in this order:

1. Reads `.env` file from backend directory
2. Uses `DATABASE_URL` from .env if present, otherwise uses config.yaml
3. Uses `MINIO_*` variables from .env if present, otherwise uses config.yaml
4. Tests actual connections (not just config validation)

## Finding Flutter Device IDs

### iOS Simulators

```bash
xcrun simctl list devices
```

Look for the device ID (UUID format) in the output.

### Android Emulators

```bash
emulator -list-avds
```

Use the AVD name (e.g., `Pixel_9a`) as the `device_id` in config.

### Running Devices

```bash
flutter devices
```

Shows currently running devices with their IDs.

## Troubleshooting

### Config File Not Found

If crux can't find your config file:

1. Create a `config.yaml` in your project root (copy from `config.example.yaml`)
2. Or use `--config` flag to specify the path
3. Or create `~/.crux/config.yaml` for a global config

### Flutter Device Not Found

Make sure your device/emulator is running and visible:

```bash
flutter devices
```

Update the `device_id` in your config.yaml. Crux will automatically launch emulators/simulators if they're not running.

### Backend Build Fails

**For Go backends:**
- Ensure Go is installed and in PATH
- Backend dependencies are installed (`go mod download`)
- Backend path in config is correct

**For Python backends:**
- Ensure virtual environment is activated in your start script
- Dependencies are installed (`pip install -r requirements.txt`)
- Check that `run.sh` or start script is executable

### PostgreSQL/Redis Connection Fails

Check:
- Services are running (Docker, local install, etc.)
- Connection details in config match your setup
- `.env` file has correct `DATABASE_URL` if using one
- Ports are not blocked by firewall

### S3/MinIO Connection Fails

Check:
- MinIO is running (Docker: `docker run -p 9000:9000 minio/minio server /data`)
- `MINIO_ENDPOINT_URL` in `.env` or `s3.endpoint` in config.yaml is correct
- Access keys match your MinIO setup

### Web App Not Starting

- Ensure Node.js is installed
- Dependencies are installed (`npm install`, `yarn install`, etc.)
- `start_script` command is correct (e.g., `"npm run dev"`, `"yarn dev"`)
- Port is not already in use

## Development

### Building from Source

```bash
git clone <repository>
cd crux
go build -o crux ./cmd/crux
```

### Running Tests

```bash
go test ./...
```

### Project Structure

```
crux/
├── cmd/
│   ├── crux/          # Main CLI application
│   └── install/        # Installer utility
├── internal/
│   ├── config/         # Configuration loading
│   ├── flutter/        # Flutter app runner
│   ├── process/        # Process management
│   ├── ui/             # Interactive menu
│   ├── validator/      # Dependency validation
│   └── webapp/         # Web app runner
├── config.example.yaml # Example configuration
├── README.md           # This file
└── .gitignore          # Git ignore rules
```

## Version

Current version: **v0.6.0**

Check version:
```bash
crux --version
```

## License

Local development tool - use at your own risk.

## Contributing

This is a local development tool. Contributions welcome, but keep it simple and focused on local dev workflows.

## Examples

### Example 1: Go Backend + Flutter Apps

```yaml
backend:
  path: "backend"
  # Go backend - crux will use 'go run' automatically

flutter:
  path: "."
  instances:
    - name: "ios"
      device_id: "90266925-B62F-4741-A89E-EF11BFA0CC57"
      platform: "ios"
      app_path: "mobile"
```

### Example 2: Python Backend + Flutter + Web Apps

```yaml
backend:
  path: "backend"
  start_script: "run.sh"  # Runs uvicorn

flutter:
  path: "."
  instances:
    - name: "consumer"
      device_id: "Pixel_9a"
      platform: "android"
      app_path: "flutter-consumer"

webapps:
  instances:
    - name: "admin"
      path: "webapps/admin"
      start_script: "npm run dev"
```

### Example 3: Using .env Files

Backend `.env` file:
```bash
DATABASE_URL=postgresql://user:pass@localhost:5432/mydb
MINIO_ENDPOINT_URL=http://localhost:9000
MINIO_ACCESS_KEY_ID=minioadmin
MINIO_SECRET_ACCESS_KEY=minioadmin
```

`config.yaml` (minimal - .env overrides):
```yaml
backend:
  path: "backend"

dependencies:
  postgres:
    host: "localhost"  # Will be overridden by DATABASE_URL from .env
    port: 5432
    # ...
```
