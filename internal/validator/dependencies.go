package validator

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	redis "github.com/go-redis/redis/v8"
	"github.com/glorko/crux/internal/config"
)

// ValidationResult represents the result of dependency validation
type ValidationResult struct {
	Valid   bool
	Errors  []string
	Warnings []string
}

// ValidateAll checks all dependencies and required binaries
func ValidateAll(cfg *config.Config) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Check required binaries
	checkBinary(result, "go", "Go compiler")
	checkBinary(result, "flutter", "Flutter SDK")
	
	// Optional binaries (warnings only)
	if !hasBinary("psql") {
		result.Warnings = append(result.Warnings, "psql not found (optional, used for manual DB access)")
	}
	if !hasBinary("redis-cli") {
		result.Warnings = append(result.Warnings, "redis-cli not found (optional, used for manual Redis access)")
	}

	// Check PostgreSQL connection (try .env first, then config.yaml)
	if err := validatePostgres(cfg); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("PostgreSQL: %v", err))
	}

	// Check Redis connection
	if err := validateRedis(cfg); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Redis: %v", err))
	}

	// Check S3/MinIO connection (try .env first, then config.yaml)
	if err := validateS3(cfg); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("S3/MinIO: %v", err))
	}

	return result, nil
}

// checkBinary verifies if a binary exists in PATH
func checkBinary(result *ValidationResult, binary, description string) {
	_, err := exec.LookPath(binary)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("%s not found in PATH (%s)", description, binary))
	}
}

// hasBinary checks if a binary exists in PATH (returns bool, doesn't add to errors)
func hasBinary(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}

// validatePostgres checks PostgreSQL connection
// Tries to read DATABASE_URL from .env file first, then falls back to config.yaml
// Always forces sslmode=disable for local development
func validatePostgres(cfg *config.Config) error {
	var connStr string
	
	// Try reading from .env file first
	env, err := ReadEnvFile(cfg.Backend.Path)
	if err == nil && env.DatabaseURL != "" {
		connStr = env.DatabaseURL
	} else {
		// Fall back to config.yaml
		connStr = cfg.GetPostgresConnectionString()
	}

	// Force sslmode=disable for local development (remove any existing sslmode param first)
	connStr = ensureSSLModeDisabled(connStr)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to open connection: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	return nil
}

// ensureSSLModeDisabled ensures sslmode=disable is set in the connection string
// For local development, we never want SSL
func ensureSSLModeDisabled(connStr string) string {
	// Parse the URL
	parsed, err := url.Parse(connStr)
	if err != nil {
		// If parsing fails, just append sslmode=disable
		if strings.Contains(connStr, "?") {
			return connStr + "&sslmode=disable"
		}
		return connStr + "?sslmode=disable"
	}

	// Remove any existing sslmode parameter
	query := parsed.Query()
	query.Del("sslmode")
	query.Del("sslmode")
	
	// Set sslmode=disable
	query.Set("sslmode", "disable")
	
	// Rebuild the URL
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// validateRedis checks Redis connection using Go client
func validateRedis(cfg *config.Config) error {
	redisCfg := cfg.Dependencies.Redis

	// First try TCP connection
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", redisCfg.Host, redisCfg.Port), 2*time.Second)
	if err != nil {
		return fmt.Errorf("cannot connect to Redis at %s:%d: %w", redisCfg.Host, redisCfg.Port, err)
	}
	conn.Close()

	// Try actual Redis ping using Go client
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", redisCfg.Host, redisCfg.Port),
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis ping failed: %w", err)
	}

	return nil
}

// validateS3 checks S3/MinIO connection
// Tries to read MINIO_* variables from .env file first, then falls back to config.yaml
func validateS3(cfg *config.Config) error {
	var endpoint string
	
	// Try reading from .env file first
	env, err := ReadEnvFile(cfg.Backend.Path)
	if err == nil {
		if env.MinIOEndpoint != "" {
			endpoint = env.MinIOEndpoint
		}
	}
	
	// Fall back to config.yaml if .env didn't have values
	if endpoint == "" {
		s3Cfg := cfg.Dependencies.S3
		if s3Cfg.Endpoint == "" {
			return fmt.Errorf("S3/MinIO endpoint not configured (check .env or config.yaml)")
		}
		endpoint = s3Cfg.Endpoint
	}
	
	if endpoint == "" {
		return fmt.Errorf("S3/MinIO endpoint not configured")
	}
	
	// Parse endpoint URL to extract host and port
	endpointURL := endpoint
	if !strings.HasPrefix(endpointURL, "http://") && !strings.HasPrefix(endpointURL, "https://") {
		endpointURL = "http://" + endpointURL
	}
	
	parsedURL, err := url.Parse(endpointURL)
	if err != nil {
		return fmt.Errorf("invalid S3/MinIO endpoint URL: %w", err)
	}
	
	host := parsedURL.Hostname()
	port := parsedURL.Port()
	if port == "" {
		if parsedURL.Scheme == "https" {
			port = "443"
		} else {
			port = "9000" // MinIO default
		}
	}
	
	// Test TCP connection
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%s", host, port), 2*time.Second)
	if err != nil {
		return fmt.Errorf("cannot connect to S3/MinIO at %s:%s: %w", host, port, err)
	}
	conn.Close()
	
	// Try HTTP health check (MinIO has a health endpoint)
	healthURL := fmt.Sprintf("%s/minio/health/live", endpoint)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(healthURL)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return nil // MinIO health check passed
		}
	}
	
	// If health check fails, at least we know the endpoint is reachable
	// (might be S3-compatible service without health endpoint)
	return nil
}

// PrintResults prints validation results in a user-friendly format
func (r *ValidationResult) PrintResults() {
	if r.Valid {
		fmt.Println("✅ All dependencies validated successfully")
	} else {
		fmt.Println("❌ Dependency validation failed:")
		for _, err := range r.Errors {
			fmt.Printf("  ❌ %s\n", err)
		}
	}

	if len(r.Warnings) > 0 {
		fmt.Println("⚠️  Warnings:")
		for _, warn := range r.Warnings {
			fmt.Printf("  ⚠️  %s\n", warn)
		}
	}
}
