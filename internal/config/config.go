package config

import (
	"os"
	"strconv"
	"strings"
)

const (
	DefaultNSFWThreshold   = 0.85
	DefaultMaxImageSize    = 10 * 1024 * 1024 // 10 MB
	DefaultGRPCPort        = 9090
	DefaultHTTPPort        = 8080
	DefaultNSFWEndpoint    = "http://localhost:8081"
)

// Config holds runtime configuration for img-validation-service.
type Config struct {
	AppName  string
	AppHost  string
	HTTPPort int
	GRPCPort int
	LogLevel string
	DebugMode bool

	NSFWEnabled        bool
	NSFWEndpoint       string
	NSFWScoreThreshold float64
	MaxImageSizeBytes  int64
	ReadinessSkipNSFW  bool
}

// Load reads configuration from environment variables.
func Load() *Config {
	return &Config{
		AppName:            getEnv("APP_NAME", "img-validation-service"),
		AppHost:            getEnv("HTTP_HOST", "0.0.0.0"),
		HTTPPort:           getEnvAsInt("HTTP_PORT", DefaultHTTPPort),
		GRPCPort:           getEnvAsInt("GRPC_PORT", DefaultGRPCPort),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
		DebugMode:          getEnvAsBool("DEBUG_MODE", false),
		NSFWEnabled:        getEnvAsBool("NSFW_ENABLED", false),
		NSFWEndpoint:       getEnv("NSFW_ENDPOINT", DefaultNSFWEndpoint),
		NSFWScoreThreshold: getEnvAsFloat("NSFW_SCORE_THRESHOLD", DefaultNSFWThreshold),
		MaxImageSizeBytes:  getEnvAsInt64("MAX_IMAGE_SIZE_BYTES", DefaultMaxImageSize),
		ReadinessSkipNSFW:  getEnvAsBool("READINESS_SKIP_NSFW", false),
	}
}

func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			return n
		}
	}
	return defaultValue
}

func getEnvAsInt64(key string, defaultValue int64) int64 {
	if value, ok := os.LookupEnv(key); ok {
		if n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			return n
		}
	}
	return defaultValue
}

func getEnvAsFloat(key string, defaultValue float64) float64 {
	if value, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return defaultValue
}
