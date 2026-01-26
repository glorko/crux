package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the complete configuration structure
type Config struct {
	Backend      BackendConfig      `yaml:"backend"`
	Flutter      FlutterConfig      `yaml:"flutter"`
	WebApps      WebAppsConfig      `yaml:"webapps"`
	Dependencies DependenciesConfig `yaml:"dependencies"`
}

// BackendConfig contains backend-specific configuration
type BackendConfig struct {
	Path        string            `yaml:"path"`
	StartScript string            `yaml:"start_script"`
	Env         map[string]string `yaml:"env"`
}

// FlutterConfig contains Flutter-specific configuration
type FlutterConfig struct {
	Path      string           `yaml:"path"`
	Instances []FlutterInstance `yaml:"instances"`
}

// FlutterInstance represents a single Flutter app instance
type FlutterInstance struct {
	Name        string `yaml:"name"`
	DeviceID    string `yaml:"device_id"`     // Emulator ID (e.g., "emulator-5554") or iOS simulator ID
	AVDName     string `yaml:"avd_name"`      // Optional: Android AVD name (e.g., "Pixel_9a") - required if device_id is emulator ID
	Platform    string `yaml:"platform"`
	AppPath     string `yaml:"app_path"`      // Optional: subdirectory for Flutter app
}

// WebAppsConfig contains web app configuration
type WebAppsConfig struct {
	Instances []WebAppInstance `yaml:"instances"`
}

// WebAppInstance represents a single web app instance
type WebAppInstance struct {
	Name        string            `yaml:"name"`
	Path        string            `yaml:"path"`
	StartScript string            `yaml:"start_script"` // e.g., "npm run dev", "yarn dev", "pnpm dev"
	Port        int               `yaml:"port"`          // Optional: for display purposes
	Env         map[string]string `yaml:"env"`           // Optional: environment variables
}

// DependenciesConfig contains dependency configuration
type DependenciesConfig struct {
	Postgres PostgresConfig `yaml:"postgres"`
	Redis    RedisConfig    `yaml:"redis"`
	S3       S3Config       `yaml:"s3"`
}

// PostgresConfig contains PostgreSQL connection details
type PostgresConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// RedisConfig contains Redis connection details
type RedisConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// S3Config contains S3/MinIO connection details
type S3Config struct {
	Endpoint        string `yaml:"endpoint"`         // e.g., "http://localhost:9000"
	AccessKeyID     string `yaml:"access_key_id"`   // e.g., "minioadmin"
	SecretAccessKey string `yaml:"secret_access_key"` // e.g., "minioadmin"
	Region          string `yaml:"region"`           // e.g., "us-east-1"
	Bucket          string `yaml:"bucket"`           // Optional: bucket name to check
	UseSSL          bool   `yaml:"use_ssl"`         // Optional: whether to use SSL
}

// LoadConfig searches for and loads the configuration file
func LoadConfig(configPath string) (*Config, error) {
	var configFile string
	var err error

	if configPath != "" {
		configFile = configPath
	} else {
		configFile, err = FindConfigFile()
		if err != nil {
			return nil, fmt.Errorf("failed to find config file: %w", err)
		}
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configFile, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Resolve relative paths to absolute
	configDir := filepath.Dir(configFile)
	if !filepath.IsAbs(config.Backend.Path) {
		config.Backend.Path = filepath.Join(configDir, config.Backend.Path)
	}
	if !filepath.IsAbs(config.Flutter.Path) {
		config.Flutter.Path = filepath.Join(configDir, config.Flutter.Path)
	}
	// Resolve web app paths
	for i := range config.WebApps.Instances {
		if !filepath.IsAbs(config.WebApps.Instances[i].Path) {
			config.WebApps.Instances[i].Path = filepath.Join(configDir, config.WebApps.Instances[i].Path)
		}
	}

	return &config, nil
}

// FindConfigFile searches for config.yaml in multiple locations
func FindConfigFile() (string, error) {
	// 1. Current directory
	cwd, err := os.Getwd()
	if err == nil {
		configPath := filepath.Join(cwd, "config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}
	}

	// 2. Parent directories (up to 5 levels)
	current := cwd
	for i := 0; i < 5; i++ {
		configPath := filepath.Join(current, "config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	// 3. Global config
	homeDir, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(homeDir, ".crux", "config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}
	}

	return "", fmt.Errorf("config.yaml not found in current directory, parent directories, or ~/.crux/config.yaml")
}

// GetPostgresConnectionString returns a PostgreSQL connection string
func (c *Config) GetPostgresConnectionString() string {
	pg := c.Dependencies.Postgres
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		pg.User, pg.Password, pg.Host, pg.Port, pg.Database)
}

// GetRedisURL returns a Redis URL
func (c *Config) GetRedisURL() string {
	redis := c.Dependencies.Redis
	return fmt.Sprintf("redis://%s:%d", redis.Host, redis.Port)
}
