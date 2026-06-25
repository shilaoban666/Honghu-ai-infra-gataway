# Go + K8s + vLLM + eBPF + AWS EKS + NVIDIA 企业级项目计划

## 1. 项目目标

项目名称：`Honghu LLM Infra Gateway`

一句话定位：

> 面向企业 AI 应用的高性能推理网关与 GPU 推理平台，统一接入本地 vLLM 和云端大模型，通过智能路由、GPU 观测、Kubernetes Operator 和 EKS 基础设施治理，降低推理成本并提升推理服务稳定性。

这个项目要支撑的不是“我会调用 OpenAI API”，而是：

- 我能设计大模型推理平台的数据面和控制面。
- 我能在 Kubernetes 上管理 GPU 推理服务生命周期。
- 我能做成本、延迟、吞吐、GPU 利用率的工程化治理。
- 我能解释 eBPF 在 AI 推理平台中的边界和价值。
- 我能把项目部署到 EKS，并用压测和仪表盘证明收益。

## 2. 目标岗位与薪资支撑

优先目标岗位：

| 方向 | 匹配度 | 项目支撑点 |
|---|---:|---|
| AI Infra / 推理平台工程师 | 高 | vLLM、GPU、K8s Operator、Prometheus、EKS |
| Go 云原生后端工程师 | 高 | 高并发网关、SSE 流式代理、限流、熔断、成本追踪 |
| 大模型应用平台后端工程师 | 高 | 多模型网关、模型路由、计费、租户、管理 API |
| Kubernetes Operator 工程师 | 中高 | CRD、Controller、状态回写、故障自愈 |
| eBPF 可观测性工程师 | 中 | syscall/network I/O 采集，和业务 trace 关联 |

薪资策略：

- 上海：主冲 AI Infra / 推理平台 / 大模型平台岗位，简历目标区间建议按 `30k-60k` 投递。
- 南京 / 苏州：主投大模型应用平台、AI 后端、云原生平台岗位，建议按 `20k-40k` 投递。
- 合肥：主投车企、制造业、AI 应用平台、知识库/RAG 平台岗位，建议按 `18k-35k` 投递。

说明：项目不能保证薪资，但可以把你从“普通 Java/前端项目候选人”提升到“懂 AI 应用 + 云原生推理平台”的候选人池。薪资最终取决于你是否能把架构、代码、数据、故障处理讲清楚。

## 3. 项目边界

### 3.1 必做

- Go 网关：OpenAI-compatible API、流式响应、智能路由、成本追踪、指标暴露。
- vLLM：本地模型服务、Prometheus 指标采集、健康检查、GPU 资源申请。
- Kubernetes：Deployment、Service、Ingress、HPA、ServiceMonitor、PrometheusRule。
- Operator：`LLMEngine` CRD，管理 vLLM 实例生命周期。
- NVIDIA：Device Plugin 或 GPU Operator，DCGM Exporter，GPU 指标告警。
- eBPF Agent：采集 vLLM 进程 syscall / I/O 延迟，暴露 Prometheus 指标。
- AWS EKS：Terraform 创建 VPC、EKS、GPU 节点组、IAM、基础插件。
- Bench：压测脚本、成本对比、Grafana 截图、故障演练报告。

### 3.2 不做或延后

- 不训练模型。
- 不做复杂算法微调。
- 不承诺仅靠 eBPF 还原完整业务请求链路。
- 不一开始就支持所有云厂商，先把 AWS EKS 做深。
- 不把 DeepSeek R1 全量大模型跑在低配 GPU 上，演示优先用 7B/8B 量化模型。

## 4. 总体架构

系统分为四层：

1. 接入层：企业 AI 应用、P1 鸿鹄平台、测试客户端。
2. 数据面：Go LLM Gateway，负责统一 API、鉴权、路由、限流、流式代理、成本追踪。
3. 推理层：本地 vLLM、云端 DeepSeek/OpenAI provider。
4. 控制与观测层：LLMEngine Operator、Prometheus/Grafana、DCGM Exporter、eBPF Agent、Terraform EKS。

