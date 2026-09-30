.PHONY: all build run test docker-build k8s-apply clean

BINARY_SERVER=bin/server
BINARY_OPERATOR=bin/operator

all: test build

build:
	@echo "Building binaries..."
	@mkdir -p bin
	go build -o $(BINARY_SERVER) ./cmd/server
	go build -o $(BINARY_OPERATOR) ./cmd/operator

run-server: build
	@echo "Running API server & Robomotion Gateway..."
	./$(BINARY_SERVER)

run-operator: build
	@echo "Running K8s CRD Operator..."
	./$(BINARY_OPERATOR)

test:
	@echo "Running tests..."
	go test -v ./...

docker-build:
	@echo "Building Docker image..."
	docker build -t k8s-chromedp-synthetics:latest .

k8s-apply:
	@echo "Applying Kubernetes CRD & Deployment..."
	kubectl apply -f deployments/crds/synthetictest.yaml
	kubectl apply -f deployments/rbac.yaml
	kubectl apply -f deployments/deployment.yaml
	kubectl apply -f deployments/samples/sample-test.yaml

clean:
	@echo "Cleaning binaries and temporary data..."
	rm -rf bin/ data/
