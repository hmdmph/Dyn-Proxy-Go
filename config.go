package main

import (
	"flag"
	"math/rand"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration parameters for the dyn-proxy-go service.
// It supports both command-line flags and environment variables for flexibility.
type Config struct {
	// Server configuration
	ListenPort   int           // Port for the proxy server to listen on
	ReadTimeout  time.Duration // Maximum duration for reading the entire request
	WriteTimeout time.Duration // Maximum duration before timing out writes of the response
	IdleTimeout  time.Duration // Maximum amount of time to wait for the next request

	// Target configuration
	TargetHost   string // Hostname of the target server to proxy requests to
	TargetPort   int    // Port of the target server
	TargetScheme string // Protocol scheme (http/https) for target server
	SNI          string // Server Name Indication for TLS connections

	// Security and TLS configuration
	SkipTLSVerify bool   // Whether to skip TLS certificate verification (use with caution)
	EnableTLS     bool   // Whether to enable TLS/HTTPS for the proxy server itself
	TLSCertFile   string // Path to TLS certificate file for proxy server
	TLSKeyFile    string // Path to TLS private key file for proxy server

	// Logging configuration
	LogLevel string // Logging level (debug, info, warn, error)

	// UI configuration
	PageTitleIcon string // main Icon to just before the title
	PageTitle     string // Page title for the proxy dashboard
	SubTitle      string // Subtitle for the proxy dashboard
	PageGradient  string // Page gradient for the proxy dashboard

	// Proxy list configuration
	ProxyListYAML string           // YAML string containing proxy list
	ProxyList     *ProxyListConfig // Parsed proxy list
}

// parseConfig parses configuration from command-line flags and environment variables.
// Environment variables take precedence over default values, and command-line flags
// take precedence over environment variables.
func parseConfig() *Config {
	config := &Config{}

	flag.IntVar(&config.ListenPort, "port", getEnvInt("LISTEN_PORT", 8080), "Port to listen on")
	flag.StringVar(&config.TargetHost, "target-host", getEnvString("TARGET_HOST", "example.com"), "Target host to proxy to")
	flag.IntVar(&config.TargetPort, "target-port", getEnvInt("TARGET_PORT", 443), "Target port to proxy to")
	flag.StringVar(&config.TargetScheme, "target-scheme", getEnvString("TARGET_SCHEME", "https"), "Target scheme (http/https)")
	flag.StringVar(&config.SNI, "sni", getEnvString("SNI", ""), "SNI hostname (defaults to target-host if empty)")
	flag.StringVar(&config.LogLevel, "log-level", getEnvString("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	flag.BoolVar(&config.SkipTLSVerify, "skip-tls-verify", getEnvBool("SKIP_TLS_VERIFY", false), "Skip TLS certificate verification (use with caution)")
	flag.BoolVar(&config.EnableTLS, "enable-tls", getEnvBool("ENABLE_TLS", false), "Enable TLS/HTTPS for the proxy server")
	flag.StringVar(&config.TLSCertFile, "tls-cert", getEnvString("TLS_CERT_FILE", "/etc/certs/tls.crt"), "Path to TLS certificate file")
	flag.StringVar(&config.TLSKeyFile, "tls-key", getEnvString("TLS_KEY_FILE", "/etc/certs/tls.key"), "Path to TLS private key file")
	flag.StringVar(&config.ProxyListYAML, "proxy-list", getEnvString("PROXY_LIST", ""), "YAML configuration for proxy list")
	flag.StringVar(&config.PageTitle, "page-title", getEnvString("PAGE_TITLE", ""), "Page title for the proxy dashboard")
	flag.StringVar(&config.SubTitle, "sub-title", getEnvString("SUB_TITLE", ""), "Subtitle for the proxy dashboard")
	flag.StringVar(&config.PageGradient, "page-gradient", getEnvString("PAGE_GRADIENT", ""), "Page gradient for the proxy dashboard")
	flag.StringVar(&config.PageTitleIcon, "page-title-icon", getEnvString("PAGE_TITLE_ICON", ""), "Page title icon for the proxy dashboard")

	readTimeoutSec := flag.Int("read-timeout", getEnvInt("READ_TIMEOUT", 30), "Read timeout in seconds")
	writeTimeoutSec := flag.Int("write-timeout", getEnvInt("WRITE_TIMEOUT", 30), "Write timeout in seconds")
	idleTimeoutSec := flag.Int("idle-timeout", getEnvInt("IDLE_TIMEOUT", 120), "Idle timeout in seconds")

	flag.Parse()

	// Set SNI to target host if not specified
	if config.SNI == "" {
		config.SNI = config.TargetHost
	}

	config.ReadTimeout = time.Duration(*readTimeoutSec) * time.Second
	config.WriteTimeout = time.Duration(*writeTimeoutSec) * time.Second
	config.IdleTimeout = time.Duration(*idleTimeoutSec) * time.Second

	// If proxy-list value is a file path, read the file contents
	config.ProxyListYAML = resolveProxyListYAML(config.ProxyListYAML)

	// Initialize random seed for icon generation
	rand.Seed(time.Now().UnixNano())

	return config
}

// resolveProxyListYAML checks if the value is a file path and reads the file contents.
// If the value starts with "/" or "." and the file exists, it reads the file.
// Otherwise, it returns the value as-is (inline YAML).
func resolveProxyListYAML(value string) string {
	if value == "" {
		return value
	}
	// Check if value looks like a file path
	if value[0] == '/' || value[0] == '.' {
		data, err := os.ReadFile(value)
		if err == nil {
			return string(data)
		}
		// If file doesn't exist, treat as inline YAML
	}
	return value
}

// Helper functions for reading environment variables with type conversion and defaults

// getEnvString returns the environment variable value or the default if not set or empty.
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt returns the environment variable as an integer or the default if not set or invalid.
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvBool returns the environment variable as a boolean or the default if not set or invalid.
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
