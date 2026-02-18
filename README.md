# Redis PubSub Exporter

Prometheus exporter for Redis Pub/Sub metrics. Monitors channels, patterns, and per-client subscription details that the standard Redis exporter does not expose.

## Features

- **Channel metrics** -- subscriber count per channel, orphan channel detection
- **Pattern metrics** -- auto-discovers active patterns from channel naming conventions + explicit pattern list
- **Client-level detail** -- per-client subscription counts via `CLIENT LIST` parsing
- **Hash metrics** -- expose application-managed subscriber counts from Redis hashes as Prometheus gauges
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
8. Reads configured Redis hashes (via `HASH_METRICS`) and emits field values as gauges

## Hash Metrics

Redis `PUBSUB NUMSUB` only reports the number of **Redis connections** subscribed to a channel. When a service multiplexes many clients over a single connection (e.g. WebSocket → Redis), `NUMSUB` always shows `1`.

Hash metrics solve this by reading **application-managed** subscriber counts from Redis hashes and exposing them as Prometheus gauges.

### Configuration

Set the `HASH_METRICS` environment variable. Each definition is separated by `;`, fields by `,`:

```bash
HASH_METRICS="redis_key=<your-hash-key>,metric=<metric_name>,help=<description>,label=<label_name>"
```

| Field | Required | Description |
|-------|----------|-------------|
| `redis_key` | Yes | Redis hash key to read |
| `metric` | Yes | Prometheus metric name suffix (`redis_pubsub_` prefix added automatically) |
| `label` | Yes | Label name for hash fields |
| `help` | No | Metric HELP text (auto-generated if omitted) |

### Example Output

Given a Redis hash:
```
HSET myapp:active_users user-1 5 user-2 3 user-3 2
```

And configuration:
```bash
HASH_METRICS="redis_key=myapp:active_users,metric=active_user_count,help=Active user sessions,label=user"
```

The exporter produces:
```
redis_pubsub_active_user_count{user="user-1"} 5
redis_pubsub_active_user_count{user="user-2"} 3
redis_pubsub_active_user_count{user="user-3"} 2
```

### Multiple Hashes

```bash
HASH_METRICS="redis_key=app:subscribers,metric=subscriber_count,label=channel;redis_key=app:connections,metric=connection_count,label=service"
```

## License

Apache License 2.0. See [LICENSE](LICENSE).
