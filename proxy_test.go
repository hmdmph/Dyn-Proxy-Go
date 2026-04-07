package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestConfig returns a Config suitable for testing with sensible defaults.
func newTestConfig() *Config {
	return &Config{
		ListenPort:    0, // Use any available port
		TargetPort:    443,
		TargetScheme:  "https",
		TargetHost:    "example.com",
		SNI:           "example.com",
		LogLevel:      "error",
		SkipTLSVerify: true,
		ReadTimeout:   5 * time.Second,
		WriteTimeout:  5 * time.Second,
		IdleTimeout:   10 * time.Second,
	}
}

// newTestProxyServer creates a ProxyServer configured for testing.
func newTestProxyServer(config *Config) *ProxyServer {
	return NewProxyServer(config)
}

// --- Health Check Tests ---

func TestHealthHandler_ReturnsOK(t *testing.T) {
	ps := newTestProxyServer(newTestConfig())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	ps.healthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var result map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse health response JSON: %v", err)
	}

	if result["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", result["status"])
	}

	if _, ok := result["timestamp"]; !ok {
		t.Error("expected 'timestamp' field in health response")
	}
}

// --- Dashboard / Root Handler Tests ---

func TestRootHandler_ServesDashboard(t *testing.T) {
	config := newTestConfig()
	config.PageTitle = "Test Dashboard"
	config.SubTitle = "Test Subtitle"
	config.ProxyListYAML = `proxyList:
  - name: TestProxy
    path: /test`

	ps := newTestProxyServer(config)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	ps.rootHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	body := rec.Body.String()

	if !strings.Contains(body, "Test Dashboard") {
		t.Error("expected dashboard to contain page title 'Test Dashboard'")
	}
	if !strings.Contains(body, "Test Subtitle") {
		t.Error("expected dashboard to contain subtitle 'Test Subtitle'")
	}
	if !strings.Contains(body, "TestProxy") {
		t.Error("expected dashboard to contain proxy name 'TestProxy'")
	}
}

func TestRootHandler_EmptyProxyList(t *testing.T) {
	config := newTestConfig()
	ps := newTestProxyServer(config)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	ps.rootHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "No proxies configured") {
		t.Error("expected 'No proxies configured' message for empty proxy list")
	}
}

// --- Proxy Handler Tests ---

func TestProxyHandler_MissingHost(t *testing.T) {
	ps := newTestProxyServer(newTestConfig())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "" // Clear Host header
	rec := httptest.NewRecorder()

	ps.proxyHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestProxyHandler_ProxiesToBackend(t *testing.T) {
	// Create a test backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-Host", r.Host)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "backend response")
	}))
	defer backend.Close()

	// Extract backend host and port
	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	parts := strings.Split(backendAddr, ":")
	backendPort := 80
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &backendPort)
	}

	config := newTestConfig()
	config.TargetScheme = "http"
	config.TargetPort = backendPort

	ps := newTestProxyServer(config)

	req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
	// Use full host:port so isSelfReferencing sees a different port than ListenPort
	req.Host = backendAddr
	rec := httptest.NewRecorder()

	ps.proxyHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	body := rec.Body.String()
	if body != "backend response" {
		t.Errorf("expected 'backend response', got '%s'", body)
	}
}

func TestProxyHandler_PreservesHostHeader(t *testing.T) {
	var receivedHost string

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	parts := strings.Split(backendAddr, ":")
	backendPort := 80
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &backendPort)
	}

	config := newTestConfig()
	config.TargetScheme = "http"
	config.TargetPort = backendPort

	ps := newTestProxyServer(config)

	// Send request with full host:port so self-referencing check passes
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = backendAddr
	req.URL.Path = "/some-path"
	rec := httptest.NewRecorder()

	ps.proxyHandler(rec, req)

	if receivedHost != backendAddr {
		t.Errorf("expected backend to receive Host '%s', got '%s'", backendAddr, receivedHost)
	}
}

// --- Response Writer Tests ---

func TestResponseWriter_CapturesStatusCode(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusNotFound)

	if rw.statusCode != http.StatusNotFound {
		t.Errorf("expected captured status %d, got %d", http.StatusNotFound, rw.statusCode)
	}
}

func TestResponseWriter_DefaultStatusCode(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	if rw.statusCode != http.StatusOK {
		t.Errorf("expected default status %d, got %d", http.StatusOK, rw.statusCode)
	}
}

// --- Model Tests ---

func TestParseProxyList_ValidYAML(t *testing.T) {
	yaml := `proxyList:
  - name: API
    path: /api
  - name: Web
    path: /web`

	result, err := parseProxyList(yaml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.ProxyList) != 2 {
		t.Fatalf("expected 2 proxy entries, got %d", len(result.ProxyList))
	}

	if result.ProxyList[0].Name != "API" {
		t.Errorf("expected name 'API', got '%s'", result.ProxyList[0].Name)
	}
	if result.ProxyList[0].Path != "/api" {
		t.Errorf("expected path '/api', got '%s'", result.ProxyList[0].Path)
	}
	if result.ProxyList[1].Name != "Web" {
		t.Errorf("expected name 'Web', got '%s'", result.ProxyList[1].Name)
	}

	// Each entry should have a random icon assigned
	for i, entry := range result.ProxyList {
		if entry.Icon == "" {
			t.Errorf("entry %d should have a random icon assigned", i)
		}
	}
}

func TestParseProxyList_EmptyYAML(t *testing.T) {
	result, err := parseProxyList("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ProxyList) != 0 {
		t.Errorf("expected 0 proxy entries for empty YAML, got %d", len(result.ProxyList))
	}
}

