# ctf-agent

一个用 Go 编写的本地 CTF 解题代理。它会把题目放进隔离的 Docker sandbox 里，由一个或多个模型并行尝试解题。

## 构建 Sandbox

```bash
docker build -f sandbox/Dockerfile.sandbox -t ctf-sandbox .
```

题目目录结构如下：

```text
challenge/
  metadata.yml
  distfiles/
  workspace/      # 不存在时会自动创建
```

## 使用方法

只有附件和靶机地址时，直接用 `run`：

```bash
go run ./cmd/ctf-agent run \
  --target "nc host 31337" \
  --file ./chall.zip \
  --category pwn
```

Web 题目示例：

```bash
go run ./cmd/ctf-agent run \
  --target "http://host:8080" \
  --file ./source.zip \
  --category web \
  --name baby-web
```

`run` 会自动在 `./challenges` 下创建题目目录，把附件复制到 `distfiles/`，写入 `metadata.yml`，然后开始解题。

已有题目目录时，也可以直接跑：

```bash
go run ./cmd/ctf-agent single ./challenges/example
go run ./cmd/ctf-agent solve ./challenges
```

## 模型配置

通过环境变量配置模型和密钥：

```bash
export MODEL_SPECS=openai/gpt-5.4,anthropic/claude-opus-4-6
export OPENAI_API_KEY=...
export ANTHROPIC_API_KEY=...
```
