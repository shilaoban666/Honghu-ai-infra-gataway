# Go Project Structure

## One Sentence

This project is an API gateway. The HTTP request enters at `cmd/gateway/main.go`, is handled by `internal/gateway/http`, routed by `internal/router`, and exits through `internal/provider` to a model service.

## Request Flow

```text
Client / Browser / OpenAI SDK
        |
        v
cmd/gateway/main.go
        |
        v
internal/gateway/http
  - GET /healthz
  - GET /readyz
  - GET /metrics
  - GET /v1/models
  - POST /v1/chat/completions
        |
        v
internal/router
  - decide local_vllm / deepseek / openai_compatible / fake_local
        |
        v
internal/provider
  - FakeProvider for local tests
  - OpenAICompatibleProvider for vLLM / DeepSeek / OpenAI-compatible APIs
        |
        v
Downstream model API
```

## Entry And Exit

- Program entry: `cmd/gateway/main.go`
- HTTP API entry: `internal/gateway/http/server.go`
- Route decision: `internal/router/router.go`
- Model-service exit: `internal/provider/types.go`
- Local fake exit: `internal/provider/fake.go`
- Real OpenAI-compatible exit: `internal/provider/openai.go`
- Config source: `internal/config/config.go`
- Metrics export: `internal/observability/metrics.go`

## Important Commands

```powershell
go test ./...
go run ./cmd/gateway
go run ./cmd/router-bench
```

For local development, run the gateway with a fake provider:

```powershell
$env:GATEWAY_ADDR=":18080"
$env:ENABLE_FAKE_PROVIDER="true"
$env:DEFAULT_MODEL="honghu-local-demo"
go run ./cmd/gateway
```
