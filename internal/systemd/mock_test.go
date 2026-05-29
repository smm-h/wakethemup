//go:build linux

package systemd

import (
	"context"
	"fmt"
	"strings"
)

// MockResponse holds a predefined response for a MockCommander call.
type MockResponse struct {
	Stdout string
	Stderr string
	Err    error
}

// MockCommander implements Commander for testing. It matches calls by joining
// args with spaces and looking up the result in Responses. If no exact match
// is found, it tries prefix matching (first arg only). Calls are recorded.
type MockCommander struct {
	// Responses maps a key to a response. The key is the full args joined by
	// space (e.g., "systemctl --user daemon-reload"). For convenience, a
	// fallback key of just the first arg is also tried.
	Responses []mockEntry
	Calls     [][]string

	// LookPathFunc is called by LookPath. If nil, returns ("", nil).
	LookPathFunc func(name string) (string, error)
}

type mockEntry struct {
	key      string
	response MockResponse
}

// OnCall registers a response for a specific command string.
func (m *MockCommander) OnCall(argsKey string, resp MockResponse) {
	m.Responses = append(m.Responses, mockEntry{key: argsKey, response: resp})
}

// LookPath returns the result of LookPathFunc, or ("", nil) if not set.
func (m *MockCommander) LookPath(name string) (string, error) {
	if m.LookPathFunc != nil {
		return m.LookPathFunc(name)
	}
	return "", nil
}

// Run looks up the response for the given args.
func (m *MockCommander) Run(_ context.Context, args ...string) (string, string, error) {
	m.Calls = append(m.Calls, args)
	key := strings.Join(args, " ")

	// Try exact match (last registered wins to allow overrides)
	for i := len(m.Responses) - 1; i >= 0; i-- {
		if m.Responses[i].key == key {
			r := m.Responses[i].response
			return r.Stdout, r.Stderr, r.Err
		}
	}

	// Try first-arg match as fallback
	if len(args) > 0 {
		for i := len(m.Responses) - 1; i >= 0; i-- {
			if m.Responses[i].key == args[0] {
				r := m.Responses[i].response
				return r.Stdout, r.Stderr, r.Err
			}
		}
	}

	return "", "", fmt.Errorf("no mock response for: %s", key)
}
