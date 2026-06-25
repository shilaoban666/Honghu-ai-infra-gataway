# 实施 Backlog

## Epic 0：工程骨架

目标：让项目从第一天开始就是可构建、可测试、可部署的工程。

任务：

- 初始化 Go module：`github.com/honghu-ai/llm-infra-gateway`。
- 创建 `cmd/gateway`、`cmd/ebpf-agent`、`api/v1alpha1`、`internal` 目录。
- 增加 `Makefile`：
  - `make test`
  - `make lint`
  - `make run-gateway`
  - `make docker-build`
  - `make manifests`
  - `make deploy-local`
- 增加 `.golangci.yml`。
- 增加 GitHub Actions：
  - go test
  - golangci-lint
  - docker build
- 增加 Dockerfile。
- 增加 `docker-compose.yml`，本地启动 PostgreSQL、Redis、Prometheus、Grafana、fake-vLLM。

验收：

- 新机器 clone 后执行 `make test` 通过。
- 执行 `docker compose up` 后 Gateway 能启动。

## Epic 1：Gateway API

目标：实现可被 OpenAI SDK 调用的推理网关。

任务：

- 定义 OpenAI-compatible request/response DTO。
- 实现 `POST /v1/chat/completions`。
- 实现 `GET /v1/models`。
- 实现 `GET /healthz`、`GET /readyz`。
- 实现 non-streaming 调用。
- 实现 SSE streaming proxy。
- 支持 client disconnect cancellation。
- 增加 request id / trace id。
- 增加结构化日志。

验收：

- curl 可以调用 non-streaming。
- curl `stream=true` 可以看到 SSE chunk。
- 客户端断开后下游请求取消。

## Epic 2：Provider 抽象

目标：统一 vLLM、DeepSeek、OpenAI-compatible provider。

任务：

- 定义 `Provider` 接口。
- 实现 fake provider，用于单测。
- 实现 vLLM provider。
- 实现 OpenAI-compatible provider。
- 实现 DeepSeek provider 配置。
- 实现 provider health check。
- 实现 provider timeout 和 retry。
- 实现首 chunk 前 fallback。

验收：

- vLLM provider 可调用本地 vLLM。
- DeepSeek/OpenAI-compatible provider 可通过环境变量配置。
- 本地 provider 异常时 fallback 到云端。

## Epic 3：路由策略

目标：让网关不只是转发，而是 LLM-aware Router。

任务：

- 定义 `RouteRequest`、`RouteDecision`。
- 实现 token 估算。
- 实现任务类型分类：
  - summary
  - classify
  - rewrite
  - extract
  - code
  - reasoning
  - general
- 实现规则路由。
- 实现 provider health 熔断。
- 实现 queue depth / P99 / GPU utilization 输入接口。
- 实现按 tenant 的策略配置。
- 记录 `route_decision_log`。

验收：

- 简单短任务走 local。
- 复杂任务走 cloud。
- local unhealthy 时走 cloud。
- 每次请求能查到 route decision 和 reason。

## Epic 4：计费与成本追踪

目标：支撑成本降低的量化证明。

任务：

- 设计 PostgreSQL schema：
  - tenants
  - api_keys
  - model_catalog
  - provider_registry
  - model_pricing
  - usage_events
  - route_decision_log
  - budget_policies
- 实现 migration。
- 实现 request cost reservation。
- 实现 usage commit。
- 实现 tenant budget check。
- 实现 daily/monthly cost aggregation。
- 暴露成本查询 API。

验收：

- 每个请求产生 usage event。
- 每个请求有 local/cloud cost。
- 能计算 cloud-only vs hybrid 成本差。

## Epic 5：Prometheus 指标

目标：让网关具备生产可观测性。

任务：

- Gateway 指标：
  - `llm_gateway_requests_total`
  - `llm_gateway_request_duration_seconds`
  - `llm_gateway_first_token_duration_seconds`
  - `llm_gateway_tokens_total`
  - `llm_gateway_cost_total`
  - `llm_gateway_route_decisions_total`
  - `llm_gateway_provider_errors_total`
- Provider 指标：
  - provider health
  - provider latency
  - fallback count
- 增加 `/metrics`。
- 增加 Grafana dashboard JSON。

验收：

- Prometheus 能抓到 Gateway 指标。
- Grafana 能展示 RPS、P99、TTFT、路由比例、成本。

## Epic 6：vLLM Kubernetes 部署

