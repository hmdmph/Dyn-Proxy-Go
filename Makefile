# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=dyn-proxy-go
DOCKER_IMAGE=dyn-proxy-go
DOCKER_TAG=latest

# Default target
.DEFAULT_GOAL := help

## help: Show this help message
.PHONY: help
help:
	@echo "Available commands:"
	@echo ""
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
	@echo ""

## build: Build the binary
.PHONY: build
build:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...

## clean: Clean build files
.PHONY: clean
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

## test: Run tests
.PHONY: test
test:
	$(GOTEST) -v ./...

## test-coverage: Run tests with coverage
.PHONY: test-coverage
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

## run: Run the application with default settings
.PHONY: run
run: build
	./$(BINARY_NAME)

## run-dev: Run the application with development settings(as a proxy to httpbin.org)
.PHONY: run-dev
run-dev: build
	./$(BINARY_NAME) -log-level=error -skip-tls-verify=true -target-host=httpbin.org -target-port=443 -sni=httpbin.org -proxy-list=/tmp/plist.yaml

## deps: Download and tidy dependencies
.PHONY: deps
deps:
	$(GOMOD) download
	$(GOMOD) tidy

## fmt: Format Go code
.PHONY: fmt
fmt:
	$(GOCMD) fmt ./...

## vet: Run go vet
.PHONY: vet
vet:
	$(GOCMD) vet ./...

## lint: Run golangci-lint (requires golangci-lint to be installed)
.PHONY: lint
lint:
	golangci-lint run

## docker-build: Build Docker image
.PHONY: docker-build
docker-build:
	podman build --tls-verify=false -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

## docker-run: Run Docker container
.PHONY: docker-run
docker-run:
	podman --tls-verify=false run -p 8080:8080 --rm $(DOCKER_IMAGE):$(DOCKER_TAG) --tls-verify=false

## docker-run-dev: Run Docker container with development settings
.PHONY: docker-run-dev
docker-run-dev:
	podman run -p 8080:8080 --rm \
		-e TARGET_HOST=httpbin.org \
		-e TARGET_PORT=443 \
		-e SNI=httpbin.org \
		-e LOG_LEVEL=debug \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

## docker-push: Push Docker image to registry
.PHONY: docker-push
docker-push:
	podman push $(DOCKER_IMAGE):$(DOCKER_TAG)

## kind-load: Load image into Kind cluster
.PHONY: kind-load
kind-load:
	podman save localhost/$(DOCKER_IMAGE):$(DOCKER_TAG) | kind load image-archive /dev/stdin --name dyn-proxy-go-cluster

## k8s-deploy: Deploy to Kubernetes (requires kubectl and k8s manifests)
.PHONY: k8s-deploy
k8s-deploy:
	kubectl apply -f k8s/

## k8s-delete: Delete from Kubernetes
.PHONY: k8s-delete
k8s-delete:
	kubectl delete -f k8s/

## kind-deploy: Build, load into Kind, and deploy to Kubernetes
.PHONY: kind-deploy
kind-deploy: docker-build kind-load k8s-deploy

## kind-setup: Complete Kind setup - create cluster, build, load, and deploy
.PHONY: kind-setup
kind-setup: kind-create docker-build kind-load k8s-deploy

## install-tools: Install development tools
.PHONY: install-tools
install-tools:
	$(GOGET) -u github.com/golangci/golangci-lint/cmd/golangci-lint@latest

## all: Run fmt, vet, test, and build
.PHONY: all
all: fmt vet test build

## release: Build for multiple platforms
.PHONY: release
release:
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-linux-amd64 ./...
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-darwin-amd64 ./...
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-windows-amd64.exe ./...
