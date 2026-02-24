package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port int

	// MinIO / S3
	MinioEndpoint      string
	MinioPort          int
	MinioUseSSL        bool
	MinioAccessKey     string
	MinioSecretKey     string
	MinioDefaultBucket string
	MinioCDNDomain     string
	MinioUseCDN        bool

	// Redis
	RedisURL string

	// Auth
	APIKey string

	// CORS
	AllowedOrigins []string

	// Processing
	MaxUploadSize int64 // bytes
}

func Load() (*Config, error) {
	port := envInt("PORT", 8090)
	minioPort := envInt("MINIO_PORT", 9000)

	cfg := &Config{
		Port: port,

		MinioEndpoint:      envStr("MINIO_ENDPOINT", "localhost"),
		MinioPort:          minioPort,
		MinioUseSSL:        envBool("MINIO_USE_SSL", false),
		MinioAccessKey:     envStr("MINIO_ACCESS_KEY", ""),
		MinioSecretKey:     envStr("MINIO_SECRET_KEY", ""),
		MinioDefaultBucket: envStr("MINIO_DEFAULT_BUCKET", "media"),
		MinioCDNDomain:     envStr("MINIO_CDN_DOMAIN", ""),
		MinioUseCDN:        envBool("MINIO_USE_CDN", false),

		RedisURL: envStr("REDIS_URL", "redis://localhost:6379"),

		APIKey: envStr("MEDIA_SERVICE_API_KEY", ""),

		AllowedOrigins: strings.Split(envStr("ALLOWED_CORS_ORIGINS", "*"), ","),

		MaxUploadSize: envInt64("MAX_UPLOAD_SIZE", 50*1024*1024), // 50MB
	}

	if cfg.MinioAccessKey == "" || cfg.MinioSecretKey == "" {
		return nil, fmt.Errorf("MINIO_ACCESS_KEY and MINIO_SECRET_KEY are required")
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("MEDIA_SERVICE_API_KEY is required")
	}

	return cfg, nil
}

// MinioAddr returns the endpoint with port for the MinIO client.
func (c *Config) MinioAddr() string {
	return fmt.Sprintf("%s:%d", c.MinioEndpoint, c.MinioPort)
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
