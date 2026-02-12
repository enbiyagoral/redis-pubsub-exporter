package collector

import (
	"strconv"
	"strings"
)

// PubSubClient represents a Redis client with pub/sub subscriptions.
type PubSubClient struct {
	Addr string // client address (ip:port)
	Name string // client name (from CLIENT SETNAME)
	Sub  int    // number of channel subscriptions (SUBSCRIBE)
	PSub int    // number of pattern subscriptions (PSUBSCRIBE)
}

// ParseClientList parses the output of Redis CLIENT LIST command
// and returns only clients that have active pub/sub subscriptions.
//
// CLIENT LIST returns lines like:
//
//	id=123 addr=127.0.0.1:54321 name=myapp sub=2 psub=1 ...
func ParseClientList(raw string) []PubSubClient {
	var clients []PubSubClient

	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := make(map[string]string)
		for _, pair := range strings.Split(line, " ") {
			idx := strings.IndexByte(pair, '=')
			if idx < 0 {
				continue
			}
			fields[pair[:idx]] = pair[idx+1:]
		}

		sub := parseIntField(fields, "sub")
		psub := parseIntField(fields, "psub")

		if sub == 0 && psub == 0 {
			continue
		}

		name := fields["name"]
		if name == "" {
			name = "unnamed"
		}

		addr := fields["addr"]
		if addr == "" {
			addr = "unknown"
		}

		clients = append(clients, PubSubClient{
			Addr: addr,
			Name: name,
			Sub:  sub,
			PSub: psub,
		})
	}

	return clients
}

func parseIntField(fields map[string]string, key string) int {
	v, ok := fields[key]
	if !ok {
		return 0
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return i
}
