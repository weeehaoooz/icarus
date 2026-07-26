package ratelimit

import (
	"sync"
	"time"
)

// IPLimiter tracks request timestamps per IP address to enforce rate limits.
type IPLimiter struct {
	mu           sync.Mutex
	requests     map[string][]time.Time
	maxRequests  int
	windowLength time.Duration
}

// NewIPLimiter creates a new IPLimiter.
// maxRequests: max requests allowed within windowLength (e.g. 10 requests per 1 minute).
func NewIPLimiter(maxRequests int, windowLength time.Duration) *IPLimiter {
	limiter := &IPLimiter{
		requests:     make(map[string][]time.Time),
		maxRequests:  maxRequests,
		windowLength: windowLength,
	}

	// Periodically cleanup stale IPs to prevent memory leaks
	go limiter.cleanupLoop()

	return limiter
}

// Allow checks if a request from the given IP address is allowed.
func (l *IPLimiter) Allow(ip string) bool {
	if ip == "" {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.windowLength)

	// Filter out expired timestamps for this IP
	timestamps := l.requests[ip]
	valid := make([]time.Time, 0, len(timestamps))
	for _, t := range timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= l.maxRequests {
		l.requests[ip] = valid
		return false
	}

	valid = append(valid, now)
	l.requests[ip] = valid
	return true
}

func (l *IPLimiter) cleanupLoop() {
	ticker := time.NewTicker(2 * l.windowLength)
	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-l.windowLength)
		for ip, timestamps := range l.requests {
			valid := make([]time.Time, 0, len(timestamps))
			for _, t := range timestamps {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(l.requests, ip)
			} else {
				l.requests[ip] = valid
			}
		}
		l.mu.Unlock()
	}
}

// AttemptTracker tracks failed authentication attempts and account lockouts per username.
type userAttemptInfo struct {
	failures    int
	lockedUntil time.Time
	lastFailure time.Time
}

type AttemptTracker struct {
	mu              sync.Mutex
	attempts        map[string]*userAttemptInfo
	maxFailures     int
	lockoutDuration time.Duration
}

// NewAttemptTracker creates a new AttemptTracker.
// maxFailures: max consecutive failures allowed (e.g. 5)
// lockoutDuration: duration for account lockout (e.g. 15 minutes)
func NewAttemptTracker(maxFailures int, lockoutDuration time.Duration) *AttemptTracker {
	return &AttemptTracker{
		attempts:        make(map[string]*userAttemptInfo),
		maxFailures:     maxFailures,
		lockoutDuration: lockoutDuration,
	}
}

// IsLocked checks whether the username is currently locked out.
func (t *AttemptTracker) IsLocked(username string) (bool, time.Duration) {
	if username == "" {
		return false, 0
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	info, exists := t.attempts[username]
	if !exists {
		return false, 0
	}

	if time.Now().Before(info.lockedUntil) {
		return true, time.Until(info.lockedUntil)
	}

	// Lockout expired
	if info.failures >= t.maxFailures {
		info.failures = 0
		info.lockedUntil = time.Time{}
	}

	return false, 0
}

// RecordFailure records a failed authentication attempt for a username.
// Returns (isNowLocked, currentFailures).
func (t *AttemptTracker) RecordFailure(username string) (bool, int) {
	if username == "" {
		return false, 0
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	info, exists := t.attempts[username]
	if !exists {
		info = &userAttemptInfo{}
		t.attempts[username] = info
	}

	now := time.Now()
	// Reset counter if previous failure was more than 30 minutes ago
	if now.Sub(info.lastFailure) > 30*time.Minute {
		info.failures = 0
	}

	info.failures++
	info.lastFailure = now

	if info.failures >= t.maxFailures {
		info.lockedUntil = now.Add(t.lockoutDuration)
		return true, info.failures
	}

	return false, info.failures
}

// RecordSuccess resets failure counts for a username on successful authentication.
func (t *AttemptTracker) RecordSuccess(username string) {
	if username == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.attempts, username)
}