架构图见：[system-architecture.svg](diagrams/system-architecture.svg)

## 5. 核心模块设计

### 5.1 Go LLM Gateway

职责：

- 暴露 OpenAI-compatible API：
  - `POST /v1/chat/completions`
  - `POST /v1/completions`
  - `GET /v1/models`
  - `GET /healthz`
  - `GET /readyz`
  - `GET /metrics`
- 支持 SSE streaming proxy。
- 支持本地 vLLM 与云端 DeepSeek/OpenAI provider。
- 支持租户、用户、workspace、API Key。
- 支持请求级成本追踪。
- 支持动态路由策略。

推荐技术：

- Go 1.23+
- `net/http` 或 `gin` / `chi`
- `prometheus/client_golang`
- `zap` 或 `zerolog`
- `pgx` + PostgreSQL
- Redis 用于速率限制、路由缓存、熔断状态
- OpenTelemetry 用于 traceId 传播

核心包结构：

```text
cmd/gateway/
internal/gateway/http/
internal/gateway/middleware/
internal/router/
internal/provider/
internal/provider/vllm/
internal/provider/openai/
internal/provider/deepseek/
internal/billing/
internal/tokenizer/
internal/ratelimit/
internal/observability/
internal/config/
```

关键接口：

```go
type Provider interface {
    Name() string
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
    StreamChat(ctx context.Context, req ChatRequest) (<-chan ChatChunk, error)
    Health(ctx context.Context) ProviderHealth
    Metrics(ctx context.Context) ProviderMetrics
}

type Router interface {
    Pick(ctx context.Context, req RouteRequest) (RouteDecision, error)
}

type CostTracker interface {
    Reserve(ctx context.Context, req CostReservation) error
    Commit(ctx context.Context, event UsageEvent) error
}
```

路由策略：

| 信号 | 用途 |
|---|---|
| prompt token 数 | 短文本优先本地模型 |
| 任务类型 | 简单摘要/分类走本地，复杂代码/推理走云端 |
| 用户预算 | 超预算拒绝或降级到本地 |
| vLLM queue depth | 队列过长时切云端 |
| vLLM P99 latency | 延迟劣化时切云端 |
| GPU utilization | 低利用率时提高本地路由比例 |
| provider error rate | 熔断异常 provider |

路由初版规则：

```text
if user_budget_exceeded:
  reject
if local_vllm_unhealthy:
  route cloud
if prompt_tokens <= 512 and task in [summary, classify, rewrite, extract]:
  route local_vllm
if vllm_queue_depth > threshold or local_p99 > threshold:
  route cloud
else:
  route weighted(local=70%, cloud=30%)
```

企业级增强：

- 路由决策写入 `route_decision_log`。
- 每次请求记录 `decision_reason`。
- 支持按租户配置路由策略。
- 支持灰度策略：某租户 10% 请求走新模型。
- 支持 fallback：本地失败后云端重试，但必须避免重复计费。

### 5.2 成本与计费系统

数据模型：

```text
tenants
api_keys
model_catalog
provider_registry
model_pricing
usage_events
route_decision_log
budget_policies
```

核心指标：

- prompt tokens
- completion tokens
- total tokens
- local estimated cost
- cloud actual cost
- saved cost
- route decision
- provider latency
- stream first token latency
- total latency

成本公式：

```text
cloud_cost = prompt_tokens / 1000 * input_price + completion_tokens / 1000 * output_price
local_cost = gpu_hour_price / successful_requests_per_hour + amortized_storage_and_network
saved_cost = cloud_only_cost - actual_hybrid_cost
save_rate = saved_cost / cloud_only_cost
```

注意：

- `降低成本 85%` 不能写死在项目里。
- 正确做法是提供压测数据，生成 `benchmark/cost-report.md`，由数据计算出节省比例。
- 面试时可以说：“在我的压测场景中，简单任务本地路由比例为 X%，综合成本下降 Y%。”

### 5.3 vLLM 推理服务

职责：

- 运行 OpenAI-compatible API server。
- 提供 `/v1/chat/completions`。
- 暴露 `/metrics` 给 Prometheus。
- 支持 GPU 资源申请和健康检查。

