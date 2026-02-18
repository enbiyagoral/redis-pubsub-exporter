package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultRedisHost     = "localhost"
	DefaultRedisPort     = 6379
	DefaultRedisDB       = 0
	DefaultListenAddress = ":9123"
	DefaultMaxChannels   = 500
)

// HashMetricDef defines a single Redis hash to expose as a Prometheus gauge.
// Each hash field becomes a label value; the numeric value becomes the gauge.
type HashMetricDef struct {
	RedisKey   string // Redis hash key to read via HGETALL
	MetricName string // Prometheus metric name (namespace prefix added by collector)
	Help       string // Metric HELP string
	FieldLabel string // Label name for hash fields
}

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
	HashMetrics   []HashMetricDef
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

	// Hash metrics: semicolon-separated definitions
	if raw := os.Getenv("HASH_METRICS"); raw != "" {
		defs, err := ParseHashMetrics(raw)
		if err == nil {
			c.HashMetrics = defs
		}
		// Invalid definitions are silently skipped; main.go logs the result.
	}

	return c
}

// ParseHashMetrics parses a HASH_METRICS string into HashMetricDef slice.
//
// Format: definitions separated by ";", fields separated by ",".
// Each definition requires: redis_key, metric, help, label.
//
// Example:
//
//	redis_key=myapp:stats,metric=active_count,help=Active items,label=item
func ParseHashMetrics(raw string) ([]HashMetricDef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var defs []HashMetricDef
	for _, segment := range strings.Split(raw, ";") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}

		fields := make(map[string]string)
		for _, pair := range strings.Split(segment, ",") {
			idx := strings.IndexByte(pair, '=')
			if idx < 0 {
				continue
			}
			k := strings.TrimSpace(pair[:idx])
			v := strings.TrimSpace(pair[idx+1:])
			fields[k] = v
		}

		def := HashMetricDef{
			RedisKey:   fields["redis_key"],
			MetricName: fields["metric"],
			Help:       fields["help"],
			FieldLabel: fields["label"],
		}

		if def.RedisKey == "" || def.MetricName == "" || def.FieldLabel == "" {
			return nil, fmt.Errorf("hash metric definition missing required field (redis_key, metric, label): %q", segment)
		}
		if def.Help == "" {
			def.Help = "Value from Redis hash " + def.RedisKey
		}

		defs = append(defs, def)
	}

	return defs, nil
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
