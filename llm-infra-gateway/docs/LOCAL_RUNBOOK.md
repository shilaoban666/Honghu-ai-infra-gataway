# Local Runbook

This repository now has a runnable Go gateway MVP.

## Requirements

- Go 1.23 or newer
- Optional: Docker Desktop for container builds and compose

## Run Locally

```powershell
$env:GATEWAY_ADDR=":18080"
$env:ENABLE_FAKE_PROVIDER="true"
$env:DEFAULT_MODEL="honghu-local-demo"
go run ./cmd/gateway
```

Health check:

```powershell
curl.exe http://127.0.0.1:18080/healthz
curl.exe http://127.0.0.1:18080/readyz
```

Chat completion:

```powershell
curl.exe -s http://127.0.0.1:18080/v1/chat/completions `
  -H "Content-Type: application/json" `
  -d "{\"model\":\"honghu-local-demo\",\"messages\":[{\"role\":\"user\",\"content\":\"summarize hello\"}]}"
```

Streaming chat completion:

```powershell
curl.exe -N http://127.0.0.1:18080/v1/chat/completions `
  -H "Content-Type: application/json" `
  -d "{\"model\":\"honghu-local-demo\",\"stream\":true,\"messages\":[{\"role\":\"user\",\"content\":\"summarize streaming hello\"}]}"
```

## Implemented Go Packages

- `cmd/gateway`: runnable OpenAI-compatible gateway
- `cmd/fake-vllm`: fake vLLM-compatible local provider
- `cmd/operator`: operator health stub for the later Kubebuilder phase
- `cmd/ebpf-agent`: eBPF agent health and metrics stub
- `cmd/router-bench`: small router decision benchmark
- `api/v1alpha1`: initial `LLMEngine` API shape
- `internal/provider`: fake and OpenAI-compatible providers
- `internal/router`: rule router
- `internal/gateway/http`: HTTP API, SSE streaming, auth, request IDs
- `internal/observability`: Prometheus metrics

The remaining `.gitkeep` files are only for non-Go artifact directories such as Terraform, Helm, Kustomize, Grafana dashboards, Prometheus rules, benchmark data, and generated CRDs.
