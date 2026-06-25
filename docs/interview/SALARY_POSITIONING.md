# 面试与薪资包装

## 1. 简历项目名

推荐写法：

> Honghu LLM Infra Gateway：基于 Go + Kubernetes + vLLM + eBPF + AWS EKS 的企业级大模型推理网关与 GPU 推理平台

## 2. 简历摘要

```text
设计并实现企业级大模型推理网关，统一接入本地 vLLM 与 DeepSeek/OpenAI-compatible 云端模型，支持 OpenAI-compatible API、SSE 流式代理、智能路由、请求级 Token/成本追踪、Prometheus 指标和 Grafana 成本看板。基于 Kubebuilder 实现 LLMEngine Operator 管理 vLLM Pod 生命周期，并在 AWS EKS GPU 节点组上部署 NVIDIA Device Plugin、DCGM Exporter 与 eBPF Agent，实现 GPU 利用率、vLLM 队列、P99、TTFT、syscall/network I/O 延迟的统一观测和告警。
```

## 3. 简历 bullets

根据实际完成情况选择，不要提前写未完成内容。

### Gateway

- 使用 Go 实现 OpenAI-compatible 推理网关，支持 `/v1/chat/completions` 的非流式与 SSE 流式代理，统一屏蔽 vLLM、DeepSeek、OpenAI-compatible provider 差异。
- 设计智能路由策略，综合 prompt token、任务类型、vLLM queue depth、P99、GPU 利用率、租户预算和 provider 健康状态，在本地模型与云端模型之间动态选择。
- 实现请求级成本追踪，记录 prompt/completion tokens、provider、路由原因、延迟、错误和费用，支撑租户级成本分析与预算控制。

### Kubernetes / Operator

- 基于 Kubebuilder 实现 `LLMEngine` CRD 与 Controller，自动创建和维护 vLLM Deployment、Service、ServiceMonitor、PDB、HPA，并通过 Status Conditions 回写 Ready、Degraded、MetricsAvailable 状态。
- 设计 vLLM 滚动升级和故障自愈流程，支持 CRD spec 变更后的幂等 reconcile 和子资源 owner reference 管理。

### Observability

- 构建 Prometheus + Grafana + Alertmanager 可观测体系，统一采集 Gateway、vLLM `/metrics`、NVIDIA DCGM Exporter 和 eBPF Agent 指标。
- 设计 AI 推理专用 Dashboard，覆盖 QPS、P99、TTFT、tokens/sec、queue depth、GPU 显存/利用率/温度、local/cloud 路由比例和成本趋势。

### eBPF

- 使用 cilium/ebpf 实现 eBPF DaemonSet，采集 vLLM 进程 syscall 与 TCP I/O 延迟分布，并通过 pod/container 标签聚合暴露为 Prometheus 指标。
- 明确区分业务链路追踪与内核级观测：Gateway 记录 request trace，eBPF 用于定位 syscall/network 层性能异常，避免采集用户 prompt 和敏感数据。

### AWS EKS

- 使用 Terraform 管理 AWS EKS、GPU managed node group、IAM/IRSA、基础插件和观测组件，支持 GPU 节点按需扩缩并提供销毁 runbook 控制成本。

## 4. 面试故事线

### 4.1 开场 60 秒

```text
我做的是一个企业级大模型推理网关和 GPU 推理平台。业务侧只调用 OpenAI-compatible API，网关根据任务复杂度、预算、vLLM 健康状态、GPU 利用率和队列深度，动态选择本地 vLLM 或云端大模型。控制面用 Kubebuilder 做 LLMEngine Operator 管理 vLLM Pod 生命周期。观测侧打通 Gateway、vLLM /metrics、NVIDIA DCGM Exporter 和 eBPF Agent，最终用 Grafana 展示延迟、吞吐、GPU 利用率和成本趋势。这个项目的重点是把大模型应用从 API 调用提升到推理基础设施治理。
```

### 4.2 面试官最可能问的问题

#### Q1：为什么不全部走云端模型？

回答：

