# RAG 评测落地计划（RAGAS + DeepEval）· 企业级

> 目标：把"我做了个 RAG"升级成"我**度量并守护**了 RAG 质量"。
> 适用：Honghu AI 后端的 RAG 管线（parser/splitter/向量+关键词召回/RRF 融合/作用域兜底）。
> 形态：Java 应用保持不变，评测是**独立的 Python 离线 harness**（polyglot，企业标准做法）。

---

## 0. 先讲清楚：为什么要评测，为什么是这两个

### 为什么 RAG 必须评测
RAG 会在**两个完全不同的地方**坏掉，肉眼分不清：
1. **检索坏了**：没召回到对的 chunk（或对的排在后面、噪声太多）。
2. **生成坏了**：chunk 是对的，但模型**幻觉**、忽略上下文、答非所问。

不量化就只能"凭感觉调参"——改一个配置让 3 条 query 变好，可能让 30 条变差，你根本不知道。对你的求职目标更直接：**评测是你能写出可量化结论的唯一来源**（"RRF 融合把 context precision 从 0.62 提到 0.81"），这正是"做过 RAG"和"做过并度量 RAG"的分水岭。

### RAGAS 解决什么（科学/指标层）
- **RAG 专用指标，且能"分解"故障**：`context_precision`/`context_recall` 只看**检索**，`faithfulness`/`answer_relevancy` 只看**生成**——一眼定位是哪一层烂了。这是它的核心价值。
- 有**无参考(reference-free)**指标（faithfulness、answer_relevancy 不需要标准答案），可以大面积便宜地跑。
- 能从你自己的文档**合成测试集**（TestsetGenerator），快速冷启动数据集。
- 已是事实标准的"RAG 评测科学库"，面试认可度高。

### DeepEval 解决什么（CI/回归层）
- 它是 **"LLM 界的 Pytest"**：把评测变成带阈值的**断言**（`assert faithfulness > 0.8`），回归直接**让 CI 失败 / 拦住合并**。RAGAS 给你分数，DeepEval 给你**质量门**。
- 指标更广：除了把 RAGAS 那套 RAG 指标也实现了一遍，还有 **G-Eval（自定义评判标准）、Hallucination、Bias、Toxicity、Agentic/工具调用**等，覆盖生成侧安全。
- 开发体验 + 回归看板（Confident AI）+ 红队（DeepTeam）。

### 你到底要不要两个都用？（诚实建议）
它们在核心 RAG 指标上**高度重叠**。两个都上的正当理由**不是"大家都用"，而是角色分工**：

| 工具 | 角色 | 产出物 |
|---|---|---|
| **RAGAS** | **离线实验 & 报告**：对比不同检索配置、生成基准数字 | `reports/ragas-*.md`（你的简历 artifact） |
| **DeepEval** | **CI 质量门 & 安全**：pytest 断言挡回归 + 幻觉/偏见 | 绿/红 CI + 回归看板 |

- **只想做一个**：选 **DeepEval**（它含 RAG 指标 + CI 门 + 自定义 G-Eval，覆盖 ~80%，CI 门的故事更能打）。
- **两个都做**（推荐，且不啰嗦）：RAGAS 做离线基准报告 + 合成数据，DeepEval 做 CI 门 + 安全指标。两个不同的面试谈点。
- **反 cargo-cult 的关键**：别因为库里有 12 个指标就全开。挑 **4–5 个**映射到你真实故障模式的，设阈值，并**用人工标注校准 judge**（见 Phase 3）——最后这步才是"企业级"和"我跑了个 RAGAS"的区别。

---

## 1. 架构：Java RAG + Python 评测 harness

```
            ┌─────────────────────────┐
            │  golden dataset (jsonl)  │  问题 + 标准答案 + 相关chunk_id
            └────────────┬────────────┘
                         │ 每条问题
                         ▼
  Python harness ──HTTP──►  Java: POST /api/v1/rag/eval/answer
   (RAGAS+DeepEval)        返回 {answer, contexts[], chunkIds[], model, latencyMs}
                         │
        ┌────────────────┴───────────────┐
        ▼                                ▼
   RAGAS 离线基准                    DeepEval pytest 断言
   reports/*.md + csv               assert_test → CI 绿/红
```

