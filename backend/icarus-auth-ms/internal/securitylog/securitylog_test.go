package securitylog

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestSecurityLogger(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "sec_log_test_*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	os.Setenv("SECURITY_LOG_FILE", tmpFile.Name())
	defer os.Unsetenv("SECURITY_LOG_FILE")

	logger := NewLogger("test-service")
	defer logger.Close()

	ctx := context.Background()
	logger.LogEvent(ctx, Event{
		EventType:      DomainAuth,
		Action:         "LOGIN_SUCCESS",
		Severity:       SeverityInfo,
		Actor:          "admin",
		ActorIP:        "127.0.0.1",
		UserAgent:      "Go-Test",
		TargetResource: "user:1",
		Status:         StatusSuccess,
		Details:        map[string]interface{}{"method": "password"},
	})

	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !bytes.Contains(content, []byte(`"sourcetype":"icarus:security"`)) {
		t.Errorf("Expected log file to contain sourcetype, got: %s", string(content))
	}
	if !bytes.Contains(content, []byte(`"service":"test-service"`)) {
		t.Errorf("Expected log file to contain service name, got: %s", string(content))
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(content), &entry); err != nil {
		t.Fatalf("Failed to parse log line as JSON: %v", err)
	}

	if entry["event_type"] != DomainAuth {
		t.Errorf("Expected event_type %s, got %v", DomainAuth, entry["event_type"])
	}
	if entry["actor"] != "admin" {
		t.Errorf("Expected actor 'admin', got %v", entry["actor"])
	}
}
