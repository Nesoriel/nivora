# Nivora

[English](README.md) | 简体中文

Nivora 是一个使用 Go 编写、可复用且支持多租户的智能客服 Agent Runtime。它使用 Eino 进行 Agent 编排，并通过版本化的 Provider API 访问产品数据。

Lumio 是 Nivora 的第一个接入方，但 Nivora 本身并不知道 Lumio 的数据库表、NextAuth、积分系统、生成流水线或 SQLite 实现。

## 当前能力

- 基于 Eino `ChatModelAgent`，通过官方 `eino-ext` 适配器接入火山引擎方舟 Ark
- 提供与具体业务无关的通用工具：知识检索、用户上下文、业务资源、故障诊断、账务流水和人工客服工单
- 稳定的 Server-Sent Events（SSE）协议
- 产品 BFF 与 Nivora 之间的私有服务鉴权
- 将短期用户上下文安全转发给产品 Provider API
- 健康检查、就绪检查和版本信息接口
- 默认仅监听回环地址的生产部署示例

## 架构

```text
浏览器
  -> 产品 BFF（会话、租户、品牌、限流）
     -> Nivora :3100（Eino Runtime，私有服务）
        -> 产品 Provider API（鉴权与业务事实来源）
           -> 产品服务与数据库
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
```

聊天请求必须由可信的产品 BFF 发起：

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
- Nivora 第一版只允许读取操作和 `case.create`，不自动退款、补偿、删除、取消或修改权限。

Provider 接口规范参见 [Provider API v1](docs/provider-api.md)。

## 开发

```bash
make fmt
make test
make vet
make build
```

## 路线图

1. 实现 Lumio Provider API 适配器和 BFF 客户端。
2. 根据 Provider Capability 动态注册 Tool。
3. 在 Nivora 自有存储中加入持久化会话、审计日志和客服工单。
4. 加入 Shadow 和 Canary 模式，实现安全的生产灰度发布。
5. 对未来的高风险写操作加入 Eino Interrupt/Resume 人工审批流程。
