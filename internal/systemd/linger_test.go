//go:build linux

package systemd

import (
	"fmt"
	"os"
	"testing"
)

func TestCheckLinger_Enabled(t *testing.T) {
	user := os.Getenv("USER")
	mock := &MockCommander{}
	mock.OnCall("loginctl show-user "+user+" --property=Linger", MockResponse{
		Stdout: "Linger=yes\n",
	})

	enabled, err := CheckLinger(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Fatal("expected linger to be enabled")
	}
}

func TestCheckLinger_Disabled(t *testing.T) {
	user := os.Getenv("USER")
	mock := &MockCommander{}
	mock.OnCall("loginctl show-user "+user+" --property=Linger", MockResponse{
		Stdout: "Linger=no\n",
	})

	enabled, err := CheckLinger(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enabled {
		t.Fatal("expected linger to be disabled")
	}
}

func TestCheckLinger_UnexpectedValue(t *testing.T) {
	user := os.Getenv("USER")
	mock := &MockCommander{}
	mock.OnCall("loginctl show-user "+user+" --property=Linger", MockResponse{
		Stdout: "Linger=maybe\n",
	})

	_, err := CheckLinger(mock)
	if err == nil {
		t.Fatal("expected error for unexpected value")
	}
}

func TestCheckLinger_CommandError(t *testing.T) {
	user := os.Getenv("USER")
	mock := &MockCommander{}
	mock.OnCall("loginctl show-user "+user+" --property=Linger", MockResponse{
		Err: fmt.Errorf("loginctl: failed"),
	})

	_, err := CheckLinger(mock)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEnableLinger_Success(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("loginctl enable-linger", MockResponse{})

	err := EnableLinger(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnableLinger_Failure(t *testing.T) {
	mock := &MockCommander{}
	mock.OnCall("loginctl enable-linger", MockResponse{
		Err: fmt.Errorf("loginctl: permission denied"),
	})

	err := EnableLinger(mock)
	if err == nil {
		t.Fatal("expected error")
	}
}