模型选择：

- 本地演示：7B/8B instruct 量化模型，保证单卡能跑起来。
- 云端兜底：DeepSeek / OpenAI-compatible provider。
- 生产扩展：按 GPU 规格支持更大模型或 tensor parallel。

Kubernetes 资源：

```yaml
resources:
  limits:
    nvidia.com/gpu: "1"
```

健康检查：

- `/health`
- vLLM process alive
- `/metrics` scrape success
- first token latency 小于阈值
- queue depth 小于阈值

关键指标：

- QPS
- P50/P95/P99 latency
- time to first token
- tokens/sec
- queue depth
- prompt tokens total
- generation tokens total
- GPU memory used
- GPU utilization
- GPU temperature

### 5.4 NVIDIA GPU 监控

部署方式：

- EKS GPU 节点使用 NVIDIA Device Plugin 或 NVIDIA GPU Operator。
- DCGM Exporter 作为 DaemonSet 部署。
- Prometheus 采集 DCGM Exporter `/metrics`。

告警规则：

| 告警 | 条件 | 处理 |
|---|---|---|
| GPU 利用率过低 | `< 30%` 持续 10 分钟 | 触发缩容建议或减少本地副本 |
| GPU 显存接近上限 | `> 90%` 持续 5 分钟 | 降低 batch / 拒绝新请求 |
| GPU 温度过高 | `> 80C` 持续 5 分钟 | 告警并迁移流量 |
| vLLM 队列堆积 | queue depth 持续升高 | 扩容或路由云端 |
| P99 延迟劣化 | 超过 SLO | 熔断本地模型 |

### 5.5 LLMEngine CRD + Operator

使用 Kubebuilder 实现控制面。

CRD 示例：

```yaml
apiVersion: ai.honghu.io/v1alpha1
kind: LLMEngine
metadata:
  name: qwen-7b-local
spec:
  model:
    name: qwen-7b
    image: vllm/vllm-openai:latest
    modelURI: s3://honghu-models/qwen-7b
    tensorParallelSize: 1
    maxModelLen: 4096
  serving:
    replicas: 1
    port: 8000
    gpu: 1
    resources:
      cpu: "4"
      memory: "24Gi"
  autoscaling:
    enabled: true
    minReplicas: 1
    maxReplicas: 3
    targetQueueDepth: 8
    targetGPUUtilization: 70
  observability:
    serviceMonitor: true
    grafanaDashboard: true
  routing:
    enabled: true
    weight: 70
    fallbackProvider: deepseek
```

Operator Reconcile 职责：

- 读取 `LLMEngine`。
- 创建/更新 Deployment。
- 创建 Service。
- 创建 ServiceMonitor。
- 创建 HPA 或自定义 autoscaler 配置。
- 创建 PodDisruptionBudget。
- 创建 ConfigMap / Secret 引用。
- 更新 CRD Status。
- 处理滚动升级。
- 处理失败重试。
- 删除时清理子资源。

Status 示例：

```yaml
status:
  phase: Ready
  endpoint: http://qwen-7b-local.ai-inference.svc:8000
  availableReplicas: 1
  observedGeneration: 3
  conditions:
    - type: Ready
      status: "True"
      reason: VLLMHealthy
    - type: MetricsAvailable
      status: "True"
      reason: PrometheusScrapeOK
```

企业级点：

- 使用 OwnerReference 管理资源归属。
- 使用 finalizer 做清理。
- 使用 Conditions 表达状态。
- 单元测试使用 envtest。
- 事件记录到 Kubernetes Events。
- 不把业务逻辑塞进 controller，保持 reconcile 幂等。

### 5.6 eBPF 可观测 Agent

目标：

- 采集 vLLM 进程 syscall 延迟分布。
- 采集网络 I/O 延迟和错误计数。
- 将 pid/cgroup/container/pod 信息关联成 Prometheus 标签。
- 和 Gateway 的 traceId 指标在 Grafana 上做关联分析。

必须注意：

