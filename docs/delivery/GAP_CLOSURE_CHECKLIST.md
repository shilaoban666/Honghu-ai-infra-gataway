# Honghu AI 求职增强落地清单（Gap Closure Checklist）

> 目标城市：上海 / 南京 / 苏州 / 无锡 / 合肥
> 目标岗位：大模型应用平台后端 · AI 应用后端 ·（进阶）AI Infra / 推理平台
> 适用项目：`Honghu-ai-assigmemt`（后端）+ `Honghu-ai-assigmemt-UI`（前端）
>
> 这份清单只列**面试会被打、且投入产出高**的改造。每一项都给了：目标 / 具体任务（点到真实类）/ 验收标准 / 工期 / 面试防守话术 / 可量化产出。
> 按 P0 → P1 → P2 顺序做。**P0 不做完，不要把"企业级"三个字写进简历。**

---

## 怎么用这份清单

- 每做完一项，把 `[ ]` 改成 `[x]`，并在"可量化产出"里填上真实数字（截图、报告路径、压测结果）。
- 简历里**只能写已经打勾且有产出的项**。没产出的不写。
- 优先级判断标准：①能不能消除一个面试红线 ②能不能产出一个可量化数字 ③工期是否可控。

---

## P0 · 消除面试红线（建议 1–2 周，必做）

这三项是目前"一问就破"的地方，也是 ROI 最高的改造。

### [x] P0-1　真正的认证鉴权：Spring Security + JWT　✅ 已落地（2026-06-22）

> 已实现：`spring-boot-starter-security` + jjwt 0.12.5；`JwtService`(HS256)、`JwtAuthenticationFilter`(验签后用 token 里的可信 userId 覆盖 `X-User-Id`)、`SecurityConfig`(无状态/CORS/方法级 RBAC，用户列表仅 ADMIN)。`CurrentUserService` 改为读 SecurityContext，删除内存 opaque token。`dev-header-fallback` 默认 false（生产只信 JWT）。新增 `JwtServiceTest`(5 例)。后端 168 测试全绿。前端 `identity.js` 统一带 `Authorization: Bearer`，登录态持久化 `authToken`。

**为什么必须做**：当前 `security/UserSessionTokenService` + 前端注入 `X-User-Id` 请求头（见 UI `src/api/identity.js`）做身份，`pom.xml` 里**只有 `spring-security-crypto`，没有 `spring-boot-starter-security`**。面试官一句"我把 `X-User-Id` 改成别人的 id 不就越权了？"直接破防。这是你 README roadmap 里已经埋了伏笔的事（`CurrentUserService` 已解耦），现在兑现。

**具体任务**：
- `pom.xml` 增加 `spring-boot-starter-security` + `io.jsonwebtoken:jjwt-api/impl/jackson`。
- 新增 `config/SecurityConfig`：`SecurityFilterChain` 放行 `/api/v1/users/login|register`、`/actuator/health`、`/swagger-ui/**`，其余需认证。
- 新增 `security/JwtAuthenticationFilter`（继承 `OncePerRequestFilter`）：从 `Authorization: Bearer` 解析 JWT → 还原 userId/role/workspaceId → 写入 `SecurityContext`。
- 改造 `security/CurrentUserService`：身份来源从"请求头"切到"`SecurityContext`"（业务层不用动，这就是你之前解耦的价值，面试可讲）。
- 登录接口签发 JWT（含 role、workspaceId、过期时间）；前端 `src/api/auth.js` 存 token、`http.js` 统一带 `Authorization`，删掉/弱化 `X-User-Id`。
- 角色落地：`@EnableMethodSecurity` + `@PreAuthorize("hasRole('ADMIN')")` 加在 `admin/` 控制器；模型级访问继续走你已有的 `AiModelAccessService`。

**验收标准**：
- 不带 token 调 `/api/v1/chat/**` 返回 401；带普通用户 token 调 `/api/v1/admin/**` 返回 403。
- 伪造/篡改/过期 token 被拒。
- 原有单测全绿（业务层因 `CurrentUserService` 解耦不受影响）。

**面试防守话术**：
> "身份不再信任前端请求头。登录签发 JWT，`JwtAuthenticationFilter` 校验签名和过期后还原可信主体写进 SecurityContext，业务层通过 `CurrentUserService` 拿身份——因为我早就把取身份这件事抽象成了一个接口，所以从 header 切到 JWT 没有动任何业务代码。"