目标：在 Kubernetes 上跑本地 GPU 推理。

任务：

- 编写 vLLM Deployment。
- 配置 GPU resource limit。
- 配置 readiness/liveness probe。
- 配置 Service。
- 配置 ServiceMonitor。
- 配置 PrometheusRule。
- 准备一个低成本 7B/8B 模型样例。
- 准备模型启动参数说明。

验收：

- vLLM Pod 能在 GPU 节点启动。
- Gateway 能访问 vLLM Service。
- vLLM `/metrics` 被 Prometheus 抓取。

## Epic 7：NVIDIA GPU 监控

目标：把 GPU 资源纳入平台治理。

任务：

- 部署 NVIDIA Device Plugin 或 GPU Operator。
- 部署 DCGM Exporter。
- 配置 ServiceMonitor。
- 配置 GPU Dashboard。
- 配置告警：
  - GPU util low
  - GPU memory high
  - GPU temperature high
  - GPU exporter down

验收：

- Grafana 展示 GPU utilization、memory、temperature。
- 低利用率告警可触发。

## Epic 8：LLMEngine Operator

目标：用 CRD 管理 vLLM 服务生命周期。

任务：

- 使用 Kubebuilder 初始化项目结构。
- 定义 `LLMEngine` Spec/Status。
- 生成 CRD manifest。
- 实现 Deployment reconcile。
- 实现 Service reconcile。
- 实现 ServiceMonitor reconcile。
- 实现 PDB reconcile。
- 实现 HPA/autoscaling 配置 reconcile。
- 实现 Status Conditions。
- 实现 finalizer。
- 实现 Kubernetes Events。
- 增加 envtest。

验收：

- apply LLMEngine 后自动创建 vLLM 资源。
- 修改 spec 后滚动更新。
- 删除 LLMEngine 后清理子资源。
- Status 能表达 Ready/Degraded。

## Epic 9：eBPF Agent

目标：补齐内核级运行时观测。

任务：

- 设置 clang/bpf2go 构建链路。
- 编写 read/write syscall latency eBPF program。
- 编写 TCP connect/retransmit 采集。
- 实现 ringbuf 事件读取。
- 实现 pid -> pod/container 元数据关联。
- 实现 Prometheus exporter。
- 编写 DaemonSet manifest。
- 编写权限说明和安全边界。

验收：

- 能看到 vLLM 进程 syscall latency histogram。
- 网络异常时 TCP 指标变化。
- 不输出 prompt、completion、API key。

## Epic 10：AWS EKS Terraform

目标：让平台能真实上云。

任务：

- 编写 VPC module。
- 编写 EKS cluster。
- 编写 general node group。
- 编写 GPU node group。
- GPU node group 默认 desired size 为 0。
- 配置 IRSA。
- 配置 EBS CSI。
- 配置 Load Balancer Controller。
- 输出 kubeconfig 指令。
- 输出 Grafana 访问地址。
- 编写 destroy runbook。

验收：

- `terraform apply` 能创建 EKS。
- GPU node group 扩容后节点 Ready。
- `terraform destroy` 能清理资源。

## Epic 11：Benchmark 与报告

目标：拿数据支撑薪资和简历。

任务：

- 准备压测数据集：
  - simple prompts
  - reasoning prompts
  - code prompts
  - long context prompts
- 编写 k6 脚本。
- 编写 cloud-only 测试。
- 编写 hybrid 测试。
- 输出 latency report。
- 输出 throughput report。
- 输出 cost report。
- 输出 failure drill report。
- 保存 Grafana 截图。

验收：

- 能回答“成本降低多少，怎么算的”。
- 能回答“P99 和 TTFT 怎么变化”。
- 能回答“GPU 利用率是否真的提升”。

## Epic 12：P1 鸿鹄平台联通

目标：让已有 Java/Vue 项目变成上游业务调用方。

任务：

- Gateway 提供 OpenAI-compatible endpoint。
- P1 后端增加 provider 配置：
  - baseURL 指向 Gateway
  - apiKey 指向 Gateway tenant key
- P1 请求携带 workspace/user 信息。
- Gateway 记录 P1 tenant usage。
- P1 前端计费看板可展示 Gateway 返回的 usage。

验收：

- P1 聊天请求能通过 Gateway 路由到 vLLM 或云端。
- Gateway usage event 能按 P1 workspace 统计。
- 演示链路完整：AI 应用 -> Gateway -> vLLM/EKS。

