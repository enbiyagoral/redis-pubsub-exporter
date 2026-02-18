package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/redis-pubsub-exporter/internal/collector"
	"github.com/redis-pubsub-exporter/internal/config"
)

var (
	// Build-time variables (set via -ldflags)
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cfg := config.Load()

	app := kingpin.New("redis-pubsub-exporter",
		"Prometheus exporter for Redis Pub/Sub channels, patterns, and client subscriptions.")
	app.Version(fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date))
	app.HelpFlag.Short('h')

	app.Flag("redis.host", "Redis server hostname.").
		Envar("REDIS_HOST").
		Default(cfg.RedisHost).
		StringVar(&cfg.RedisHost)

	app.Flag("redis.port", "Redis server port.").
		Envar("REDIS_PORT").
		Default(strconv.Itoa(cfg.RedisPort)).
		IntVar(&cfg.RedisPort)

	app.Flag("redis.password", "Redis server password.").
		Envar("REDIS_PASSWORD").
		Default("").
		StringVar(&cfg.RedisPassword)

	app.Flag("redis.db", "Redis database number.").
		Envar("REDIS_DB").
		Default(strconv.Itoa(cfg.RedisDB)).
		IntVar(&cfg.RedisDB)

	app.Flag("redis.tls", "Enable TLS for Redis connection.").
		Envar("REDIS_TLS").
		Default("false").
		BoolVar(&cfg.RedisTLS)

	app.Flag("web.listen-address", "Address to listen on for metrics (e.g. :9123 or 0.0.0.0:9123).").
		Envar("EXPORTER_LISTEN_ADDRESS").
		Default(cfg.ListenAddress).
		StringVar(&cfg.ListenAddress)

	app.Flag("max-channels", "Maximum number of channels to track (high cardinality guard).").
		Envar("MAX_CHANNELS").
		Default(strconv.Itoa(cfg.MaxChannels)).
		IntVar(&cfg.MaxChannels)

	kingpin.MustParse(app.Parse(os.Args[1:]))

	// Logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	logger.Info("starting Redis PubSub Exporter",
		"version", version,
		"redis", cfg.RedisAddr(),
		"redis_tls", cfg.RedisTLS,
		"listen", cfg.ListenAddress,
		"max_channels", cfg.MaxChannels,
		"known_patterns", cfg.KnownPatterns,
		"hash_metrics", len(cfg.HashMetrics),
	)

	for _, hm := range cfg.HashMetrics {
		logger.Info("hash metric configured",
			"redis_key", hm.RedisKey,
			"metric", hm.MetricName,
			"label", hm.FieldLabel,
		)
	}

	// Redis client
	opts := &redis.Options{
		Addr:         cfg.RedisAddr(),
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		PoolSize:     5,
	}
	if cfg.RedisTLS {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}
	rdb := redis.NewClient(opts)

	// Create and register collector
	coll := collector.New(rdb, cfg.MaxChannels, cfg.KnownPatterns, cfg.HashMetrics, logger)
	prometheus.MustRegister(coll)

	// Exporter build info
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "redis_pubsub",
		Name:      "exporter_build_info",
		Help:      "Build information for the Redis PubSub Exporter.",
	}, []string{"version", "commit", "date"})
	buildInfo.WithLabelValues(version, commit, date).Set(1)
	prometheus.MustRegister(buildInfo)

	// HTTP server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if coll.IsRedisUp() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "ok")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "redis not reachable")
		}
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<html>
<head><title>Redis PubSub Exporter</title></head>
<body>
<h1>Redis PubSub Exporter</h1>
<p>Version: %s</p>
<p><a href="/metrics">Metrics</a></p>
<p><a href="/healthz">Health</a></p>
<p><a href="/readyz">Ready</a></p>
</body>
</html>`, version)
	})

	srv := &http.Server{
		Addr:         cfg.ListenAddress,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.ListenAddress)
		errCh <- srv.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		logger.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		logger.Error("server error", "error", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
	if err := rdb.Close(); err != nil {
		logger.Error("redis close error", "error", err)
	}

	logger.Info("exporter stopped")
}
