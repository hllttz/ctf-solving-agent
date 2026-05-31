# ctf-agent

`ctf-agent` 是一个用 Go 编写的本地 CTF 解题代理。它会为每个 solver 创建隔离的 Docker sandbox，把题目附件挂载进去，然后让一个或多个模型并行尝试解题。多个 solver 之间可以通过消息总线共享发现，operator 也可以在运行中发送提示、查看状态、终止或手动启动本地题目。

当前版本专注于本地题目目录和手工输入题目。solver 找到 flag 后会通过 `report_flag` 记录结果。

## 功能概览

- 多模型并发：同一道题可以同时由多个模型求解，先找到 flag 的结果胜出。
- Docker sandbox：题目附件只读挂载到 `/challenge/distfiles`，工作目录挂载到 `/workspace`。
- 本地题目工作流：支持已有题目目录，也支持用靶机地址和附件快速创建题目。
- 跨 solver 协作：solver 可以 `post_finding`、`notify_coordinator`，其他 solver 会周期性收到共享发现。
- operator 控制：运行中可通过本地 HTTP 接口查看状态、广播提示、定向 bump、kill、读取 trace、启动本地题目。
- 成本与 trace：记录模型 token usage、估算成本，并把工具调用、模型响应和 usage 写入 `logs/*.jsonl`。

## 环境要求

- Go 1.24+
- Docker
- 已构建的 sandbox 镜像
- 至少一个模型 provider 的 API Key

构建 sandbox：

```bash
docker build -f sandbox/Dockerfile.sandbox -t ctf-sandbox .
```

## 题目目录格式

已有题目推荐放在 `./challenges/<slug>/` 下：

```text
challenges/
  baby-web/
    metadata.yml
    distfiles/
      source.zip
    workspace/      # 不存在时会自动创建
```

`metadata.yml` 示例：

```yaml
name: baby-web
category: web
description: Find the flag in the web service.
connection_info: http://example.com:8080
files:
  - source.zip
```

常用字段：

```yaml
name: chall-name
category: pwn        # web / pwn / rev / crypto / forensics / misc
description: ...
value: 500
connection_info: nc host 31337
tags:
  - heap
hints:
  - content: check the login flow
```

也兼容 `host`、`port`、`service_type` 字段。若存在连接信息，prompt 会要求 solver 第一步优先探测远端服务。

## 使用方法

### 1. 手工创建并求解题目

只有附件和靶机地址时，直接使用 `run`：

```bash
go run ./cmd/ctf-agent run \
  --target "nc host 31337" \
  --file ./chall.zip \
  --category pwn \
  --name baby-pwn
```

Web 题目：

```bash
go run ./cmd/ctf-agent run \
  --target "http://host:8080" \
  --file ./source.zip \
  --category web \
  --name baby-web
```

`run` 会在 `CHALLENGES_DIR`，默认 `./challenges`，下创建题目目录，复制附件到 `distfiles/`，写入 `metadata.yml`，然后启动求解。

在交互式终端里，如果缺少 `--target`、`--file`、`--name` 等参数，程序会提示输入；非交互环境不会提示。

### 2. 求解单个已有题目

```bash
go run ./cmd/ctf-agent single ./challenges/baby-web
```

### 3. 求解目录下所有题目

```bash
go run ./cmd/ctf-agent solve ./challenges
```

`solve` 会扫描所有包含 `metadata.yml` 的子目录，并按 `MAX_CONCURRENT_CHALLENGES` 控制并发题目数。每道题内部仍会按 `MODEL_SPECS` 启动多个 solver。

## 模型配置

通过环境变量配置模型和密钥：

```bash
export MODEL_SPECS=openai/gpt-5.4,anthropic/claude-opus-4-6
export OPENAI_API_KEY=...
export ANTHROPIC_API_KEY=...
export GEMINI_API_KEY=...
export DEEPSEEK_API_KEY=...
```

只使用 DeepSeek：

```bash
export MODEL_SPECS=deepseek/deepseek-v4-flash
export DEEPSEEK_API_KEY=...
```

默认模型：

```text
claude-sdk/claude-opus-4-6
claude-sdk/claude-sonnet-4-6
openai/gpt-5.4
```

支持的 provider 前缀：

