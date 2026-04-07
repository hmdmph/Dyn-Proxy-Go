package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// main is the entry point for the dyn-proxy-go service.
// It handles configuration parsing, server initialization, signal handling, and graceful shutdown.
func main() {
	// Parse configuration from flags and environment variables
	config := parseConfig()

	// Initialize the proxy server with the parsed configuration
	proxyServer := NewProxyServer(config)

	// Set up signal handling for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// Start the server in a separate goroutine to avoid blocking
	go func() {
		if err := proxyServer.Start(); err != nil && err != http.ErrServerClosed {
			proxyServer.logger.Error("Server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	// Block until we receive a shutdown signal
	<-signalChan
	proxyServer.logger.Info("Received shutdown signal, initiating graceful shutdown")

	// Create a context with timeout for graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Attempt graceful shutdown with timeout
	if err := proxyServer.Stop(shutdownCtx); err != nil {
		proxyServer.logger.Error("Graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	proxyServer.logger.Info("Dyn-Proxy-Go service shutdown completed successfully")
}
