package collector

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

const namespace = "redis_pubsub"

// RedisPubSubCollector implements prometheus.Collector.
// It queries Redis on every Prometheus scrape and returns fresh metrics.
type RedisPubSubCollector struct {
	client        *redis.Client
	maxChannels   int
	knownPatterns []string
	logger        *slog.Logger

	mu      sync.RWMutex // RWMutex: Collect holds write, IsRedisUp holds read
	redisUp bool         // cached for health checks

	// ---- metric descriptors ----

	// Channel metrics
	channelSubscriberCount *prometheus.Desc
	channelsTotal          *prometheus.Desc
	orphanChannelsTotal    *prometheus.Desc

	// Pattern metrics
	patternSubscriberCount *prometheus.Desc
	patternsTotal          *prometheus.Desc

	// Client metrics
	clientsTotal      *prometheus.Desc
	clientChannelSubs *prometheus.Desc
	clientPatternSubs *prometheus.Desc

	// Redis health
	redisUpDesc           *prometheus.Desc
	redisConnectedClients *prometheus.Desc
	redisUsedMemoryBytes  *prometheus.Desc

	// Exporter health
	scrapeDurationSeconds *prometheus.Desc
	scrapeErrorsTotal     *prometheus.Desc

	// Internal counter for scrape errors (persists across scrapes)
	scrapeErrors float64
}

// New creates a new RedisPubSubCollector.
func New(client *redis.Client, maxChannels int, knownPatterns []string, logger *slog.Logger) *RedisPubSubCollector {
	return &RedisPubSubCollector{
		client:        client,
		maxChannels:   maxChannels,
		knownPatterns: knownPatterns,
		logger:        logger,

		// Channel
		channelSubscriberCount: prometheus.NewDesc(
			namespace+"_channel_subscriber_count",
			"Number of direct subscribers per channel",
			[]string{"channel"}, nil,
		),
		channelsTotal: prometheus.NewDesc(
			namespace+"_channels_total",
			"Total number of active pub/sub channels",
			nil, nil,
		),
		orphanChannelsTotal: prometheus.NewDesc(
			namespace+"_orphan_channels_total",
			"Number of channels with zero direct subscribers",
			nil, nil,
		),

		// Pattern
		patternSubscriberCount: prometheus.NewDesc(
			namespace+"_pattern_subscriber_count",
			"Number of channels matching this pattern with active subscribers",
			[]string{"pattern"}, nil,
		),
		patternsTotal: prometheus.NewDesc(
			namespace+"_patterns_total",
			"Total number of active pub/sub pattern subscriptions",
			nil, nil,
		),

		// Client
		clientsTotal: prometheus.NewDesc(
			namespace+"_clients_total",
			"Total number of clients with pub/sub subscriptions",
			nil, nil,
		),
		clientChannelSubs: prometheus.NewDesc(
			namespace+"_client_channel_subscriptions",
			"Number of channel subscriptions per client",
			[]string{"client_name", "client_addr"}, nil,
		),
		clientPatternSubs: prometheus.NewDesc(
			namespace+"_client_pattern_subscriptions",
			"Number of pattern subscriptions per client",
			[]string{"client_name", "client_addr"}, nil,
		),

		// Redis health
		redisUpDesc: prometheus.NewDesc(
			namespace+"_exporter_redis_up",
			"Whether Redis is reachable (1=up, 0=down)",
			nil, nil,
		),
		redisConnectedClients: prometheus.NewDesc(
			namespace+"_exporter_redis_connected_clients",
			"Total number of connected Redis clients",
			nil, nil,
		),
		redisUsedMemoryBytes: prometheus.NewDesc(
			namespace+"_exporter_redis_used_memory_bytes",
			"Redis used memory in bytes",
			nil, nil,
		),

		// Exporter health
		scrapeDurationSeconds: prometheus.NewDesc(
			namespace+"_exporter_scrape_duration_seconds",
			"Duration of the last scrape",
			nil, nil,
		),
		scrapeErrorsTotal: prometheus.NewDesc(
			namespace+"_exporter_scrape_errors_total",
			"Total number of scrape errors",
			nil, nil,
		),
	}
}

// Describe sends all metric descriptors to the channel.
func (c *RedisPubSubCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.channelSubscriberCount
	ch <- c.channelsTotal
	ch <- c.orphanChannelsTotal
	ch <- c.patternSubscriberCount
	ch <- c.patternsTotal
	ch <- c.clientsTotal
	ch <- c.clientChannelSubs
	ch <- c.clientPatternSubs
	ch <- c.redisUpDesc
	ch <- c.redisConnectedClients
	ch <- c.redisUsedMemoryBytes
	ch <- c.scrapeDurationSeconds
	ch <- c.scrapeErrorsTotal
}

// Collect is called by Prometheus on each scrape.
// redis_up is emitted exactly once per scrape to avoid duplicate metric panics.
func (c *RedisPubSubCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	up := 0.0
	if err := c.scrape(ctx, ch); err != nil {
		c.scrapeErrors++
		c.logger.Error("scrape failed", "error", err)
	} else {
		up = 1.0
	}

	c.redisUp = up == 1.0
	ch <- prometheus.MustNewConstMetric(c.redisUpDesc, prometheus.GaugeValue, up)
	ch <- prometheus.MustNewConstMetric(c.scrapeErrorsTotal, prometheus.CounterValue, c.scrapeErrors)
	ch <- prometheus.MustNewConstMetric(c.scrapeDurationSeconds, prometheus.GaugeValue, time.Since(start).Seconds())
}