**唯一需要改 Java 的地方**：暴露一个"评测友好"端点，把**检索到的上下文**和答案一起返回（评测必须拿到 contexts，不能只拿最终答案）。你已经有 `RagPipeline` / `RagResult`，这是个薄封装。

---

## 2. Phase 0 · 数据与契约（1–2 天）

### 2.1 评测端点（Java，复用现有管线）
```
POST /api/v1/rag/eval/answer
{ "question": "...", "scope": "FILE|CHAT|SESSION", "retrievalMode": "keyword|vector|rrf",
  "topK": 4, "fusion": true, "scopeFallback": true }
→
{ "answer": "...", "contexts": ["chunk1 文本", "chunk2 文本"], "chunkIds": ["c1","c2"],
  "model": "deepseek-v4-flash", "latencyMs": 1234 }
```
- 复用 `RagPipeline` 拿 `RagResult`（候选+融合后片段），再走网关生成答案。
- **关键**：把 `retrievalMode/topK/fusion/scopeFallback` 做成**单请求可覆盖**的参数，这样 harness 不用重启服务就能扫描配置。
- 安全：该端点放管理/内网，或要 ADMIN（别把内部 chunk 暴露给公网）。

### 2.2 Golden dataset（schema 固定，git 版本化）
`rag-eval/datasets/golden.v1.jsonl`：
```json
{"id":"q001","question":"……","ground_truth":"……","relevant_chunk_ids":["c1","c2"],"tags":["faq","scope:file"]}
```
- 规模：先 **30–80 条**覆盖真实场景（FAQ、跨文档、长上下文、易幻觉、作用域兜底）。
- 来源：① 手工挑你语料里的真实问题；② 用 **RAGAS TestsetGenerator** 从你文档自动合成一批，**再人工筛**（合成的必须人审，否则 garbage in）。
- 每条带 `tags`，方便按场景切片看指标。

**验收**：端点能稳定返回 `{answer, contexts}`；golden.v1 至少 30 条且人工过审。

---

## 3. Phase 1 · RAGAS 离线基准（报告 artifact，2–3 天）

### 3.1 选指标（4 个，分别盯检索/生成）
| 指标 | 盯哪层 | 要不要 ground_truth |
|---|---|---|
| `context_precision` | 检索：相关片段是否排在前面 | 需要 |
| `context_recall` | 检索：该召回的都召回了吗 | 需要 |
| `faithfulness` | 生成：答案是否忠于上下文（反幻觉） | 不需要 |
| `answer_relevancy` | 生成：答案是否切题 | 不需要 |
| （选）`noise_sensitivity` | 检索噪声对答案的影响 | 需要 |

### 3.2 核心实验：用评测**证明你的检索设计有效**
对**同一个 golden set**跑多组配置，出对比表——这直接利用你已有的 RAG 开关：

| 配置 | context_precision | context_recall | faithfulness | answer_relevancy |
|---|---|---|---|---|
| keyword only | … | … | … | … |
| vector only | … | … | … | … |
| **RRF 融合** | … | … | … | … |
| RRF + 作用域兜底 | … | … | … | … |

→ 输出 `rag-eval/reports/ragas-<date>.md` + `.csv` + 柱状图。**这张表就是简历那句量化结论的出处。**

### 3.3 judge 配置（成本/确定性）
- judge LLM 用你已有的 **DeepSeek / Qwen**（或 gpt-4o-mini），**temperature=0**，开缓存。
- embedding 用你 RAG 同款（保证一致）。
- 记录 judge 模型 + 版本到报告里（可复现）。

**验收**：一条命令生成对比报告；能回答"哪个配置好、好多少、用什么算的"。

---

## 4. Phase 2 · DeepEval CI 质量门（回归防护，2 天）

