package main

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture HTTP status codes for logging.
// This allows us to monitor response status codes in our structured logs.
type responseWriter struct {
	http.ResponseWriter
	statusCode int // Captured status code
}

// WriteHeader captures the status code before delegating to the underlying ResponseWriter.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// createDynamicProxy creates a reverse proxy for the given request host with dynamic SNI.
// It uses the request domain for both target host and SNI, avoiding loops by using a different port.
// browserHost is the original Host header from the browser (used for Location header rewriting).
// If empty, it defaults to requestHost.
func (ps *ProxyServer) createDynamicProxy(requestHost, browserHost string) *httputil.ReverseProxy {
	if browserHost == "" {
		browserHost = requestHost
	}
	// Extract just the hostname without port for SNI
	sniHostname := requestHost
	if host, _, err := net.SplitHostPort(requestHost); err == nil {
		sniHostname = host
	}

	// Use the request domain for the target connection
	// The key is to use a different port than what the proxy is listening on to avoid loops
	targetURL := &url.URL{
		Scheme: ps.config.TargetScheme,
		Host:   net.JoinHostPort(sniHostname, strconv.Itoa(ps.config.TargetPort)),
	}

	// Initialize reverse proxy with the dynamic target URL
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Configure HTTP transport with dynamic SNI support
	transport := &http.Transport{
		// Connection dialer with reasonable timeouts
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second, // Connection timeout
			KeepAlive: 30 * time.Second, // Keep-alive probe interval
		}).DialContext,

		// TLS configuration with dynamic SNI support
		TLSClientConfig: &tls.Config{
			ServerName:         sniHostname,             // SNI hostname from request domain
			InsecureSkipVerify: ps.config.SkipTLSVerify, // Certificate verification setting
		},

		// Connection pool settings for performance
		MaxIdleConns:          100,              // Maximum idle connections across all hosts
		IdleConnTimeout:       90 * time.Second, // How long idle connections are kept
		TLSHandshakeTimeout:   10 * time.Second, // TLS handshake timeout
		ExpectContinueTimeout: 1 * time.Second,  // Time to wait for 100-continue response
	}

	proxy.Transport = transport

	// Configure custom request director for detailed logging and header management
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		// Apply default director behavior first
		originalDirector(req)

		// Skip verbose logging for health checks to reduce log noise
		isHealthCheck := req.URL.Path == "/health"

		// Collect header information for logging (skip for health checks)
		if !isHealthCheck {
			headerCount := len(req.Header)
			headerNames := make([]string, 0, headerCount)
			for name := range req.Header {
				headerNames = append(headerNames, name)
			}

			// Log detailed request information (debug level)
			ps.logger.Debug("Proxying request",
				"method", req.Method,
				"url", req.URL.String(),
				"remote_addr", req.RemoteAddr,
				"user_agent", req.Header.Get("User-Agent"),
				"target_host", targetURL.Host,
				"sni_hostname", sniHostname,
				"request_host", requestHost,
				"header_count", headerCount,
				"headers", headerNames,
			)
		}

		// Preserve the original Host header from the request
		// The backend server should see the original requested hostname
		req.Host = requestHost

		// Log authentication and session information (debug level, skip for health checks)
		if !isHealthCheck {
			if auth := req.Header.Get("Authorization"); auth != "" {
				ps.logger.Debug("Forwarding Authorization header", "auth_type", strings.Split(auth, " ")[0])
			}
			if cookie := req.Header.Get("Cookie"); cookie != "" {
				ps.logger.Debug("Forwarding Cookie header", "cookie_count", len(strings.Split(cookie, ";")))
			}
		}
	}

	// Configure response modifier to rewrite redirect Location headers
	// so the browser stays on the proxy instead of following redirects to the backend
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp != nil {
			// Log redirect responses for debugging
			if resp.StatusCode >= 300 && resp.StatusCode < 400 {
				ps.logger.Info("Redirect response from backend",
					"status_code", resp.StatusCode,
					"location", resp.Header.Get("Location"),
					"backend_host", sniHostname,
					"browser_host", browserHost,
				)
			}

			// Rewrite Location header in redirect responses (3xx)
			if location := resp.Header.Get("Location"); location != "" {
				rewritten := ps.rewriteLocationHeader(location, sniHostname, browserHost)
				if rewritten != location {
					resp.Header.Set("Location", rewritten)
					ps.logger.Info("Rewrote Location header",
						"original", location,
						"rewritten", rewritten,
					)
				}
			}

			// Collect response header information
			responseHeaderCount := len(resp.Header)
			responseHeaderNames := make([]string, 0, responseHeaderCount)
			for name := range resp.Header {
				responseHeaderNames = append(responseHeaderNames, name)
			}

			// Log response details at debug level
			ps.logger.Debug("Response received from target",
				"status_code", resp.StatusCode,
				"status", resp.Status,
				"target_host", targetURL.Host,
				"response_header_count", responseHeaderCount,
				"response_headers", responseHeaderNames,
			)
		}
		return nil
	}

	// Configure error handler for proxy failures
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		// Log detailed error information
		ps.logger.Error("Proxy error occurred",
			"error", err.Error(),
			"method", r.Method,
			"url", r.URL.String(),
			"remote_addr", r.RemoteAddr,
			"target_host", targetURL.Host,
		)
		// Return standard HTTP 502 Bad Gateway response
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	return proxy
}

// rewriteLocationHeader rewrites a Location header URL so redirect responses
// keep the browser on the proxy instead of redirecting to the backend directly.
// It replaces the backend's scheme+host with the proxy's scheme+host.
func (ps *ProxyServer) rewriteLocationHeader(location, backendHost, originalRequestHost string) string {
	parsed, err := url.Parse(location)
	if err != nil {
		return location
	}

	// Only rewrite absolute URLs that point to the backend
	if parsed.Host == "" {
		// Relative redirect — already stays on the proxy
		return location
	}

	// Normalize hostnames by stripping "www." prefix for comparison
	normalizeHost := func(h string) string {
		return strings.TrimPrefix(strings.ToLower(h), "www.")
	}

	locationHost := normalizeHost(parsed.Hostname())
	normalizedBackend := normalizeHost(backendHost)
	normalizedTarget := normalizeHost(ps.config.TargetHost)

	// Check if the redirect points to the backend or the configured target host
	if locationHost == normalizedBackend || locationHost == normalizedTarget {
		// Determine the proxy's own scheme and host
		proxyScheme := "http"
		if ps.config.EnableTLS {
			proxyScheme = "https"
		}
		proxyHost := originalRequestHost
		// If originalRequestHost doesn't have a port, add the listen port
		if _, _, err := net.SplitHostPort(proxyHost); err != nil {
			proxyHost = net.JoinHostPort(proxyHost, strconv.Itoa(ps.config.ListenPort))
		}

		parsed.Scheme = proxyScheme
		parsed.Host = proxyHost
		return parsed.String()
	}

	ps.logger.Debug("Location header not rewritten (host mismatch)",
		"location", location,
		"location_host", locationHost,
		"backend_host", normalizedBackend,
		"target_host", normalizedTarget,
	)
	return location
}
