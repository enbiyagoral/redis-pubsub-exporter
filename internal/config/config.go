package config

import (
	"os"
	"strconv"
	"strings"
)

const (
	DefaultRedisHost      = "localhost"
	DefaultRedisPort      = 6379
	DefaultRedisDB        = 0
	DefaultListenAddress  = ":9123"
	DefaultMaxChannels    = 500
)

// Config holds all configuration for the exporter.
type Config struct {
	RedisHost     string
	RedisPort     int
	RedisPassword string
	RedisDB       int
	RedisTLS      bool
	ListenAddress string
	MaxChannels   int
	KnownPatterns []string
}

// Load reads configuration from environment variables.
// Flags set via kingpin will override after this call.
func Load() *Config {
	c := &Config{
		RedisHost:     envString("REDIS_HOST", DefaultRedisHost),
		RedisPort:     envInt("REDIS_PORT", DefaultRedisPort),
		RedisDB:       envInt("REDIS_DB", DefaultRedisDB),
		RedisTLS:      envBool("REDIS_TLS", false),
		ListenAddress: envString("EXPORTER_LISTEN_ADDRESS", DefaultListenAddress),
		MaxChannels:   envInt("MAX_CHANNELS", DefaultMaxChannels),
	}

	// Backward compat: EXPORTER_PORT overrides listen address if set
	if port := os.Getenv("EXPORTER_PORT"); port != "" {
		c.ListenAddress = ":" + port
	}

	// Empty password string is treated as no password
	if pw := os.Getenv("REDIS_PASSWORD"); pw != "" {
		c.RedisPassword = pw
	}

	// Comma-separated patterns
	if raw := os.Getenv("KNOWN_PATTERNS"); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				c.KnownPatterns = append(c.KnownPatterns, p)
			}
		}
	}

	return c
}

// RedisAddr returns "host:port" for the Redis connection.
func (c *Config) RedisAddr() string {
	return c.RedisHost + ":" + strconv.Itoa(c.RedisPort)
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