> eBPF syscall 采集不能单独证明完整业务请求耗时。完整链路耗时由 Gateway 记录；eBPF 负责解释底层 I/O、系统调用、网络异常是否影响推理服务。

第一版采集：

- `sys_enter_read` / `sys_exit_read`
- `sys_enter_write` / `sys_exit_write`
- `tcp_connect`
- `tcp_retransmit_skb` 或 TCP 重传 tracepoint
- process exec / exit，用于识别 vLLM worker

实现：

- `cilium/ebpf`
- `bpf2go`
- ring buffer 将事件送到 Go user space
- Prometheus exporter 暴露：
  - `vllm_syscall_latency_seconds_bucket`
  - `vllm_syscall_errors_total`
  - `vllm_tcp_connect_latency_seconds_bucket`
  - `vllm_tcp_retransmits_total`

部署：

- DaemonSet
- Linux only
- hostPID
- 挂载 `/sys/kernel/debug`、`/sys/fs/bpf`、`/proc`
- 最小化 capability，必要时使用 `CAP_BPF`、`CAP_PERFMON`、`CAP_SYS_ADMIN` 兼容旧内核

安全：

- 默认只监控指定 namespace 和 pod label。
- 不采集请求正文。
- 不采集 prompt、completion、API Key。
- 不输出用户敏感数据。

### 5.7 AWS EKS + Terraform

目录：

```text
infra/terraform/aws-eks/
  main.tf
  versions.tf
  variables.tf
  outputs.tf
  vpc.tf
  eks.tf
  node-groups.tf
  iam.tf
  addons.tf
  observability.tf
```

资源：

- VPC
- Public/Private Subnets
- EKS Cluster
- Managed Node Group:
  - general node group
  - gpu node group
- IAM roles
- IRSA
- EBS CSI Driver
- AWS Load Balancer Controller
- NVIDIA device plugin / GPU Operator
- kube-prometheus-stack
- Grafana ingress

GPU 节点策略：

| 环境 | 节点 |
|---|---|
| 本地/演示 | kind + fake vLLM，或单机 Docker vLLM |
| 低成本 AWS 演示 | `g4dn.xlarge`，跑 7B/8B 量化模型 |
| 更稳定演示 | `g5.xlarge` / `g5.2xlarge` |
| 企业扩展 | g5/g6/p4/p5 多规格节点池 |

成本控制：

- Terraform 默认 `gpu_desired_size = 0`。
- 需要演示时扩到 1。
- 提供 `make eks-scale-gpu-up` 和 `make eks-scale-gpu-down`。
- 所有 AWS 资源打 tag。
- 提供 `terraform destroy` runbook。

### 5.8 Observability

Prometheus 采集：

- gateway `/metrics`
- vLLM `/metrics`
- DCGM Exporter `/metrics`
- eBPF Agent `/metrics`
- Kubernetes node/pod metrics

Grafana Dashboard：

1. Gateway Overview
   - RPS
   - Error rate
   - P50/P95/P99
   - first token latency
   - route local/cloud ratio
2. Cost Dashboard
   - daily token usage
   - local/cloud cost
   - saved cost
   - cost per tenant
3. vLLM Dashboard
   - queue depth
   - tokens/sec
   - request latency
   - failed requests
4. GPU Dashboard
   - GPU utilization
   - memory usage
   - temperature
   - power draw
5. eBPF Runtime Dashboard
   - syscall latency
   - network retries
   - I/O error count

Alertmanager：

- gateway error rate > 5%
- local vLLM unhealthy
- cloud provider timeout
- P99 latency > SLO
- GPU memory > 90%
- GPU utilization < 30% for 10m
- vLLM queue depth > threshold
- eBPF agent down

## 6. Repo 结构

最终代码目录建议：

