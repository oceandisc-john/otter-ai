package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- NewRateLimiter ---

func TestNewRateLimiter_Defaults(t *testing.T) {
	rl := NewRateLimiter(0, 0)
	if rl.limit != DefaultRateLimit {
		t.Errorf("limit = %d; want %d", rl.limit, DefaultRateLimit)
	}
	if rl.window != DefaultRateLimitWindow {
		t.Errorf("window = %v; want %v", rl.window, DefaultRateLimitWindow)
	}
}

func TestNewRateLimiter_Custom(t *testing.T) {
	rl := NewRateLimiter(10, 5*time.Second)
	if rl.limit != 10 {
		t.Errorf("limit = %d", rl.limit)
	}
	if rl.window != 5*time.Second {
		t.Errorf("window = %v", rl.window)
	}
}

// --- Allow ---

func TestAllow_UnderLimit(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	for i := 0; i < 5; i++ {
		if !rl.Allow("client1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
}

func TestAllow_OverLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		rl.Allow("client1")
	}
	if rl.Allow("client1") {
		t.Error("4th request should be denied")
	}
}

func TestAllow_DifferentClients(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	if !rl.Allow("client1") {
		t.Error("client1 should be allowed")
	}
	if rl.Allow("client1") {
		t.Error("client1 second request should be denied")
	}
	if !rl.Allow("client2") {
		t.Error("client2 should be allowed")
	}
}

// --- Middleware ---

func TestMiddleware_Allowed(t *testing.T) {
	rl := NewRateLimiter(10, time.Minute)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", w.Code)
	}
}

func TestMiddleware_RateLimited(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"

	// First request ok
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("first request status = %d", w.Code)
	}

	// Second request rate limited
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second request status = %d; want 429", w.Code)
	}
}

// --- getClientIP ---

func TestGetClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	ip := getClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("ip = %q; want 10.0.0.1", ip)
	}
}

func TestGetClientIP_XForwardedFor_WithPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1:8080")
	ip := getClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("ip = %q; want 10.0.0.1", ip)
	}
}

func TestGetClientIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Real-IP", "192.168.1.1")
	ip := getClientIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("ip = %q; want 192.168.1.1", ip)
	}
}

func TestGetClientIP_XRealIP_WithPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Real-IP", "192.168.1.1:9090")
	ip := getClientIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("ip = %q; want 192.168.1.1", ip)
	}
}

func TestGetClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "172.16.0.1:5555"
	ip := getClientIP(req)
	if ip != "172.16.0.1" {
		t.Errorf("ip = %q; want 172.16.0.1", ip)
	}
}

func TestGetClientIP_RemoteAddr_NoPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "172.16.0.1"
	ip := getClientIP(req)
	if ip != "172.16.0.1" {
		t.Errorf("ip = %q; want 172.16.0.1", ip)
	}
}
