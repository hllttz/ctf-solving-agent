# ctf-agent

Autonomous local CTF solving agent written in Go. It runs one or more model-backed solvers against challenge directories in isolated Docker sandboxes.

## Build Sandbox

```bash
docker build -f sandbox/Dockerfile.sandbox -t ctf-sandbox .
```

The sandbox expects challenge directories shaped like:

```text
challenge/
  metadata.yml
  distfiles/
  workspace/      # created automatically when missing
```

## Run

For a manual challenge where you only have attachments and a target:

```bash
go run ./cmd/ctf-agent run \
  --target "nc host 31337" \
  --file ./chall.zip \
  --category pwn
```

For a web target:

```bash
go run ./cmd/ctf-agent run \
  --target "http://host:8080" \
  --file ./source.zip \
  --category web \
  --name baby-web
```

The `run` command creates a challenge directory under `./challenges`, copies attachments into `distfiles/`, writes `metadata.yml`, then starts solving.

```bash
go run ./cmd/ctf-agent single ./challenges/example
go run ./cmd/ctf-agent solve ./challenges
```

Configure models and keys with environment variables:

```bash
export MODEL_SPECS=openai/gpt-5.4,anthropic/claude-opus-4-6
export OPENAI_API_KEY=...
export ANTHROPIC_API_KEY=...
```
