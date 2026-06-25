GO ?= go
IMAGE ?= honghu/llm-infra-gateway:dev

.PHONY: test lint run-gateway run-fake-vllm docker-build manifests deploy-local tidy

test:
	$(GO) test ./...

lint:
	$(GO) vet ./...

run-gateway:
	$(GO) run ./cmd/gateway

run-fake-vllm:
	$(GO) run ./cmd/fake-vllm

docker-build:
	docker build -t $(IMAGE) .

manifests:
	@echo "Kubernetes CRD/manifests will be generated in the operator phase."

deploy-local:
	docker compose up --build

tidy:
	$(GO) mod tidy
