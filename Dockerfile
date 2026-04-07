# Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod and go.sum files (go.sum may not exist for projects without external dependencies)
COPY go.mod ./
COPY go.su[m] ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 go build -a -installsuffix cgo -o dyn-proxy-go .

# Final stage - distroless
FROM gcr.io/distroless/static-debian11:nonroot

# Copy the binary from builder stage
COPY --from=builder /app/dyn-proxy-go /dyn-proxy-go

# Use non-root user
USER nonroot:nonroot

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/dyn-proxy-go", "-help"] || exit 1

# Run the binary
ENTRYPOINT ["/dyn-proxy-go"]