- `openai/...`
- `anthropic/...`
- `claude-sdk/...`
- `gemini/...`
- `google/...`
- `deepseek/...`
- `codex/...`，走 OpenAI 兼容路径

如果使用 OpenAI 兼容代理，可以设置：

```bash
export OPENAI_BASE_URL=https://proxy.example.com/v1/chat/completions
```

DeepSeek 默认使用 `https://api.deepseek.com/chat/completions`。如果需要代理或私有网关：

```bash
export DEEPSEEK_BASE_URL=https://proxy.example.com
```

## 其他配置

```bash
export SANDBOX_IMAGE=ctf-sandbox
export CONTAINER_MEMORY_LIMIT=16g
export MAX_CONCURRENT_CHALLENGES=10
export CHALLENGES_DIR=./challenges
export SKILLS_DIR=./skills
export MSG_ADDR=127.0.0.1:0
```

说明：

- `SANDBOX_IMAGE`：Docker sandbox 镜像名。
- `CONTAINER_MEMORY_LIMIT`：每个 solver 容器的内存限制。
- `MAX_CONCURRENT_CHALLENGES`：同时求解的题目数量上限。
- `MODEL_SPECS`：逗号分隔的 solver 模型列表。
- `SKILLS_DIR`：额外 prompt 技能目录，会拼接到题目 prompt 后。
- `MSG_ADDR`：operator HTTP 服务监听地址，默认随机端口。

## Operator 接口

运行 `solve` 时会启动本地 operator HTTP 服务，日志里会显示地址，例如：

```text
operator message server listening on http://127.0.0.1:9400/msg
```

常用接口：

```bash
# 查看当前状态、结果、成本、usage
curl http://127.0.0.1:9400/status

# 给所有运行中的 swarm 广播提示
curl -X POST http://127.0.0.1:9400/msg \
  -H 'Content-Type: application/json' \
  -d '{"message":"优先检查 /admin 和 JWT secret"}'

# 给指定题目广播提示
curl -X POST http://127.0.0.1:9400/broadcast \
  -H 'Content-Type: application/json' \
  -d '{"challenge":"baby-web","message":"重点看 cookie 签名"}'

# 定向 bump 某个模型
curl -X POST http://127.0.0.1:9400/bump \
  -H 'Content-Type: application/json' \
  -d '{"challenge":"baby-web","model":"openai/gpt-5.4","insights":"其他 solver 发现存在 SSTI"}'

# 终止某道题的 swarm
curl -X POST http://127.0.0.1:9400/kill \
  -H 'Content-Type: application/json' \
  -d '{"challenge":"baby-web"}'

# 启动一个本地已有题目
curl -X POST http://127.0.0.1:9400/spawn \
  -H 'Content-Type: application/json' \
  -d '{"challenge":"baby-web"}'

# 读取最近 trace
curl 'http://127.0.0.1:9400/trace?challenge=baby-web&model=gpt-5.4&last=40'
```

`/spawn` 只接受本地 `CHALLENGES_DIR/<challenge>/metadata.yml` 已存在的题目。

## Solver 工具

solver 可用工具包括：

- `bash`：在 sandbox 内执行命令。
- `read_file` / `write_file` / `list_files`：读取、写入、列目录。
- `view_image`：识别图片文件并给出分析建议。
- `web_fetch`：从宿主侧发 HTTP 请求，默认阻止内网/private IP。
- `webhook_create` / `webhook_get_requests`：创建和读取 webhook.site 回连请求。
- `post_finding` / `check_findings`：跨 solver 分享发现。
- `notify_coordinator`：向 coordinator/operator 通知重要发现。
- `report_flag`：记录最终 flag。

## 输出、日志与成本

运行结束后会输出：

- 每道题状态。
- 找到的 flag 和方法。
- step 数。
- 成本汇总。

trace 写入 `logs/trace-<challenge>-<model>-<timestamp>.jsonl`，包含：

- `start`
- `tool_call`
- `tool_result`
- `model_response`
- `usage`
- `bump`
- `finish`
- `error`

成本估算来自模型响应里的 token usage。若 provider 没返回 usage，该次调用不会计入成本。

## 开发与测试

普通测试：

```bash
go test ./...
```

如果默认 Go build cache 不可写，可以使用项目内 cache：

```bash
GOCACHE=$PWD/.cache/go-build \
GOMODCACHE=$PWD/.cache/go-mod \
go test ./...
```