// IsRedisUp reports whether the last scrape reached Redis successfully.
// Uses RLock so health checks don't block during scrapes.
func (c *RedisPubSubCollector) IsRedisUp() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.redisUp
}

// scrape queries Redis and emits metrics. Does NOT emit redis_up (caller handles that).
func (c *RedisPubSubCollector) scrape(ctx context.Context, ch chan<- prometheus.Metric) error {
	// Ping
	if err := c.client.Ping(ctx).Err(); err != nil {
		return err
	}

	// Redis INFO: clients
	clientsInfo, err := c.client.InfoMap(ctx, "clients").Result()
	if err != nil {
		return err
	}
	if section := infoSection(clientsInfo, "clients"); section != nil {
		if v, ok := section["connected_clients"]; ok {
			ch <- prometheus.MustNewConstMetric(c.redisConnectedClients, prometheus.GaugeValue, parseFloat(v))
		}
	}

	// Redis INFO: memory
	memInfo, err := c.client.InfoMap(ctx, "memory").Result()
	if err != nil {
		return err
	}
	if section := infoSection(memInfo, "memory"); section != nil {
		if v, ok := section["used_memory"]; ok {
			ch <- prometheus.MustNewConstMetric(c.redisUsedMemoryBytes, prometheus.GaugeValue, parseFloat(v))
		}
	}

	// 1. Active channels
	channels, err := c.client.PubSubChannels(ctx, "*").Result()
	if err != nil {
		return err
	}

	// High cardinality guard
	if len(channels) > c.maxChannels {
		c.logger.Warn("channel count exceeds MAX_CHANNELS, truncating",
			"count", len(channels), "max", c.maxChannels)
		channels = channels[:c.maxChannels]
	}

	ch <- prometheus.MustNewConstMetric(c.channelsTotal, prometheus.GaugeValue, float64(len(channels)))

	// NUMSUB for each channel
	orphanCount := 0
	if len(channels) > 0 {
		numsub, err := c.client.PubSubNumSub(ctx, channels...).Result()
		if err != nil {
			return err
		}
		for channel, count := range numsub {
			ch <- prometheus.MustNewConstMetric(c.channelSubscriberCount, prometheus.GaugeValue, float64(count), channel)
			if count == 0 {
				orphanCount++
			}
		}
	}
	ch <- prometheus.MustNewConstMetric(c.orphanChannelsTotal, prometheus.GaugeValue, float64(orphanCount))

	// 2. Pattern count
	numpat, err := c.client.PubSubNumPat(ctx).Result()
	if err != nil {
		return err
	}
	ch <- prometheus.MustNewConstMetric(c.patternsTotal, prometheus.GaugeValue, float64(numpat))

	// 3. CLIENT LIST
	clientListRaw, err := c.client.ClientList(ctx).Result()
	if err != nil {
		return err
	}
	pubsubClients := ParseClientList(clientListRaw)
	ch <- prometheus.MustNewConstMetric(c.clientsTotal, prometheus.GaugeValue, float64(len(pubsubClients)))

	for _, cl := range pubsubClients {
		if cl.Sub > 0 {
			ch <- prometheus.MustNewConstMetric(c.clientChannelSubs, prometheus.GaugeValue, float64(cl.Sub), cl.Name, cl.Addr)
		}
		if cl.PSub > 0 {
			ch <- prometheus.MustNewConstMetric(c.clientPatternSubs, prometheus.GaugeValue, float64(cl.PSub), cl.Name, cl.Addr)
		}
	}

	// 4. Pattern activity inference
	patternSet := make(map[string]struct{})
	for _, p := range c.knownPatterns {
		patternSet[p] = struct{}{}
	}
	// Auto-discover prefixes from channel names
	for _, channelName := range channels {
		if idx := strings.IndexByte(channelName, '.'); idx >= 0 {
			patternSet[channelName[:idx]+".*"] = struct{}{}
		}
	}

	for pattern := range patternSet {
		matching, err := c.client.PubSubChannels(ctx, pattern).Result()
		if err != nil {
			c.logger.Warn("failed to query pattern channels", "pattern", pattern, "error", err)
			continue
		}
		if len(matching) > 0 {
			ch <- prometheus.MustNewConstMetric(c.patternSubscriberCount, prometheus.GaugeValue, float64(len(matching)), pattern)
		}
	}

	return nil
}

// infoSection does a case-insensitive lookup for a section key in Redis InfoMap output.
// go-redis may return "Clients" or "clients" depending on version.
func infoSection(m map[string]map[string]string, key string) map[string]string {
	// Try exact match first (fast path)
	if section, ok := m[key]; ok {
		return section
	}
	// Case-insensitive fallback
	lower := strings.ToLower(key)
	for k, v := range m {
		if strings.ToLower(k) == lower {
			return v
		}
	}
	return nil
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
