.PHONY: help build run test clean docker-build docker-push deploy undeploy logs

# Variables
APP_NAME := p2000-forwarder
DOCKER_IMAGE := ghcr.io/kaije/p2000-nfty
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
NAMESPACE := default

help: ## Display this help message
	@echo "P2000-NFTY Makefile Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the Go binary
	@echo "Building $(APP_NAME)..."
	go build -ldflags="-s -w" -o bin/$(APP_NAME) ./cmd/p2000-forwarder
	@echo "Build complete: bin/$(APP_NAME)"

run: ## Run the application locally
	@echo "Running $(APP_NAME)..."
	go run ./cmd/p2000-forwarder

test: ## Run tests
	@echo "Running tests..."
	go test -v -race -cover ./...

fmt: ## Format code
	@echo "Formatting code..."
	go fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

lint: fmt vet ## Run all linters
	@echo "Linting complete"

tidy: ## Tidy go modules
	@echo "Tidying go modules..."
	go mod tidy

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download

clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -rf bin/
	go clean

docker-build: ## Build Docker image
	@echo "Building Docker image $(DOCKER_IMAGE):$(VERSION)..."
	docker build -t $(DOCKER_IMAGE):$(VERSION) .
	docker tag $(DOCKER_IMAGE):$(VERSION) $(DOCKER_IMAGE):latest
	@echo "Docker image built: $(DOCKER_IMAGE):$(VERSION)"

docker-push: ## Push Docker image to registry
	@echo "Pushing Docker image $(DOCKER_IMAGE):$(VERSION)..."
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest
	@echo "Docker image pushed"

docker-run: ## Run Docker container locally
	@echo "Running Docker container..."
	docker run --rm -it \
		-p 8080:8080 \
		-e CONFIG_PATH=/config/config.yaml \
		-v $(PWD)/config.yaml:/config/config.yaml:ro \
		$(DOCKER_IMAGE):latest

deploy: ## Deploy to Kubernetes
	@echo "Deploying to Kubernetes namespace: $(NAMESPACE)..."
	kubectl apply -f kubernetes/configmap.yaml -n $(NAMESPACE)
	kubectl apply -f kubernetes/deployment.yaml -n $(NAMESPACE)
	kubectl apply -f kubernetes/service.yaml -n $(NAMESPACE)
	@echo "Deployment complete"
	@echo "Checking rollout status..."
	kubectl rollout status deployment/p2000-forwarder -n $(NAMESPACE)

undeploy: ## Remove from Kubernetes
	@echo "Removing from Kubernetes namespace: $(NAMESPACE)..."
	kubectl delete -f kubernetes/service.yaml -n $(NAMESPACE) || true
	kubectl delete -f kubernetes/deployment.yaml -n $(NAMESPACE) || true
	kubectl delete -f kubernetes/configmap.yaml -n $(NAMESPACE) || true
	@echo "Removal complete"

logs: ## Show logs from Kubernetes pod
	@echo "Fetching logs from $(NAMESPACE)..."
	kubectl logs -f -l app=p2000-forwarder -n $(NAMESPACE) --tail=100

status: ## Show Kubernetes deployment status
	@echo "Checking deployment status in $(NAMESPACE)..."
	kubectl get pods -l app=p2000-forwarder -n $(NAMESPACE)
	kubectl get svc p2000-forwarder -n $(NAMESPACE)

restart: ## Restart Kubernetes deployment
	@echo "Restarting deployment in $(NAMESPACE)..."
	kubectl rollout restart deployment/p2000-forwarder -n $(NAMESPACE)
	kubectl rollout status deployment/p2000-forwarder -n $(NAMESPACE)

all: lint test build ## Run lint, test and build

release: all docker-build docker-push ## Build, test and push Docker image
