package collector

import (
	"strconv"
	"testing"
)

func TestParseClientList(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int    // expected number of clients
		checks  func(t *testing.T, clients []PubSubClient)
	}{
		{
			name: "basic case with mixed client types",
			input: "id=1 addr=10.0.0.1:12345 fd=5 name=orders-service age=100 idle=0 flags=S db=0 sub=3 psub=0 multi=-1 qbuf=0 obl=0 oll=0 events=r cmd=subscribe\n" +
				"id=2 addr=10.0.0.2:12346 fd=6 name=user-service age=200 idle=0 flags=S db=0 sub=0 psub=2 multi=-1 qbuf=0 obl=0 oll=0 events=r cmd=psubscribe\n" +
				"id=3 addr=10.0.0.3:12347 fd=7 name=web-app age=300 idle=10 flags=N db=0 sub=0 psub=0 multi=-1 qbuf=0 obl=0 oll=0 events=r cmd=get",
			want: 2,
			checks: func(t *testing.T, clients []PubSubClient) {
				t.Helper()
				assertClient(t, clients[0], "orders-service", "10.0.0.1:12345", 3, 0)
				assertClient(t, clients[1], "user-service", "10.0.0.2:12346", 0, 2)
			},
		},
		{
			name:  "unnamed client gets 'unnamed' label",
			input: "id=10 addr=10.0.0.5:9999 fd=8 name= age=50 idle=0 flags=S db=0 sub=1 psub=0 multi=-1 qbuf=0 obl=0 oll=0 events=r cmd=subscribe",
			want:  1,
			checks: func(t *testing.T, clients []PubSubClient) {
				t.Helper()
				if clients[0].Name != "unnamed" {
					t.Errorf("expected name 'unnamed', got %q", clients[0].Name)
				}
			},
		},
		{
			name:  "mixed subscribe and psubscribe on same client",
			input: "id=20 addr=10.0.0.10:4444 fd=9 name=mixed-client age=10 idle=0 flags=S db=0 sub=2 psub=3 multi=-1 qbuf=0 obl=0 oll=0 events=r cmd=subscribe",
			want:  1,
			checks: func(t *testing.T, clients []PubSubClient) {
				t.Helper()
				assertClient(t, clients[0], "mixed-client", "10.0.0.10:4444", 2, 3)
			},
		},
		{
			name:  "empty input returns nil slice",
			input: "",
			want:  0,
		},
		{
			name: "clients with zero subscriptions are filtered out",
			input: "id=1 addr=10.0.0.1:1111 fd=5 name=app1 age=100 idle=0 flags=N db=0 sub=0 psub=0 multi=-1 qbuf=0 obl=0 oll=0 events=r cmd=get\n" +
				"id=2 addr=10.0.0.2:2222 fd=6 name=app2 age=200 idle=0 flags=N db=0 sub=0 psub=0 multi=-1 qbuf=0 obl=0 oll=0 events=r cmd=set",
			want: 0,
		},
		{
			name:  "malformed line is skipped gracefully",
			input: "garbage data without equals signs\nid=1 addr=10.0.0.1:1234 name=valid sub=1 psub=0",
			want:  1,
			checks: func(t *testing.T, clients []PubSubClient) {
				t.Helper()
				if clients[0].Name != "valid" {
					t.Errorf("expected name 'valid', got %q", clients[0].Name)
				}
			},
		},
		{
			name:  "missing addr field defaults to 'unknown'",
			input: "id=1 name=noaddr sub=5 psub=0",
			want:  1,
			checks: func(t *testing.T, clients []PubSubClient) {
				t.Helper()
				if clients[0].Addr != "unknown" {
					t.Errorf("expected addr 'unknown', got %q", clients[0].Addr)
				}
			},
		},
		{
			name:  "leading/trailing whitespace is trimmed",
			input: "\n  id=1 addr=10.0.0.1:5555 name=trimmed sub=2 psub=1  \n\n  id=2 addr=10.0.0.2:6666 name=trimmed2 sub=0 psub=0  \n",
			want:  1,
			checks: func(t *testing.T, clients []PubSubClient) {
				t.Helper()
				assertClient(t, clients[0], "trimmed", "10.0.0.1:5555", 2, 1)
			},
		},
		{
			name:  "value containing equals sign is parsed correctly",
			input: "id=1 addr=10.0.0.1:1234 name=app=v2 sub=1 psub=0",
			want:  1,
			checks: func(t *testing.T, clients []PubSubClient) {
				t.Helper()
				// name should be "app=v2" (split on first '=' only)
				if clients[0].Name != "app=v2" {
					t.Errorf("expected name 'app=v2', got %q", clients[0].Name)
				}
			},
		},
		{
			name:  "large sub count is parsed",
			input: "id=1 addr=10.0.0.1:1234 name=heavy sub=9999 psub=500",
			want:  1,
			checks: func(t *testing.T, clients []PubSubClient) {
				t.Helper()
				assertClient(t, clients[0], "heavy", "10.0.0.1:1234", 9999, 500)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clients := ParseClientList(tt.input)
			if len(clients) != tt.want {
				t.Fatalf("expected %d clients, got %d", tt.want, len(clients))
			}
			if tt.checks != nil {
				tt.checks(t, clients)
			}
		})
	}
}

func assertClient(t *testing.T, got PubSubClient, name, addr string, sub, psub int) {
	t.Helper()
	if got.Name != name {
		t.Errorf("name: want %q, got %q", name, got.Name)
	}
	if got.Addr != addr {
		t.Errorf("addr: want %q, got %q", addr, got.Addr)
	}
	if got.Sub != sub {
		t.Errorf("sub: want %d, got %d", sub, got.Sub)
	}
	if got.PSub != psub {
		t.Errorf("psub: want %d, got %d", psub, got.PSub)
	}
}

func BenchmarkParseClientList(b *testing.B) {
	// Simulate 100 clients, 20 with subscriptions
	var lines string
	for i := 0; i < 100; i++ {
		sub, psub := 0, 0
		if i%5 == 0 {
			sub = 3
			psub = 1
		}
		lines += "id=" + string(rune('0'+i%10)) + " addr=10.0.0.1:" + string(rune('0'+i%10)) + "000 name=client-" + string(rune('0'+i%10)) +
			" sub=" + itoa(sub) + " psub=" + itoa(psub) + " flags=N db=0\n"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseClientList(lines)
	}
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