**工期**：3–4 天　**可量化产出**：`______`（越权用例截图 / 测试通过数）

---

### [x] P0-2　可观测性：Micrometer → Prometheus → Grafana　✅ 已落地（2026-06-22）

> 已实现：actuator + micrometer-registry-prometheus，暴露 `/actuator/prometheus`；`GatewayMetrics` 在 `UsageEventService` 落库点统一导出业务指标——请求数(按 provider/model/status/route/tier)、token、计费成本、调用时延 P50/P95/P99，以及网关流式路径的 **TTFT 首 token 时延**。docker-compose 加 Prometheus(9090)+Grafana(3000)，预置数据源与「Honghu AI Gateway」看板(8 个面板)。指标低基数，含本地 vs 云端路由比例。

**为什么必须做**：`pom.xml` 没有 `actuator-metrics` / `micrometer-registry-prometheus` / OpenTelemetry，只有自写的 `monitor/MonitorController` 看 CPU 内存。**这是你"应用项目"和"infra 计划"之间的那座桥**——做完它，你 `llm-infra-gateway` 计划里 Epic 5 的"可观测体系"等于提前兑现了 80%，简历可以直接讲。

**具体任务**：
- `pom.xml` 增加 `spring-boot-starter-actuator` + `micrometer-registry-prometheus`，暴露 `/actuator/prometheus`。
- 在 `AiChatModelGatewayService` 埋点（这是核心，面试会问）：
  - `Timer` 记请求总时延、**首 token 时延（TTFT）**——流式分支里在 `doOnNext` 第一帧打点。
  - `Counter` 记**路由决策**：`llm_route_total{target="local|cloud_fallback", reason="..."}`，数据你已经有了（`resolveExecutableModel` / `resolveFallbackModelAfterLocalFailure`）。
  - `Counter` 记 token 与成本：从 `BillingService` 的 `CostBreakdown` 出。
  - `Counter` 记 `ai_usage_event` 的各种 Status（SUCCESS/FAILED/BLOCKED_BY_QUOTA/BLOCKED_BY_PRICING）——你枚举都现成。
- Provider 健康/回退次数做成指标。
- 提交一份 `observability/grafana-dashboard.json`（QPS / P99 / TTFT / 本地vs云端比例 / 成本趋势 / 错误率），docker-compose 里加 Prometheus + Grafana。

**验收标准**：
- `curl /actuator/prometheus` 能看到自定义指标。
- Grafana 面板能看到一次压测下的 P99、TTFT、路由比例曲线。

**面试防守话术**：
> "我没有只暴露 CPU 内存。我把网关的业务语义指标暴露出来：TTFT、路由 local/cloud 比例、按 Status 分类的请求数、按模型的 token 和成本。这样 SRE 能直接从 Grafana 看出'是不是云端兜底太多导致成本飙升'，而不是只知道机器还活着。"

**工期**：3–4 天　**可量化产出**：`______`（Grafana 截图 / 指标清单）

---

### [x] P0-3　结构化日志 + 全链路 traceId（MDC）　✅ 已落地（2026-06-22）

> 已实现：`TraceIdFilter`(最高优先级，早于 Security 链) 生成/透传 `traceId`+`requestId` 写入 MDC，回写响应头 `X-Trace-Id`，请求结束清理；`application.yml` 日志格式加入 `[%X{traceId}]`，一次请求全链路日志可按 traceId 串联。

**为什么做**：你 `AiCallContext` 里已经有 `requestId`，但日志没串起来。一次请求从 Controller → Gateway → Provider → Billing 没有统一 traceId，出问题没法追。

**具体任务**：
- `OncePerRequestFilter` 在入口生成/透传 `traceId`、`requestId` 写入 MDC。
- logback `%X{traceId}` 进日志格式；JSON 结构化输出（`logstash-logback-encoder` 可选）。
- traceId 随下游 vLLM/Provider 请求头透传；写进 `ai_usage_event`。

**验收标准**：同一次请求的所有日志行带同一个 traceId；usage event 能按 traceId 反查。

**工期**：1–2 天　**可量化产出**：`______`

---

## P1 · 从「应用」抬到「平台」（建议 1–2 周）

### [ ] P1-1　Kubernetes 部署（Helm / Kustomize + kind 本地跑通）

**为什么做**：现在只有 docker-compose。从"会做应用"到"会做平台"，K8s 是必经证明。**不一定要上云**，kind 本地集群跑通就够讲。