```text
llm-infra-gateway/
├── cmd/
│   ├── gateway/
│   ├── operator/
│   ├── ebpf-agent/
│   └── router-bench/
├── api/
│   └── v1alpha1/
├── internal/
│   ├── gateway/
│   ├── provider/
│   ├── router/
│   ├── billing/
│   ├── tokenizer/
│   ├── observability/
│   ├── store/
│   └── security/
├── ebpf/
│   ├── bpf/
│   └── collector/
├── config/
│   ├── crd/
│   ├── manager/
│   ├── rbac/
│   └── samples/
├── deploy/
│   ├── helm/
│   ├── kustomize/
│   └── manifests/
├── infra/
│   └── terraform/
│       └── aws-eks/
├── observability/
│   ├── grafana/
│   ├── prometheus-rules/
│   └── service-monitors/
├── benchmark/
│   ├── k6/
│   ├── datasets/
│   └── reports/
├── docs/
│   ├── diagrams/
│   ├── delivery/
│   └── interview/
├── scripts/
├── Makefile
├── go.mod
└── README.md
```

## 7. 里程碑计划

### Phase 0：项目骨架与工程标准，2-3 天

交付：

- Go module 初始化。
- Makefile。
- golangci-lint。
- Dockerfile。
- GitHub Actions。
- 基础 README。
- 本地 fake vLLM server。

验收：

- `make test` 通过。
- `make docker-build` 通过。
- CI 通过。

### Phase 1：Go 推理网关 MVP，5-7 天

交付：

- `/v1/chat/completions`。
- 支持非流式和流式。
- 接入 vLLM provider。
- 接入 DeepSeek/OpenAI-compatible provider。
- 基础路由：local-first + fallback。
- Prometheus 指标。

验收：

- 能用 OpenAI SDK 调 Gateway。
- vLLM 不可用时自动切云端。
- Grafana 能看到 RPS、延迟、错误。

### Phase 2：成本追踪与智能路由，5-7 天

交付：

- token 估算。
- usage_events 表。
- route_decision_log 表。
- 路由策略引擎。
- tenant budget。
- 成本 Dashboard。

验收：

- 每个请求都能查到路由原因和成本。
- 压测后自动生成成本报告。
- 能证明 local/cloud 混合路由比 cloud-only 更省。

### Phase 3：vLLM + GPU 监控，4-6 天

交付：

- vLLM Kubernetes manifest。
- NVIDIA Device Plugin 或 GPU Operator。
- DCGM Exporter。
- ServiceMonitor。
- Grafana GPU Dashboard。
- PrometheusRule 告警。

验收：

- Prometheus 能抓到 vLLM 指标。
- Prometheus 能抓到 GPU 指标。
- GPU 利用率低于阈值能触发告警。

### Phase 4：LLMEngine Operator，8-12 天

交付：

- Kubebuilder 项目。
- `LLMEngine` CRD。
- Reconciler。
- Deployment/Service/ServiceMonitor/PDB/HPA 管理。
- Status Conditions。
- envtest 单测。

验收：

- `kubectl apply -f config/samples/llmengine.yaml` 后自动创建 vLLM 服务。
- 修改 CRD spec 后能滚动更新。
- 删除 CRD 后资源清理。
- vLLM 异常时 Status 能反映。

### Phase 5：eBPF Agent，7-10 天

交付：

- bpf2go 构建链路。
- syscall latency 采集。
- TCP connect/retransmit 指标。
- Pod 元数据关联。
- Prometheus exporter。
- DaemonSet 部署。

验收：

- vLLM Pod 运行时能看到 syscall 延迟直方图。
- 网络异常时 TCP 指标变化。
- 不采集 prompt 或 API Key。

### Phase 6：AWS EKS + Terraform，5-7 天

交付：

- Terraform EKS。
- GPU managed node group。
- IRSA。
- Observability 安装。
- vLLM 部署。
- Gateway 部署。

验收：

- 一键创建 EKS。
- 一键部署平台。
- Gateway 能访问 EKS 内 vLLM。
- Grafana 有完整指标。
- 一键销毁避免持续扣费。

### Phase 7：压测、故障演练、面试材料，5-7 天

交付：

- k6 压测脚本。
- cloud-only vs hybrid 成本报告。
- latency / throughput 报告。
- GPU 利用率报告。
- 故障演练报告：
  - vLLM Pod crash
  - cloud provider timeout
  - GPU node drain
  - Prometheus unavailable
- 简历 bullets。
- 面试 Q&A。

验收：