### 4.1 pytest 断言（挑 10–20 条"冒烟子集"，每条都设阈值）
```python
# rag-eval/tests/test_rag_quality.py
from deepeval import assert_test
from deepeval.test_case import LLMTestCase
from deepeval.metrics import (FaithfulnessMetric, AnswerRelevancyMetric,
                              ContextualPrecisionMetric, ContextualRecallMetric, GEval)

def build_case(row):
    r = call_eval_endpoint(row["question"])          # 调 Java /rag/eval/answer
    return LLMTestCase(input=row["question"], actual_output=r["answer"],
                       retrieval_context=r["contexts"], expected_output=row["ground_truth"])

def test_rag(row):  # 参数化遍历冒烟子集
    case = build_case(row)
    assert_test(case, [
        FaithfulnessMetric(threshold=0.8),
        AnswerRelevancyMetric(threshold=0.8),
        ContextualPrecisionMetric(threshold=0.7),
        ContextualRecallMetric(threshold=0.7),
        GEval(name="中文且只引用上下文",
              criteria="答案必须用中文，且只能基于 retrieval_context，不得编造",
              evaluation_params=["actual_output", "retrieval_context"], threshold=0.8),
    ])
```

### 4.2 CI 接法（省钱 + 不阻塞）
- GitHub Actions 单独 job：**手动触发 / nightly / 仅 RAG 相关 PR**（因为要花 LLM judge 钱 + 要把 app 跑起来）。
- 冒烟子集（10–20 条）跑得快；全量 golden 放 nightly。
- 失败即红 → 阻止把 RAG 质量改差的 PR 合进去。
- （选）接 Confident AI 看板做趋势回归。

**验收**：故意把 topK 调成 1 或关掉融合 → CI 变红；恢复 → 变绿。

---

## 5. Phase 3 · 企业级严谨（这部分才是"可信"，1–2 天）

1. **judge 校验（最重要）**：人工标注 ~30 条（faithful 与否、相关与否），算 judge 分数与人工的**相关性/一致率**，写进报告。能讲"我验证过我的评测本身可信"——这是 90% 的人不做、但面试官最吃的一点。
2. **成本与确定性**：judge temp=0 + 缓存；分层采样（PR 跑冒烟、nightly 跑全量）；设月度调用预算上限。
3. **指标治理**：每个阈值写清 owner + 触发动作（拦合并 / 告警），避免"指标摆设"。
4. **漂移监控**：定时（nightly/weekly）跑离线基准，把分数**落库 + 画趋势**。可把 4 个指标作为 gauge 推到你刚搭的 **Prometheus/Grafana**，和系统指标并排看——一个很漂亮的"应用质量也可观测"的整合故事。
5. **数据闭环**：把线上答错的真实 query 回灌进 golden set，dataset 版本化（v1→v2）。

---

## 6. 目录结构（建议放后端仓库或同级 `rag-eval/`）
```
rag-eval/
├── pyproject.toml            # uv/poetry；deps: ragas, deepeval, pytest, httpx, pandas
├── datasets/golden.v1.jsonl
├── harness/
│   ├── client.py             # 调 Java /rag/eval/answer
│   ├── judge.py              # 自定义 DeepSeek/Qwen judge 包装
│   └── run_ragas.py          # Phase 1：跑基准 + 出报告
├── tests/test_rag_quality.py # Phase 2：DeepEval pytest 断言
├── reports/                  # ragas-*.md / *.csv / 图
└── README.md                 # 一键跑法 + judge 配置说明
```

---

## 7. 里程碑与验收
| 阶段 | 工期 | 验收 |
|---|---|---|
| P0 数据与契约 | 1–2d | `/rag/eval/answer` 返回 contexts；golden.v1 ≥30 条过审 |
| P1 RAGAS 基准 | 2–3d | 一键出多配置对比报告，有可复现数字 |
| P2 DeepEval CI 门 | 2d | 故意改差 → CI 红；恢复 → 绿 |
| P3 企业级严谨 | 1–2d | judge 与人工相关性报告 + 指标趋势落库 |

**最小可交付 = P0 + P1**（拿到那张对比表就能写简历）。想要"守护质量"的故事再上 P2/P3。

---

## 8. 简历 bullets（做完后按真实数字填）
- 搭建 RAG 评测体系：用 **RAGAS** 在 ___ 条 golden set 上对比检索配置，**RRF 融合**把 context precision 从 ___ 提升到 ___；用 **DeepEval** 把 faithfulness/answer relevancy 等指标做成 **pytest 阈值断言**接入 CI，防止 RAG 质量回归。
- 用人工标注校验 LLM-judge 可信度（相关性 ___），并将评测指标推送 Prometheus/Grafana 做质量漂移监控。
