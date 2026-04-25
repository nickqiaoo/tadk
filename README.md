```
████████╗  █████╗  ██████╗  ██╗  ██╗
╚══██╔══╝ ██╔══██╗ ██╔══██╗ ██║ ██╔╝
   ██║    ███████║ ██║  ██║ █████╔╝
   ██║    ██╔══██║ ██║  ██║ ██╔═██╗
   ██║    ██║  ██║ ██████╔╝ ██║  ██╗
```

# TADK

A Go-based toolkit for building AI agents with LLMs, tools, and workflows.

## Features

- **Agents** – LLM agents with support for sequential, parallel, and loop workflows
- **Tools** – Built-in tools (bash, read/write file, grep, find, edit) plus custom function tools and MCP toolsets
- **Models** – Unified adapters for OpenAI, Anthropic, and Gemini
- **Runner** – Stateful execution loop with checkpoints and context transfer
- **Session Storage** – In-memory, file, or database (GORM) backends
- **TUI** – Terminal user interface for interactive agent sessions
- **REST API** – `adkrest` server for programmatic access
- **Temporal** – Durable workflow integration for long-running tool executions
- **Telemetry** – OpenTelemetry support out of the box

## Quick Start

```bash
go get github.com/Nickqiaoo/tadk
```

Run the TUI:
```bash
go run ./cmd/tui
```

Run the REST server:
```bash
go run ./cmd/launcher
```

See [`examples/`](./examples) for sample agents.

## License

MIT
