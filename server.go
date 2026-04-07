package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
)

//go:embed templates/dashboard.html
var templateFS embed.FS

// ProxyServer encapsulates all components needed for the HTTP proxy service.
// It manages the HTTP server, reverse proxy, logging, and graceful shutdown.
type ProxyServer struct {
	config   *Config                // Configuration parameters
	logger   *slog.Logger           // Structured logger instance
	server   *http.Server           // HTTP server instance
	proxy    *httputil.ReverseProxy // Reverse proxy handler
	template *template.Template     // HTML template for proxy list page
}

// NewProxyServer creates and configures a new proxy server instance.
// It sets up structured logging and creates a dynamic proxy that uses the request domain
// for both target host and SNI configuration.
func NewProxyServer(config *Config) *ProxyServer {
	// Initialize structured JSON logging with configurable level
	var logLevel slog.Level
	switch strings.ToLower(config.LogLevel) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Verify logger initialization
	logger.Info("Logger initialized", "log_level", config.LogLevel, "effective_level", logLevel.String())

	// Parse proxy list configuration
	proxyList, err := parseProxyList(config.ProxyListYAML)
	if err != nil {
		logger.Error("Failed to parse proxy list", "error", err)
		proxyList = &ProxyListConfig{ProxyList: []ProxyEntry{}}
	}
	config.ProxyList = proxyList

	// Load HTML template from embedded filesystem
	tmpl, err := template.ParseFS(templateFS, "templates/dashboard.html")
	if err != nil {
		logger.Error("Failed to parse HTML template", "error", err)
	}

	return &ProxyServer{
		config:   config,
		logger:   logger,
		proxy:    nil, // We'll create dynamic proxies per request
		template: tmpl,
	}
}

// Start initializes and starts the HTTP proxy server.
// It sets up the HTTP routes, configures the server with timeouts,
// and starts listening for incoming connections.
func (ps *ProxyServer) Start() error {
	// Create HTTP request multiplexer for routing
	mux := http.NewServeMux()

	// Register health check endpoint for Kubernetes probes
	mux.HandleFunc("/health", ps.healthHandler)

	// Register catch-all handler for both dashboard and proxying requests
	mux.HandleFunc("/", ps.rootHandler)

	ps.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", ps.config.ListenPort),
		Handler:      mux,
		ReadTimeout:  ps.config.ReadTimeout,
		WriteTimeout: ps.config.WriteTimeout,
		IdleTimeout:  ps.config.IdleTimeout,
	}

	ps.logger.Info("Starting dynamic proxy server",
		"listen_port", ps.config.ListenPort,
		"target_port", ps.config.TargetPort,
		"target_scheme", ps.config.TargetScheme,
		"log_level", ps.config.LogLevel,
		"skip_tls_verify", ps.config.SkipTLSVerify,
		"enable_tls", ps.config.EnableTLS,
		"mode", "dynamic_host_and_sni",
	)

	if ps.config.EnableTLS {
		ps.logger.Info("Starting HTTPS server", "cert_file", ps.config.TLSCertFile, "key_file", ps.config.TLSKeyFile)
		return ps.server.ListenAndServeTLS(ps.config.TLSCertFile, ps.config.TLSKeyFile)
	} else {
		ps.logger.Info("Starting HTTP server")
		return ps.server.ListenAndServe()
	}
}

// Stop gracefully shuts down the proxy server.
// It waits for active connections to finish within the provided context timeout.
func (ps *ProxyServer) Stop(ctx context.Context) error {
	ps.logger.Info("Initiating graceful shutdown")
	return ps.server.Shutdown(ctx)
}
