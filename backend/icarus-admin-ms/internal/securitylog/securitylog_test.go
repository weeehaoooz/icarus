package securitylog

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestSecurityLoggerAdmin(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "sec_log_test_admin_*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	os.Setenv("SECURITY_LOG_FILE", tmpFile.Name())
	defer os.Unsetenv("SECURITY_LOG_FILE")

	logger := NewLogger("icarus-admin-ms")
	defer logger.Close()

	ctx := context.Background()
	logger.LogEvent(ctx, Event{
		EventType:      DomainModuleMgmt,
		Action:         "ONBOARD_MODULE",
		Severity:       SeverityInfo,
		Actor:          "admin",
		ActorIP:        "127.0.0.1",
		UserAgent:      "Go-Test",
		TargetResource: "module:finance",
		Status:         StatusSuccess,
	})

	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(content), &entry); err != nil {
		t.Fatalf("Failed to parse log line as JSON: %v", err)
	}

	if entry["service"] != "icarus-admin-ms" {
		t.Errorf("Expected service 'icarus-admin-ms', got %v", entry["service"])
	}
	if entry["event_type"] != DomainModuleMgmt {
		t.Errorf("Expected event_type %s, got %v", DomainModuleMgmt, entry["event_type"])
	}
}
