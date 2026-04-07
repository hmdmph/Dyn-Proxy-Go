package main

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

// isSelfReferencing checks if the request host points back to the proxy itself.
// This prevents the proxy from trying to connect to localhost:443 when the browser
// sends requests with Host: localhost:8080.
func (ps *ProxyServer) isSelfReferencing(host string) bool {
	hostname, port, err := net.SplitHostPort(host)
	if err != nil {
		// No port in host header, just use the hostname
		hostname = host
		port = ""
	}

	// Check if the hostname is a loopback or local address
	isLocal := hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" || hostname == "0.0.0.0"

	if !isLocal {
		return false
	}

	// If there's a port and it matches our listen port, it's definitely self-referencing
	if port != "" {
		p, _ := strconv.Atoi(port)
		return p == ps.config.ListenPort
	}

	// Local hostname without port — likely self-referencing
	return true
}

// healthHandler provides a health check endpoint for monitoring and Kubernetes probes.
// Returns JSON response with status and timestamp.
func (ps *ProxyServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	ps.logger.Debug("Health check request received", "remote_addr", r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

// rootHandler serves the proxy dashboard at the root URL or handles proxy requests
func (ps *ProxyServer) rootHandler(w http.ResponseWriter, r *http.Request) {
	// If the path is exactly "/", serve the proxy dashboard
	if r.URL.Path == "/" {
		ps.proxyListHandler(w, r)
		return
	}

	// For all other paths, use the existing proxy handler
	ps.proxyHandler(w, r)
}

// proxyListHandler serves the HTML page with the list of available proxies
func (ps *ProxyServer) proxyListHandler(w http.ResponseWriter, r *http.Request) {
	ps.logger.Info("Proxy dashboard requested", "remote_addr", r.RemoteAddr, "user_agent", r.Header.Get("User-Agent"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if ps.template == nil {
		ps.logger.Error("HTML template not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Create template data with both proxy list and config
	templateData := struct {
		*ProxyListConfig
		PageTitle     string
		SubTitle      string
		PageGradient  string
		PageTitleIcon string
	}{
		ProxyListConfig: ps.config.ProxyList,
		PageTitle:       ps.config.PageTitle,
		SubTitle:        ps.config.SubTitle,
		PageGradient:    ps.config.PageGradient,
		PageTitleIcon:   ps.config.PageTitleIcon,
	}

	if err := ps.template.Execute(w, templateData); err != nil {
		ps.logger.Error("Failed to execute template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ps.logger.Debug("Proxy dashboard served successfully", "proxy_count", len(ps.config.ProxyList.ProxyList))
}

// proxyHandler processes all incoming requests and forwards them to the target server.
// It extracts the domain from the request Host header (HTTP/1.1) or :authority pseudo-header (HTTP/2)
// and uses it for both target host and SNI. Health check requests are only logged in debug mode to reduce log noise.
func (ps *ProxyServer) proxyHandler(w http.ResponseWriter, r *http.Request) {
	// Skip proxy handling for health check
	if r.URL.Path == "/health" {
		http.NotFound(w, r)
		return
	}

	// Check if this is a health check request to reduce log noise
	isHealthCheck := r.URL.Path == "/health"

	// Start timing the request for performance monitoring
	start := time.Now()

	// Extract the target host from the request Host header or :authority pseudo-header (HTTP/2)
	targetHost := r.Host
	hostSource := "Host"
	if targetHost == "" {
		// For HTTP/2, check the :authority pseudo-header
		if authority := r.Header.Get(":authority"); authority != "" {
			targetHost = authority
			hostSource = ":authority"
		}
	}
	if targetHost == "" {
		ps.logger.Error("No Host header or :authority pseudo-header found in request", "remote_addr", r.RemoteAddr)
		http.Error(w, "Bad Request: Missing Host header or :authority pseudo-header", http.StatusBadRequest)
		return
	}

	// Detect self-referencing requests (e.g. browser sending Host: localhost:8080)
	// For root path, serve the dashboard; for other paths, proxy to the configured TargetHost
	if ps.isSelfReferencing(targetHost) {
		if r.URL.Path == "/" {
			ps.logger.Debug("Self-referencing request detected, serving dashboard", "request_host", targetHost)
			ps.proxyListHandler(w, r)
			return
		}
		// Use the configured TargetHost for non-root self-referencing requests
		targetHost = ps.config.TargetHost
		hostSource = "TargetHost(self-referencing)"
		ps.logger.Debug("Self-referencing request, routing to configured target host",
			"original_host", r.Host, "target_host", targetHost, "path", r.URL.Path)
	}

	// Log incoming request details (skip health checks unless debug mode)
	if !isHealthCheck {
		ps.logger.Info("Request received",
			"method", r.Method,
			"url", r.URL.String(),
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"user_agent", r.Header.Get("User-Agent"),
			"request_host", targetHost,
			"host_source", hostSource,
		)
	} else {
		ps.logger.Debug("Health check request received",
			"method", r.Method,
			"remote_addr", r.RemoteAddr,
		)
	}

	// Create dynamic proxy for this specific target host
	// Pass original browser Host for Location header rewriting
	dynamicProxy := ps.createDynamicProxy(targetHost, r.Host)

	// Wrap response writer to capture status code for logging
	wrappedWriter := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	// Forward request to target server via dynamic reverse proxy
	dynamicProxy.ServeHTTP(wrappedWriter, r)

	// Calculate and log request completion metrics (skip health checks unless debug mode)
	duration := time.Since(start)
	if !isHealthCheck {
		ps.logger.Info("Request completed",
			"method", r.Method,
			"url", r.URL.String(),
			"remote_addr", r.RemoteAddr,
			"request_host", targetHost,
			"host_source", hostSource,
			"status_code", wrappedWriter.statusCode,
			"duration_ms", duration.Milliseconds(),
		)
	} else {
		ps.logger.Debug("Health check completed",
			"status_code", wrappedWriter.statusCode,
			"duration_ms", duration.Milliseconds(),
		)
	}
}

