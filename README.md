# Redis PubSub Exporter

Prometheus exporter for Redis Pub/Sub metrics. Monitors channels, patterns, and per-client subscription details that the standard Redis exporter does not expose.

## Features

- **Channel metrics** -- subscriber count per channel, orphan channel detection
- **Pattern metrics** -- auto-discovers active patterns from channel naming conventions + explicit pattern list
- **Client-level detail** -- per-client subscription counts via `CLIENT LIST` parsing
- **Redis health** -- connectivity, connected clients, memory usage
- **Grafana dashboard** included (see `dashboard.json`)
- **Prometheus alerting rules** included (see `manifest.yaml`)
- **Lightweight** -- single static Go binary, ~10 MB Docker image (scratch-based)
- **No polling loop** -- implements the native `prometheus.Collector` interface; metrics are collected on each Prometheus scrape

## Quick Start

### Docker

```bash
docker run -d \
  -e REDIS_HOST=redis.example.com \
  -e REDIS_PORT=6379 \
  -p 9123:9123 \
  enbiyagoral/redis-pubsub-exporter:latest
```

### Kubernetes

You can use the provided Helm chart to deploy the exporter:

```bash
helm install redis-pubsub-exporter ./chart/redis-pubsub-exporter
```

## How It Works

Unlike traditional polling exporters, this exporter implements the `prometheus.Collector` interface. Metrics are collected fresh on every Prometheus scrape request -- there is no internal polling loop. This means:

- No stale data between scrapes
- No `SCRAPE_INTERVAL` configuration needed (Prometheus controls the interval)
- No metric label cleanup hacks

On each scrape, the exporter:

1. Pings Redis to verify connectivity
2. Fetches `INFO clients` and `INFO memory`
3. Queries `PUBSUB CHANNELS *` to get active channels
4. Queries `PUBSUB NUMSUB` for subscriber counts per channel
5. Queries `PUBSUB NUMPAT` for total pattern count
6. Parses `CLIENT LIST` output for per-client subscription detail
7. Discovers and queries patterns for activity data

## License

Apache License 2.0. See [LICENSE](LICENSE).