```text
全部走云端实现简单，但成本不可控，而且简单任务没有必要用最强模型。我把任务按 token 长度、任务类型和历史表现做分层：摘要、分类、改写、抽取等简单任务优先走本地 vLLM；复杂推理、代码和本地模型超载时走云端。所有路由都会写入 route decision log，成本收益通过 cloud-only 和 hybrid 两组压测对比计算。
```

#### Q2：为什么不用 Nginx/Envoy 直接代理？

回答：

```text
普通代理解决不了 LLM 场景的路由决策、token 成本、streaming first token latency、provider fallback 和租户预算问题。Go Gateway 不只是转发，而是 LLM-aware gateway：它理解模型、token、streaming、provider health 和成本。
```

#### Q3：vLLM 的哪些指标最重要？

回答：

```text
传统 API 看 QPS、P99、错误率；LLM 推理还要看 TTFT、tokens/sec、queue depth、prompt/generation token 数、KV cache 或显存压力。GPU 层还要看 utilization、memory used、temperature、power。路由和扩缩容主要参考 queue depth、TTFT/P99、GPU utilization 和 error rate。
```

#### Q4：eBPF 在这个项目里解决什么问题？

回答：

```text
eBPF 解决的是无侵入底层观测。vLLM 是 Python 服务，业务指标能告诉我请求慢了，但不一定能告诉我是 syscall、网络连接、磁盘 I/O 还是进程层异常。我用 eBPF 采集 syscall 和 TCP 维度的延迟分布，再和 Gateway trace 时间窗口关联。它不是替代业务 trace，而是补充 kernel/runtime 视角。
```

#### Q5：Operator 为什么必要？

回答：

```text
如果只是一个模型，可以手写 Deployment。但企业里会有多个模型、多套资源规格、多租户、多环境和滚动升级需求。Operator 把 vLLM 服务抽象成 LLMEngine，通过 CRD 声明模型、GPU、replica、autoscaling 和 observability，Controller 负责把期望状态收敛到实际资源，并回写健康状态。
```

#### Q6：GPU 低利用率自动缩容怎么做？

回答：

```text
不能只看瞬时 utilization，否则会误缩容。我的策略是 GPU utilization 低于阈值持续 10 分钟，同时 queue depth 接近 0，且没有长 streaming 请求，再降低副本或给出缩容建议。扩容则更偏向 queue depth、TTFT 和 P99，因为 GPU Pod 启动慢，不能等用户已经明显感知卡顿再扩。
```

#### Q7：如何避免成本数据被质疑？

回答：

```text
我不写固定的节省比例，而是提供 benchmark：同一批请求分别跑 cloud-only 和 hybrid，记录 token、provider、latency 和价格快照，用公式算出实际成本差。最终简历只写这组可复现压测得到的数字。
```

## 5. 薪资谈判角度

你要把自己从“会做应用”升级成“能做平台”：

- 普通 AI 应用：会调模型、会 RAG。
- 高阶 AI 应用平台：会网关、计费、权限、多 provider、观测。
- AI Infra：会 vLLM、GPU、K8s、Operator、EKS、eBPF、压测和成本治理。

谈薪时强调：

```text
我不是只做业务 CRUD 或简单大模型接口调用。我做过从应用接入、模型路由、成本追踪、GPU 监控、Kubernetes Operator 到 EKS 基础设施的完整链路。这个能力可以直接降低企业接入大模型后的推理成本和运维风险。
```

## 6. 不能踩的坑

- 不要声称“精通 eBPF 内核开发”，除非你能讲 verifier、map、ringbuf、kprobe/tracepoint 差异。
- 不要声称“生产节省 85% 成本”，除非有生产账单或可复现压测。
- 不要说“g4dn.xlarge 可以稳定跑 DeepSeek R1 大模型”，这是明显不靠谱。
- 不要把 Operator 讲成“自动生成 YAML”，要讲 reconcile 和状态收敛。
- 不要只讲技术名词，要讲为什么这些技术解决了成本、稳定性和可观测问题。

