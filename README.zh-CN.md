# Nivora

[English](README.md) | 简体中文

Nivora 是一个使用 Go 编写、可复用且支持多租户的智能客服 Agent Runtime。它使用 Eino 进行 Agent 编排，并通过版本化的 Provider API 访问产品数据。

Lumio 是 Nivora 计划接入的第一个业务系统，但 Nivora 本身并不知道 Lumio 的数据库表、NextAuth、积分系统、生成流水线或 SQLite 实现。

> Nivora 目前已经具备用于构建生产候选版本的工程能力和验收工具，但尚不能直接认定为生产验收完成。真实 Provider 契约、火山引擎环境、压力与故障恢复测试，以及经授权脱敏的 Shadow 对比，仍需在公司的隔离预发布环境中实际通过。

## 当前能力

- 基于 Eino `ChatModelAgent`，通过官方 `eino-ext` 适配器接入火山引擎方舟 Ark
- 在开始流式输出前，按顺序进行多个方舟推理接入点故障转移
- 可选接入 CozeLoop 链路追踪与 PromptHub 版本化策略，并实施严格脱敏和本地安全回退
- 根据 Provider Capability 与可信 BFF 授予的 Scope 动态注册 Tool
- 提供与具体业务无关的通用 Tool：知识检索、用户上下文、业务资源、故障诊断、账务流水和人工客服工单
- 提供基于官方 Eino VikingDB Retriever 的 Provider 侧已审核知识参考服务
- 对语义检索结果再次校验租户、审核状态、有效期、来源版本和最低置信度
- 使用 SQLite 支持开发测试，并使用 PostgreSQL 支持生产环境中的公开会话、运行元数据、脱敏 Tool 审计和工单引用
- 对请求、消息和 Tool 调用实施确定性幂等保护，并提供按租户隔离的会话转录接口
- 提供客服回答与知识召回的 JSONL 黑盒回归评测工具
- 提供针对鉴权、租户、Scope、请求结构和历史消息注入的确定性 HTTP 安全探针
- 提供 SSE 压力测试，统计首 Token 与完成时延的 p50/p95/p99、成功率和错误分布
- 提供 Baseline/Candidate Shadow 对比，结果仅保存答案哈希和字节数，不保存答案正文
- 提供带确定性知识、失败作品、退款流水、幂等工单、延迟和 429/5xx 故障注入的 synthetic Provider
- 提供可手动触发的预发布验收工作流，执行安全探针、客服回归、压力测试和 Shadow 对比
- 对幂等 Provider 读取执行有界重试，并为客服工单生成稳定幂等键
- 带心跳的稳定 Server-Sent Events（SSE）协议
- 产品 BFF 与 Nivora 之间的私有服务鉴权
- 带短期缓存的真实 Provider 与存储就绪检查
- 全局并发限制和排队超时保护
- Prometheus 兼容的 Agent 与进程运行指标
- 默认仅监听回环地址的生产部署示例

## 架构

```text
浏览器
  -> 产品 BFF（会话、租户、品牌、Scope、限流）
     -> Nivora :3100（Eino Runtime，私有服务）
        -> Nivora 会话与审计数据库
        -> 产品 Provider API（鉴权与业务事实来源）
           -> 产品服务与业务数据库
           -> 已审核知识服务 :3110
              -> VikingDB
```

Nivora 不接受聊天请求动态指定 Provider 地址，也不会直接连接产品业务数据库或 VikingDB。部署时配置的 Provider 始终是业务事实的唯一来源。

## 本地运行

```bash
cp .env.example .env
set -a; source .env; set +a
go run ./cmd/nivora
```

常用接口：

```bash
curl http://127.0.0.1:3100/healthz
curl -i http://127.0.0.1:3100/readyz
curl http://127.0.0.1:3100/metrics
curl -H 'X-Nivora-Key: replace-with-a-long-random-secret' \
  http://127.0.0.1:3100/v1/conversations/conv-id/transcript
```

聊天请求必须由可信的产品 BFF 发起。BFF 必须丢弃浏览器提交的租户与 Principal 信息，并在服务端验证会话后重新注入可信数据。

```bash
curl -N http://127.0.0.1:3100/v1/chat/stream \
  -H 'Content-Type: application/json' \
  -H 'X-Nivora-Key: replace-with-a-long-random-secret' \
  -H 'Authorization: Bearer short-lived-provider-context' \
  -d '{
    "question": "我的视频为什么失败，积分退了吗？",
    "tenant": {
      "id": "lumio",
      "brand": {
        "key": "lumio",
        "name": "Lumio",
        "agent_name": "Lumio 智能客服"
      }
    },
    "principal": {
      "authenticated": true,
      "scopes": [
        "knowledge:read",
        "customer:read",
        "resource:read",
        "transaction:read",
        "case:create"
      ]
    }
  }'
```

流式响应使用具名 SSE 事件：

- `message.delta`
- `tool.started`
- `tool.finished`
- `done`
- `error`

Tool 的原始结果不会直接返回浏览器，而是仅保留在本次 Agent Run 内部。

## 安全边界

- 将 Nivora 及其参考服务绑定到回环地址或私有 VPC 地址。
- 不要通过公网反向代理暴露 `3100`、`3110` 或 synthetic Provider。
- 产品到 Nivora、Nivora 到 Provider、Provider 到知识服务应分别使用不同的服务密钥。
- Provider API 必须校验用户对业务资源的归属，并剥离内部敏感字段。
- 匿名请求只能获得明确授予的知识检索与创建客服工单 Scope。
- 持久化存储只保存公开消息和脱敏审计元数据，不保存思维链、Bearer Context、Tool 原始载荷或业务内部配方。
- Shadow 结果只保存回答哈希和字节数，不保存回答正文。
- synthetic Provider 只能用于隔离验收环境，严禁接收生产客户流量。
- Nivora 当前只允许读取操作和具有幂等保护的 `case.create`，不自动退款、补偿、删除、取消或修改权限。

## 生产验收命令

```bash
make probe          # 确定性 HTTP 安全边界测试
make eval           # 客服能力回归评测
make knowledge-eval # 已审核知识召回评测
make load           # 有界 SSE 压力与时延测试
make shadow         # 隐私安全的 Baseline/Candidate 对比
make test-provider  # 隔离预发布环境使用的 synthetic Provider
```

仓库还提供 `.github/workflows/acceptance.yml` 手动验收工作流。它依赖受保护的 `nivora-staging` Environment 变量和 Secrets，不会自动对生产环境发起测试。

## 文档

- [Runtime API v1](docs/runtime-api.md)
- [Provider API v1](docs/provider-api.md)
- [CozeLoop 接入](docs/cozeloop.md)
- [VikingDB 已审核知识服务](docs/approved-knowledge.md)
- [持久化会话与审计](docs/durable-storage.md)
- [客服回归评测](docs/evaluation.md)
- [生产验收门槛](docs/production-acceptance.md)
- [火山引擎生产技术栈](docs/volcengine-production-stack.md)

## 开发

```bash
make fmt
make test
make vet
make build
```

## 路线图

1. 在公司的隔离预发布环境中执行完整验收门槛，确定获批的 SLO、质量、成本和回滚基线。
2. 在未来引入任何高风险写入 Tool 前，使用 Eino Interrupt/Resume 加入明确的人工审批流程。