**具体任务**：Deployment/Service/Ingress/ConfigMap/Secret + readiness/liveness 探针（接 `/actuator/health`）；PG/Redis/Milvus 用 Helm chart 或 StatefulSet；写 `deploy/helm` 或 `deploy/kustomize`；README 加"kind 一键起"。

**验收**：`kind create cluster` 后 `kubectl apply` 全 Ready，前端能打通后端。
**工期**：3–5 天　**产出**：`______`（`kubectl get pods` 截图）

### [ ] P1-2　k6 压测 + 成本对比报告（真实数字）

**为什么做**：你目前**没有任何性能数字**，这是 AI 痕迹之外最容易被追的地方（"压测过吗？瓶颈在哪？"）。也是简历里唯一能写量化收益的来源。

**具体任务**：k6 脚本造 simple/复杂两类 prompt；分别跑"全云端"和"本地优先+云端兜底"两组；记录 P50/P95/P99、TTFT、tokens/s、各 provider 占比、按价格快照算的成本。**禁止写死"降低 85%"**——用公式从数据算。

**验收**：产出 `benchmark/reports/cost-report.md` 和 `latency-report.md`，含环境、模型、并发、路由比例、成本公式。
**工期**：2–3 天　**产出**：`______`（"在 N 并发下本地承担 X% 简单任务，综合成本降 Y%"）

### [ ] P1-3　RAG 质量评估闭环

**为什么做**：你 RAG 管线很完整（parser/cleaner/4 种 splitter/RRF/作用域兜底），但**没有质量度量**。面试问"你怎么证明 RRF 融合和兜底有效？"会答不上数据。这正是你区别于"我接了个 RAG"的地方。

**具体任务**：构造一个小评测集（问题→应命中文档）；写脚本算 recall@k / 命中率 / 兜底触发率；对比"仅关键词 vs 仅向量 vs RRF 融合"三组；对比"开/关作用域兜底"。

**验收**：产出 `rag-eval-report.md`，有三组对比数字。
**工期**：2–3 天　**产出**：`______`

---

## P2 · 真正冲 Infra 岗才做（选做，高投入）

> ⚠️ **诚实红线**：下面这些**没真跑通就别写进简历**。AI Infra 面试官会往死里问内核/调度细节，吹嘘必翻车。

### [ ] P2-1　Go OpenAI 兼容网关 MVP
从 `llm-infra-gateway` 计划里抠最能防守的：Go `net/http`/`gin` 实现 `/v1/chat/completions`（流式+非流式）、Provider 接口抽象、规则路由、Prometheus 指标。**这是 Infra 岗硬通货，且完全可控。**
**产出**：`______`

### [ ] P2-2　Kubebuilder `LLMEngine` Operator
CRD + Reconciler 管 vLLM Deployment/Service/ServiceMonitor，Status Conditions 回写，envtest 单测。讲清"reconcile 幂等 + 状态收敛"，别讲成"自动生成 YAML"。
**产出**：`______`

### [ ] P2-3　eBPF / EKS / DCGM —— ⚠️ 谨慎
**默认不做、不写简历**。除非你能当面讲清 verifier、map、ringbuf、kprobe vs tracepoint 差异，且真在 EKS 上跑通 + 有销毁 runbook 控成本。否则这是**最高风险吹嘘点**，性价比最低。想了解就当认知储备，不当简历项目。

---

## 一页纸优先级速查

| 优先级 | 项 | 工期 | 消除的面试红线 / 产出 |
|---|---|---|---|
| **P0-1** | Spring Security + JWT | 3–4d | 越权红线 |
| **P0-2** | Prometheus + Grafana | 3–4d | 无可观测红线 +（桥到 infra） |
| **P0-3** | traceId / 结构化日志 | 1–2d | 不可追踪 |
| P1-1 | K8s / Helm + kind | 3–5d | 应用→平台 |
| P1-2 | k6 压测 + 成本报告 | 2–3d | 唯一可量化收益 |
| P1-3 | RAG 评估闭环 | 2–3d | RAG 质量证明 |
| P2-1 | Go 网关 MVP | 1–2w | Infra 岗敲门 |
| P2-2 | LLMEngine Operator | 1–2w | Infra 岗敲门 |
| P2-3 | eBPF/EKS | —— | ⚠️ 不跑通不写 |

**最小可投递版本 = P0 全做完。** 想冲上海 Infra 岗再上 P1 + P2-1/P2-2。
