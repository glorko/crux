package validator

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// EnvVars contains parsed environment variables from .env file
type EnvVars struct {
	DatabaseURL      string
	MinIOEndpoint    string
	MinIOAccessKey   string
	MinIOSecretKey   string
	MinIORegion      string
}

// ReadEnvFile reads and parses a .env file
func ReadEnvFile(backendPath string) (*EnvVars, error) {
	env := &EnvVars{}
	
	// Try multiple locations for .env file
	envPaths := []string{
		filepath.Join(backendPath, ".env"),
		filepath.Join(backendPath, "services", "api", ".env"),
		filepath.Join(backendPath, "services", "api", "app", ".env"),
	}
	
	var envFile string
	for _, path := range envPaths {
		if _, err := os.Stat(path); err == nil {
			envFile = path
			break
		}
	}
	
	if envFile == "" {
		return nil, fmt.Errorf("no .env file found in backend path")
	}
	
	file, err := os.Open(envFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open .env file: %w", err)
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		
		// Remove quotes if present
		value = strings.Trim(value, `"'`)
		
		switch key {
		case "DATABASE_URL":
			env.DatabaseURL = value
		case "MINIO_ENDPOINT_URL":
			env.MinIOEndpoint = value
		case "MINIO_ACCESS_KEY_ID":
			env.MinIOAccessKey = value
		case "MINIO_SECRET_ACCESS_KEY":
			env.MinIOSecretKey = value
		case "MINIO_REGION":
			env.MinIORegion = value
		}
	}
	
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading .env file: %w", err)
	}
	
	return env, nil
}

// ParseDatabaseURL parses a DATABASE_URL and returns connection details
func ParseDatabaseURL(dbURL string) (host string, port int, database string, user string, password string, err error) {
	parsed, err := url.Parse(dbURL)
	if err != nil {
		return "", 0, "", "", "", fmt.Errorf("invalid DATABASE_URL format: %w", err)
	}
	
	host = parsed.Hostname()
	if host == "" {
		host = "localhost"
	}
	
	port = 5432 // default PostgreSQL port
	if parsed.Port() != "" {
		fmt.Sscanf(parsed.Port(), "%d", &port)
	}
	
	// Get database name from path
	database = strings.TrimPrefix(parsed.Path, "/")
	if database == "" {
		database = "postgres"
	}
	
	// Get user and password from UserInfo
	if parsed.User != nil {
		user = parsed.User.Username()
		password, _ = parsed.User.Password()
	}
	
	return host, port, database, user, password, nil
}
