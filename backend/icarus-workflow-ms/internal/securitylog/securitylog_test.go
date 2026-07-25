package securitylog

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestSecurityLoggerWorkflow(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "sec_log_test_workflow_*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	os.Setenv("SECURITY_LOG_FILE", tmpFile.Name())
	defer os.Unsetenv("SECURITY_LOG_FILE")

	logger := NewLogger("icarus-workflow-ms")
	defer logger.Close()

	ctx := context.Background()
	logger.LogEvent(ctx, Event{
		EventType:      DomainWorkflow,
		Action:         "SUBMIT_CART",
		Severity:       SeverityInfo,
		Actor:          "john_doe",
		ActorIP:        "127.0.0.1",
		UserAgent:      "Go-Test",
		TargetResource: "cart:101",
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

	if entry["service"] != "icarus-workflow-ms" {
		t.Errorf("Expected service 'icarus-workflow-ms', got %v", entry["service"])
	}
	if entry["event_type"] != DomainWorkflow {
		t.Errorf("Expected event_type %s, got %v", DomainWorkflow, entry["event_type"])
	}
}
