# 验收标准

## 1. 本地开发验收

- `make test` 全部通过。
- `make lint` 通过。
- `make docker-build` 通过。
- fake vLLM server 能跑通 Gateway。
- Gateway 暴露 `/metrics`。
- 单元测试覆盖核心路由、provider、billing、streaming cancellation。

## 2. Gateway 验收

- 支持 OpenAI-compatible `/v1/chat/completions`。
- 支持 streaming 和 non-streaming。
- 支持本地 vLLM provider。
- 支持至少一个云端 OpenAI-compatible provider。
- 支持 provider health check。
- 支持首 token 前 fallback。
- 支持 request_id / trace_id。
- 每个请求都写 usage event。
- 每个请求都写 route decision。

## 3. vLLM 验收

- vLLM Pod 能在 GPU 节点启动。
- Pod 请求 `nvidia.com/gpu: 1`。
- `/v1/chat/completions` 可调用。
- `/metrics` 可被 Prometheus 抓取。
- Grafana 能展示 vLLM 请求和 token 指标。

## 4. GPU 监控验收

- NVIDIA Device Plugin 或 GPU Operator 可用。
- DCGM Exporter 正常运行。
- Prometheus 能抓到 GPU 指标。
- Grafana 能看到 GPU 利用率、显存、温度。
- GPU 低利用率告警能触发。

## 5. Operator 验收

- 安装 CRD 成功。
- `kubectl apply -f config/samples/llmengine.yaml` 后自动创建 vLLM Deployment 和 Service。
- 修改 `spec.serving.replicas` 后实际副本变化。
- 修改模型参数后滚动更新。
- 删除 LLMEngine 后子资源清理。
- Status Conditions 正确表达 Ready / Degraded / MetricsAvailable。
- envtest 覆盖核心 reconcile。

## 6. eBPF Agent 验收

- DaemonSet 成功启动。
- 能识别 vLLM 进程或容器。
- 暴露 syscall latency histogram。
- 暴露 network latency / retransmit 指标。
- 不记录 prompt、completion、API Key。
- 在非 Linux 或权限不足时有明确错误。

## 7. EKS 验收

- Terraform 能创建 EKS。
- GPU node group 默认 desired size 为 0。
- 扩容后 GPU 节点 Ready。
- vLLM 能调度到 GPU 节点。
- Gateway 能访问 vLLM Service。
- Prometheus/Grafana/Alertmanager 可访问。
- Terraform destroy 后资源清理。

## 8. 压测验收

必须生成以下报告：

- `benchmark/reports/latency-report.md`
- `benchmark/reports/throughput-report.md`
- `benchmark/reports/cost-report.md`
- `benchmark/reports/failure-drill-report.md`

报告必须包含：

- 测试环境。
- GPU 型号。
- 模型名称。
- 请求样本。
- 并发数。
- local/cloud 路由比例。
- P50/P95/P99。
- first token latency。
- tokens/sec。
- 云端-only 成本。
- 混合路由成本。
- 节省比例公式。

## 9. 简历验收

最终简历中只能写已经证明的数字。

可以写：

- “在 10k 请求压测中，本地 vLLM 承担 X% 简单任务流量，综合推理成本下降 Y%。”
- “通过 vLLM `/metrics` + DCGM Exporter + Gateway 指标构建 Grafana Dashboard，覆盖 QPS、P99、TTFT、tokens/sec、GPU 利用率。”
- “基于 Kubebuilder 实现 LLMEngine Operator，自动管理 vLLM Deployment/Service/ServiceMonitor/HPA 和状态回写。”

不能写：

- 没有报告支撑的固定成本下降比例。
- “eBPF 完整追踪 LLM 请求链路”。
- “生产级高可用”但没有故障演练。

