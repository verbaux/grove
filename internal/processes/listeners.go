// Package processes discovers local TCP listeners for Grove-assigned ports.
package processes

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"unicode"
)

// Listener identifies one process listening on a TCP port. PID and Command
// may be unavailable when the platform tool only reports socket state.
type Listener struct {
	PID     int    `json:"pid,omitempty"`
	Command string `json:"command,omitempty"`
}

// Snapshot contains one system-wide listener scan and the tool that produced it.
type Snapshot struct {
	Source    string
	Listeners map[int][]Listener
}

type lookPathFunc func(string) (string, error)
type runFunc func(string, ...string) ([]byte, error)

// FindListeners scans local TCP listening sockets using lsof, with netstat as
// a portable fallback. Each tool is invoked once per snapshot.
func FindListeners() (Snapshot, error) {
	return findListeners(exec.LookPath, runTool)
}

func findListeners(lookPath lookPathFunc, run runFunc) (Snapshot, error) {
	foundTool := false
	var failures []string

	if path, err := lookPath("lsof"); err == nil {
		foundTool = true
		output, runErr := run(path, "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn")
		if runErr == nil {
			listeners, parseErr := parseLsof(output)
			if parseErr == nil {
				return Snapshot{Source: "lsof", Listeners: map[int][]Listener(listeners)}, nil
			}
			failures = append(failures, "lsof parse: "+parseErr.Error())
		} else {
			failures = append(failures, "lsof: "+runErr.Error())
		}
	}

	if path, err := lookPath("netstat"); err == nil {
		foundTool = true
		output, runErr := run(path, "-an")
		if runErr == nil {
			listeners, parseErr := parseNetstat(output)
			if parseErr == nil {
				return Snapshot{Source: "netstat", Listeners: map[int][]Listener(listeners)}, nil
			}
			failures = append(failures, "netstat parse: "+parseErr.Error())
		} else {
			failures = append(failures, "netstat: "+runErr.Error())
		}
	}

	if !foundTool {
		return Snapshot{}, fmt.Errorf("cannot inspect listening ports: lsof or netstat is required")
	}
	return Snapshot{}, fmt.Errorf("cannot inspect listening ports: %s", strings.Join(failures, "; "))
}

func runTool(path string, args ...string) ([]byte, error) {
	output, err := exec.Command(path, args...).CombinedOutput()
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 && len(bytes.TrimSpace(output)) == 0 {
		return output, nil
	}
	if err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}
	return output, nil
}

type listenersByPort map[int][]Listener

func parseLsof(output []byte) (listenersByPort, error) {
	listeners := listenersByPort{}
	var current Listener
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			pid, err := strconv.Atoi(line[1:])
			if err != nil {
				current = Listener{}
				continue
			}
			current = Listener{PID: pid}
		case 'c':
			current.Command = sanitizeText(line[1:])
		case 'n':
			if port, ok := portFromEndpoint(line[1:]); ok {
				addListener(listeners, port, current)
			}
		}
	}
	return listeners, scanner.Err()
}

func parseNetstat(output []byte) (listenersByPort, error) {
	listeners := listenersByPort{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 6 || !strings.HasPrefix(strings.ToLower(fields[0]), "tcp") {
			continue
		}
		listening := false
		for _, field := range fields[4:] {
			if strings.EqualFold(field, "LISTEN") || strings.EqualFold(field, "LISTENING") {
				listening = true
				break
			}
		}
		if !listening {
			continue
		}
		if port, ok := portFromEndpoint(fields[3]); ok {
			addListener(listeners, port, Listener{})
		}
	}
	return listeners, scanner.Err()
}

func portFromEndpoint(endpoint string) (int, bool) {
	endpoint = strings.TrimSpace(endpoint)
	separator := strings.LastIndex(endpoint, ":")
	if dot := strings.LastIndex(endpoint, "."); dot > separator {
		separator = dot
	}
	if separator < 0 || separator == len(endpoint)-1 {
		return 0, false
	}
	portText := endpoint[separator+1:]
	if space := strings.IndexAny(portText, " \t"); space >= 0 {
		portText = portText[:space]
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return 0, false
	}
	return port, true
}

func addListener(listeners listenersByPort, port int, listener Listener) {
	for _, existing := range listeners[port] {
		if existing == listener {
			return
		}
	}
	listeners[port] = append(listeners[port], listener)
}

func sanitizeText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, value)
}