func TestParseProxyList_InvalidYAML(t *testing.T) {
	_, err := parseProxyList("not: [valid: yaml: {{")
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

// --- Config Tests ---

func TestGetEnvString(t *testing.T) {
	t.Setenv("TEST_STRING_VAR", "hello")
	if got := getEnvString("TEST_STRING_VAR", "default"); got != "hello" {
		t.Errorf("expected 'hello', got '%s'", got)
	}
	if got := getEnvString("NONEXISTENT_VAR", "default"); got != "default" {
		t.Errorf("expected 'default', got '%s'", got)
	}
}

func TestGetEnvInt(t *testing.T) {
	t.Setenv("TEST_INT_VAR", "42")
	if got := getEnvInt("TEST_INT_VAR", 0); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
	if got := getEnvInt("NONEXISTENT_VAR", 99); got != 99 {
		t.Errorf("expected 99, got %d", got)
	}

	t.Setenv("TEST_INT_INVALID", "notanumber")
	if got := getEnvInt("TEST_INT_INVALID", 10); got != 10 {
		t.Errorf("expected 10 for invalid int, got %d", got)
	}
}

func TestGetEnvBool(t *testing.T) {
	t.Setenv("TEST_BOOL_TRUE", "true")
	t.Setenv("TEST_BOOL_FALSE", "false")

	if got := getEnvBool("TEST_BOOL_TRUE", false); !got {
		t.Error("expected true, got false")
	}
	if got := getEnvBool("TEST_BOOL_FALSE", true); got {
		t.Error("expected false, got true")
	}
	if got := getEnvBool("NONEXISTENT_VAR", true); !got {
		t.Error("expected default true, got false")
	}

	t.Setenv("TEST_BOOL_INVALID", "notabool")
	if got := getEnvBool("TEST_BOOL_INVALID", false); got {
		t.Error("expected false for invalid bool, got true")
	}
}

// --- Server Lifecycle Tests ---

func TestNewProxyServer_InitializesCorrectly(t *testing.T) {
	config := newTestConfig()
	config.ProxyListYAML = `proxyList:
  - name: Test
    path: /test`

	ps := newTestProxyServer(config)

	if ps.config == nil {
		t.Fatal("expected config to be set")
	}
	if ps.logger == nil {
		t.Fatal("expected logger to be set")
	}
	if ps.template == nil {
		t.Fatal("expected template to be set")
	}
	if ps.config.ProxyList == nil {
		t.Fatal("expected proxy list to be parsed")
	}
	if len(ps.config.ProxyList.ProxyList) != 1 {
		t.Errorf("expected 1 proxy entry, got %d", len(ps.config.ProxyList.ProxyList))
	}
}

func TestProxyServer_StartAndStop(t *testing.T) {
	config := newTestConfig()
	config.ListenPort = 0 // Let OS assign port

	ps := newTestProxyServer(config)

	// Start in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- ps.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := ps.Stop(ctx); err != nil {
		t.Fatalf("failed to stop server: %v", err)
	}

	// Check server returned without unexpected error
	err := <-errCh
	if err != nil && err != http.ErrServerClosed {
		t.Errorf("unexpected server error: %v", err)
	}
}

// --- Integration Test: Full HTTP round-trip through proxy ---

func TestIntegration_ProxyRoundTrip(t *testing.T) {
	// Create a backend that echoes request info as JSON
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]string{
			"method": r.Method,
			"path":   r.URL.Path,
			"host":   r.Host,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer backend.Close()

	// Parse backend address (full host:port)
	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	parts := strings.Split(backendAddr, ":")
	backendPort := 80
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &backendPort)
	}

	// Configure proxy to forward to test backend
	config := newTestConfig()
	config.TargetScheme = "http"
	config.TargetPort = backendPort
	config.ListenPort = 0

	ps := newTestProxyServer(config)

	// Create test mux the same way Start() does
	mux := http.NewServeMux()
	mux.HandleFunc("/health", ps.healthHandler)
	mux.HandleFunc("/", ps.rootHandler)

	proxyServer := httptest.NewServer(mux)
	defer proxyServer.Close()

	// Test 1: Health check
	t.Run("health_check", func(t *testing.T) {
		resp, err := http.Get(proxyServer.URL + "/health")
		if err != nil {
			t.Fatalf("health request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected %d, got %d", http.StatusOK, resp.StatusCode)
		}
	})

	// Test 2: Dashboard
	t.Run("dashboard", func(t *testing.T) {
		resp, err := http.Get(proxyServer.URL + "/")
		if err != nil {
			t.Fatalf("dashboard request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected %d, got %d", http.StatusOK, resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Dashboard") {
			t.Error("expected dashboard HTML response")
		}
	})

	// Test 3: Proxy request to backend (use full host:port so self-referencing check passes)
	t.Run("proxy_to_backend", func(t *testing.T) {
		client := &http.Client{}
		req, _ := http.NewRequest(http.MethodGet, proxyServer.URL+"/api/test", nil)
		req.Host = backendAddr

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("proxy request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected %d, got %d", http.StatusOK, resp.StatusCode)
		}

		var result map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if result["path"] != "/api/test" {
			t.Errorf("expected path '/api/test', got '%s'", result["path"])
		}
		if result["method"] != "GET" {
			t.Errorf("expected method 'GET', got '%s'", result["method"])
		}
	})
}

// --- GetRandomIcon Test ---

func TestGetRandomIcon_ReturnsNonEmpty(t *testing.T) {
	for i := 0; i < 10; i++ {
		icon := getRandomIcon()
		if icon == "" {
			t.Error("getRandomIcon returned empty string")
		}
	}
}
