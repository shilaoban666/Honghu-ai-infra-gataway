# 系统架构说明

## 1. 架构分层

```text
Client / P1 Honghu Platform
        |
        v
Go LLM Gateway
  - Auth / Tenant / API Key
  - OpenAI-compatible API
  - Streaming Proxy
  - Router
  - Cost Tracker
  - Metrics
        |
        +--------------------+
        |                    |
        v                    v
Local vLLM              Cloud Providers
  - 7B/8B model           - DeepSeek
  - GPU Pod               - OpenAI-compatible
  - /metrics              - fallback
        |
        v
Kubernetes / AWS EKS GPU Node
  - LLMEngine Operator
  - NVIDIA Device Plugin / GPU Operator
  - DCGM Exporter
  - eBPF Agent
  - Prometheus / Grafana
```

## 2. 数据面

数据面是 Gateway + Provider。

### 2.1 请求生命周期

1. Client 调用 `/v1/chat/completions`。
2. Gateway 生成 `request_id` 和 `trace_id`。
3. 鉴权、租户识别、配额预检查。
4. 估算 prompt tokens。
5. Router 读取策略、Provider 健康、vLLM 指标、预算信息。
6. 选择本地 vLLM 或云端 provider。
7. 执行请求。
8. 统计 first token latency、total latency、completion tokens。
9. 写入 usage event 和 route decision。
10. 暴露 Prometheus 指标。

### 2.2 流式请求

流式请求必须做到：

- 上游 client 断开时取消下游请求。
- 首 token 前允许 fallback，首 token 后不 fallback。
- 每个 chunk 只做轻量处理，避免阻塞。
- 记录 first token latency。
- 最终 commit usage event。

## 3. 控制面

控制面是 `LLMEngine Operator`。

Operator 根据 `LLMEngine` CRD 管理：

- vLLM Deployment
- Service
- ServiceMonitor
- HPA / autoscaling config
- PodDisruptionBudget
- ConfigMap / Secret
- Status Conditions

### 3.1 Reconcile 原则

- 幂等。
- 以 CRD spec 为唯一期望状态。
- 所有子资源带 OwnerReference。
- 不直接删除用户未声明但不归属自己的资源。
- status 和 event 要清晰。

## 4. 可观测体系

### 4.1 指标来源

| 来源 | 指标 |
|---|---|
| Gateway | RPS、延迟、错误、路由比例、成本 |
| vLLM | queue、tokens/sec、请求状态、推理延迟 |
| DCGM Exporter | GPU 利用率、显存、温度、功耗 |
| eBPF Agent | syscall latency、network latency、TCP retransmit |
| Kubernetes | Pod 状态、重启、资源申请、节点状态 |

### 4.2 Trace 关联

业务 trace：

- Gateway 生成 `trace_id`。
- 传给 vLLM request header。
- Gateway 日志、metrics exemplar、usage event 记录 trace。

内核观测：

- eBPF 以 pod/container/pid 聚合。
- 不记录用户文本。
- 通过时间窗口和 pod 标签与业务指标关联。

## 5. 扩缩容策略

扩容信号：

- vLLM queue depth 上升。
- Gateway P99 上升。
- first token latency 上升。
- GPU memory 高但 utilization 不低。
- local provider 熔断频繁。

缩容信号：

- GPU utilization `< 30%` 持续 10 分钟。
- queue depth 接近 0。
- local route ratio 低。
- 没有长 streaming 请求。

## 6. 安全架构

- API Key 鉴权。
- 租户隔离。
- Provider key 存 Secret 或 AWS Secrets Manager。
- Gateway outbound provider endpoint allowlist。
- 不在日志记录 prompt / API key。
- eBPF Agent 不采集正文。
- Kubernetes RBAC 最小权限。
- eBPF DaemonSet 单独 namespace，限制 nodeSelector。
- NetworkPolicy 限制 Prometheus/Grafana/Provider 访问路径。

## 7. 高可用与故障处理

| 故障 | 处理 |
|---|---|
| 本地 vLLM 不可用 | Gateway 切云端 |
| 云端 provider timeout | fallback 到备用 provider 或返回明确错误 |
| GPU 节点不可用 | K8s 重新调度，Gateway 暂时切云端 |
| Operator 异常 | 已存在服务继续运行，告警 |
| Prometheus 异常 | 不影响推理，仅影响自动决策精度 |
| Redis 异常 | 降级到本地内存限流，告警 |

