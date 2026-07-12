package processes

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestParseLsofListeners(t *testing.T) {
	output := []byte("p123\ncnode\nn*:3487\nn[::1]:3487\np456\ncgo\x1bserver\nn127.0.0.1:3555\n")

	listeners, err := parseLsof(output)
	if err != nil {
		t.Fatal(err)
	}
	if got := listeners[3487]; len(got) != 1 || got[0].PID != 123 || got[0].Command != "node" {
		t.Fatalf("listeners[3487] = %+v", got)
	}
	if got := listeners[3555]; len(got) != 1 || got[0].PID != 456 || got[0].Command != "go?server" {
		t.Fatalf("listeners[3555] = %+v", got)
	}
}

func TestParseNetstatListenersAcrossPlatforms(t *testing.T) {
	output := []byte(`tcp4       0      0  *.3001                 *.*                    LISTEN
tcp6       0      0  ::1.3002               *.*                    LISTEN
tcp        0      0 127.0.0.1:3003          0.0.0.0:*              LISTEN
tcp        0      0 127.0.0.1:3004          0.0.0.0:*              ESTABLISHED
`)

	listeners, err := parseNetstat(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, port := range []int{3001, 3002, 3003} {
		if got := listeners[port]; len(got) != 1 {
			t.Fatalf("listeners[%d] = %+v", port, got)
		}
	}
	if _, ok := listeners[3004]; ok {
		t.Fatal("established connection should not be reported as a listener")
	}
}

func TestPortFromEndpointRejectsInvalidPorts(t *testing.T) {
	for _, endpoint := range []string{"*:0", "*:65536", "*:*", "no-port"} {
		if _, ok := portFromEndpoint(endpoint); ok {
			t.Fatalf("portFromEndpoint(%q) succeeded", endpoint)
		}
	}
}

func TestFindListenersPrefersLsof(t *testing.T) {
	lookPath := func(name string) (string, error) {
		return "/tools/" + name, nil
	}
	run := func(path string, args ...string) ([]byte, error) {
		if path != "/tools/lsof" {
			t.Fatalf("ran %q, want lsof", path)
		}
		wantArgs := "-nP -iTCP -sTCP:LISTEN -Fpcn"
		if got := strings.Join(args, " "); got != wantArgs {
			t.Fatalf("args = %q, want %q", got, wantArgs)
		}
		return []byte("p12\ncnode\nn*:3487\n"), nil
	}

	snapshot, err := findListeners(lookPath, run)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Source != "lsof" || len(snapshot.Listeners[3487]) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestFindListenersFallsBackToNetstat(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "lsof" {
			return "/tools/lsof", nil
		}
		return "/tools/netstat", nil
	}
	run := func(path string, args ...string) ([]byte, error) {
		if path == "/tools/lsof" {
			return nil, errors.New("lsof failed")
		}
		if got := strings.Join(args, " "); got != "-an" {
			t.Fatalf("netstat args = %q", got)
		}
		return []byte("tcp4 0 0 *.3555 *.* LISTEN\n"), nil
	}

	snapshot, err := findListeners(lookPath, run)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Source != "netstat" || len(snapshot.Listeners[3555]) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestFindListenersReportsUnavailableTools(t *testing.T) {
	lookPath := func(name string) (string, error) {
		return "", fmt.Errorf("%s missing", name)
	}
	_, err := findListeners(lookPath, func(string, ...string) ([]byte, error) {
		t.Fatal("run should not be called")
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "lsof or netstat") {
		t.Fatalf("error = %v", err)
	}
}
