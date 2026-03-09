# mcpgen

A code generator for [MCP (Model Context Protocol)](https://modelcontextprotocol.io/) tool servers. Scaffolds complete Go projects with a full JSON-RPC server, CLI mode, build system, and Claude Code integration — ready to run with a single `just build`.

## Features

- Generates a complete, working MCP tool server in Go (stdio transport)
- Dual-mode operation: runs as an MCP server or a standalone CLI tool
- Configurable tool parameters with JSON Schema types
- Produces ready-to-use build files (`justfile`, `go.mod`, `.mcp.json`)
- Pre-configured Claude Code integration (`.claude/settings.local.json`)
- Refuses to overwrite existing projects

## Prerequisites

- [Go](https://go.dev/dl/) 1.23+
- [just](https://github.com/casey/just) (command runner)
- macOS (generates `codesign` step in justfile)

## Installation

```bash
git clone https://github.com/hegner123/mcpgen.git
cd mcpgen
just install
```

This builds the binary, signs it, and copies it to `/usr/local/bin/`.

## Usage

### As an MCP Server

Add mcpgen to your Claude Code configuration:

```bash
claude mcp add mcpgen -- mcpgen
```

Then invoke it through Claude Code by asking it to generate a new MCP tool.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `name` | string | yes | Tool name (used for binary, module, and MCP server name) |
| `description` | string | yes | One-line description of what the tool does |
| `params` | array | yes | Array of parameter definitions for the generated tool |
| `output_dir` | string | no | Directory to create the project in (default: `~/Documents/Code/terse-mcp/<name>`) |
| `module` | string | no | Go module path (default: `github.com/hegner123/<name>`) |

Each entry in `params` is an object:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Parameter name |
| `type` | string | JSON Schema type: `string`, `boolean`, `integer`, `number`, `array`, `object` |
| `description` | string | Parameter description |
| `required` | boolean | Whether the parameter is required |
| `default` | any | Default value (optional) |

### As a CLI Tool

```bash
mcpgen --cli \
  --name mytool \
  --description "Does something useful" \
  --params '[{"name":"input","type":"string","description":"Input file path","required":true}]'
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--cli` | Run in CLI mode (required for direct invocation) |
| `--name` | Tool name (required) |
| `--description` | Tool description (required) |
| `--output-dir` | Output directory (optional) |
| `--module` | Go module path (optional) |
| `--params` | Tool parameters as a JSON array (optional) |

## Generated Project Structure

```
<name>/
├── main.go                      # Full MCP server + CLI with your parameters
├── go.mod                       # Go module definition
├── justfile                     # Build, install, clean, test targets
├── .mcp.json                    # MCP server registration for Claude Code
├── .gitignore                   # Go + IDE + OS ignores
└── .claude/
    └── settings.local.json      # Claude Code permission allowlist
```

The generated `main.go` includes:
- Complete MCP JSON-RPC server (initialize, tools/list, tools/call)
- CLI mode with flag parsing for all defined parameters
- Required parameter validation in both modes
- Signal handling (SIGINT/SIGTERM) for graceful shutdown
- A stub `execute()` function where you implement your tool logic

## Example

Generate a tool that formats JSON files:

```bash
mcpgen --cli \
  --name jsonformat \
  --description "Format JSON files with consistent indentation" \
  --params '[
    {"name":"file","type":"string","description":"Path to JSON file","required":true},
    {"name":"indent","type":"integer","description":"Number of spaces for indentation","required":false,"default":2},
    {"name":"sort_keys","type":"boolean","description":"Sort object keys alphabetically","required":false}
  ]'
```

This creates `~/Documents/Code/terse-mcp/jsonformat/` with a complete MCP server. To start using it:

```bash
cd ~/Documents/Code/terse-mcp/jsonformat
just build
# Edit main.go — implement the execute() function
just install
claude mcp add jsonformat -- jsonformat
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

[MIT](LICENSE)