- `benchmark/reports/final-report.md` 可直接展示。
- Grafana 截图齐全。
- 面试能讲清楚 10 个关键问题。

## 8. 技术难点与面试亮点

### 8.1 流式代理

难点：

- SSE 不能简单读完再返回。
- 客户端取消时要向下游 provider 取消。
- 需要统计 first token latency 和 total latency。
- fallback 时要避免已经输出部分 token 后再切 provider。

面试表达：

> 我把非流式和流式调用抽象成统一 Provider 接口，但流式路径单独处理 backpressure、client cancellation 和 first-token metrics。fallback 只发生在首 chunk 前，避免输出重复内容。

### 8.2 智能路由

难点：

- 不能只靠 prompt 长度。
- 要综合任务类型、预算、provider health、queue depth、P99。
- 路由原因必须可审计。

面试表达：

> 路由不是 if else 堆叠，而是策略引擎。每次决策会输出 model、provider、confidence、reason 和 fallback plan，并写入审计表。

### 8.3 Operator 幂等设计

难点：

- Reconcile 可能重复执行。
- Deployment 手动修改可能 drift。
- 子资源状态要聚合回 CRD status。

面试表达：

> Controller 只根据期望状态收敛实际状态，所有创建更新都做 idempotent patch，并通过 Conditions 表示 vLLM Ready、MetricsAvailable、Degraded 等状态。

### 8.4 GPU 扩缩容

难点：

- GPU Pod 启动慢，不能只靠 CPU HPA。
- queue depth 和 GPU 利用率要一起看。
- 缩容要考虑长请求和流式请求。

面试表达：

> 扩容看 queue depth、P99 和 GPU memory，缩容看低利用率持续窗口，同时结合 Pod graceful shutdown，避免中断正在 streaming 的请求。

### 8.5 eBPF 边界

难点：

- syscall 不是业务请求。
- vLLM 是 Python/FastAPI/worker 架构，进程模型复杂。
- 高基数标签会打爆 Prometheus。

面试表达：

> 我没有把 syscall trace 伪装成业务链路。业务链路由 Gateway trace 记录，eBPF 负责解释底层 syscall 和网络 I/O 是否异常，并且对标签做 namespace/pod 级聚合，避免高基数。

## 9. 风险清单

| 风险 | 影响 | 对策 |
|---|---|---|
| GPU 成本高 | AWS 账单失控 | GPU 节点默认 0，演示前扩容，演示后缩容/销毁 |
| g4dn 跑 7B 性能有限 | 指标不好看 | 用量化模型，明确演示场景；更稳定用 g5 |
| eBPF 在不同内核差异 | Agent 不稳定 | 先支持 EKS AL2/AL2023，记录内核版本，必要时降级采集 |
| vLLM 指标名版本变化 | Dashboard 失效 | 指标采集做兼容层，Dashboard 标注 vLLM 版本 |
| Operator 范围过大 | 交付延期 | 先支持单模型单 GPU，再扩展多副本和 TP |
| 成本节省数据被质疑 | 面试风险 | 只展示可复现压测报告和公式 |

## 10. 最终交付物

必须交付：

- 可运行 Go Gateway。
- 可部署 vLLM 服务。
- 可运行 LLMEngine Operator。
- 可运行 eBPF Agent。
- EKS Terraform。
- Helm/Kustomize 部署配置。
- Grafana Dashboard。
- Prometheus Alert Rules。
- Benchmark 报告。
- 成本报告。
- 架构图。
- 面试 Q&A。

最终演示脚本：

1. `terraform apply` 创建 EKS。
2. 安装 GPU device plugin / GPU Operator。
3. 安装 Prometheus/Grafana/DCGM。
4. 部署 LLMEngine Operator。
5. `kubectl apply -f llmengine-qwen7b.yaml`。
6. 部署 Gateway。
7. P1 鸿鹄平台调用 Gateway。
8. 压测 simple tasks，观察本地路由比例。
9. 压测 complex tasks，观察云端路由比例。
10. 停掉 vLLM Pod，观察 fallback 和 Operator 自愈。
11. 查看 Grafana 和成本报告。

