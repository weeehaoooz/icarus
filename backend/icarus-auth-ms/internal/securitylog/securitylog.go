package securitylog

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Event Domains
const (
	DomainAuth           = "AUTH"
	DomainAccessControl  = "ACCESS_CONTROL"
	DomainUserMgmt       = "USER_MGMT"
	DomainRoleMgmt       = "ROLE_MGMT"
	DomainClientMgmt     = "CLIENT_MGMT"
	DomainModuleMgmt     = "MODULE_MGMT"
	DomainTenantMgmt     = "TENANT_MGMT"
	DomainWorkflow       = "WORKFLOW"
	DomainGateway        = "GATEWAY"
	DomainSecurityConfig = "SECURITY_CONFIG"
	DomainHousekeeping   = "HOUSEKEEPING"
)

// Event Severities
const (
	SeverityInfo     = "INFO"
	SeverityWarn     = "WARN"
	SeverityError    = "ERROR"
	SeverityCritical = "CRITICAL"
)

// Event Statuses
const (
	StatusSuccess = "SUCCESS"
	StatusFailure = "FAILURE"
)

// Event defines a structured security log entry compatible with Splunk CIM.
type Event struct {
	EventType      string                 `json:"event_type"`
	Action         string                 `json:"action"`
	Severity       string                 `json:"severity"`
	Actor          string                 `json:"actor,omitempty"`
	ActorIP        string                 `json:"actor_ip,omitempty"`
	UserAgent      string                 `json:"user_agent,omitempty"`
	TargetResource string                 `json:"target_resource,omitempty"`
	Status         string                 `json:"status"`
	Details        map[string]interface{} `json:"details,omitempty"`
}

// Logger encapsulates structured security logging.
type Logger struct {
	serviceName string
	slogger     *slog.Logger
	fileWriter  io.Closer
	mu          sync.Mutex
}

// NewLogger creates a security logger for a microservice.
// It logs to stdout by default, and optionally appends to the file path in env SECURITY_LOG_FILE.
func NewLogger(serviceName string) *Logger {
	var writers []io.Writer
	writers = append(writers, os.Stdout)

	var fileCloser io.Closer
	logFilePath := os.Getenv("SECURITY_LOG_FILE")
	if logFilePath != "" {
		f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			writers = append(writers, f)
			fileCloser = f
		}
	}

	multiWriter := io.MultiWriter(writers...)
	handler := slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	return &Logger{
		serviceName: serviceName,
		slogger:     slog.New(handler),
		fileWriter:  fileCloser,
	}
}

// LogEvent records a security event into the structured log stream.
func (l *Logger) LogEvent(ctx context.Context, e Event) {
	if l == nil || l.slogger == nil {
		return
	}

	severity := e.Severity
	if severity == "" {
		severity = SeverityInfo
	}

	status := e.Status
	if status == "" {
		status = StatusSuccess
	}

	attrs := []slog.Attr{
		slog.String("sourcetype", "icarus:security"),
		slog.String("service", l.serviceName),
		slog.String("timestamp", time.Now().UTC().Format(time.RFC3339Nano)),
		slog.String("event_type", e.EventType),
		slog.String("action", e.Action),
		slog.String("severity", severity),
		slog.String("status", status),
	}

	if e.Actor != "" {
		attrs = append(attrs, slog.String("actor", e.Actor))
	}
	if e.ActorIP != "" {
		attrs = append(attrs, slog.String("actor_ip", e.ActorIP))
	}
	if e.UserAgent != "" {
		attrs = append(attrs, slog.String("user_agent", e.UserAgent))
	}
	if e.TargetResource != "" {
		attrs = append(attrs, slog.String("target_resource", e.TargetResource))
	}

	if len(e.Details) > 0 {
		for k, v := range e.Details {
			attrs = append(attrs, slog.Any("details."+k, v))
		}
	}

	var level slog.Level
	switch severity {
	case SeverityWarn:
		level = slog.LevelWarn
	case SeverityError, SeverityCritical:
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	l.slogger.LogAttrs(ctx, level, e.Action, attrs...)
}

// Close releases any allocated file resources.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fileWriter != nil {
		return l.fileWriter.Close()
	}
	return nil
}

// GetClientIP extracts client IP address from HTTP request headers or remote address.
func GetClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xreal := r.Header.Get("X-Real-IP"); xreal != "" {
		return strings.TrimSpace(xreal)
	}
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}
