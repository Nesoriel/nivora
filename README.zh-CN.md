# Nivora

[English](README.md) | 简体中文

Nivora 是一个使用 Go 编写、可复用且支持多租户的智能客服 Agent Runtime。它使用 Eino 进行 Agent 编排，并通过版本化的 Provider API 访问产品数据。

Lumio 是 Nivora 计划接入的第一个业务系统，但 Nivora 本身并不知道 Lumio 的数据库表、NextAuth、积分系统、生成流水线或 SQLite 实现。

> Nivora 当前已经具备业务接入所需的 Runtime 基础，但仍处于生产加固阶段。只有真实 Provider、安全评测、压力测试和影子流量评测全部通过后，才能认定为生产验收完成。

## 当前能力

- 基于 Eino `ChatModelAgent`，通过官方 `eino-ext` 适配器接入火山引擎方舟 Ark
- 在开始流式输出前，按顺序进行多个方舟推理接入点故障转移
- 根据 Provider Capability 与可信 BFF 授予的 Scope 动态注册 Tool
- 提供与具体业务无关的通用 Tool：知识检索、用户上下文、业务资源、故障诊断、账务流水和人工客服工单
- 对幂等 Provider 读取执行有界重试，并为客服工单生成稳定幂等键
- 带心跳的稳定 Server-Sent Events（SSE）协议
- 产品 BFF 与 Nivora 之间的私有服务鉴权
- 带短期缓存的真实 Provider 就绪检查
- 全局并发限制和排队超时保护
- Prometheus 兼容运行指标
- 默认仅监听回环地址的生产部署示例

## 架构

```text
浏览器
  -> 产品 BFF（会话、租户、品牌、Scope、限流）
     -> Nivora :3100（Eino Runtime，私有服务）
        -> 产品 Provider API（鉴权与业务事实来源）
           -> 产品服务与数据库 / 已审核知识服务
```

Nivora 不接受聊天请求动态指定 Provider 地址，也不会直接连接产品数据库。部署时配置的 Provider 始终是业务事实的唯一来源。

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

- 将 Nivora 绑定到 `127.0.0.1` 或私有 VPC 地址。
- 不要通过公网反向代理暴露 `3100` 端口。
- 产品到 Nivora、Nivora 到 Provider 应使用不同的服务密钥。
- Provider API 必须校验用户对业务资源的归属，并剥离内部敏感字段。
- 匿名请求只能获得明确授予的知识检索与创建客服工单 Scope。
- Nivora 当前只允许读取操作和具有幂等保护的 `case.create`，不自动退款、补偿、删除、取消或修改权限。

## 文档

- [Runtime API v1](docs/runtime-api.md)
- [Provider API v1](docs/provider-api.md)
- [火山引擎生产技术栈](docs/volcengine-production-stack.md)

## 开发

```bash
make fmt
make test
make vet
make build
```

## 路线图

1. 接入 CozeLoop 链路追踪、Prompt 版本管理和离线评测，并实施严格脱敏。
2. 建设 Provider 管理的知识检索链路，可使用 VikingDB 存储已审核知识向量。
3. 在 Nivora 自有存储中加入持久化会话、审计日志和客服工单。
4. 加入 Shadow 和 Canary 模式，实现安全的生产灰度发布。
5. 对未来的高风险写操作加入 Eino Interrupt/Resume 人工审批流程。
