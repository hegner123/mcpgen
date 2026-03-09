package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"
	"unicode"
)

// Exit codes for CLI mode
const (
	ExitSuccess = 0
	ExitError   = 1
)

// Param describes a single MCP tool parameter for the generated tool.
type Param struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
}

// GenerateConfig holds everything needed to scaffold a new MCP tool.
type GenerateConfig struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	OutputDir   string  `json:"output_dir"`
	Module      string  `json:"module"`
	Params      []Param `json:"params"`
}

// GenerateResult describes what was created.
type GenerateResult struct {
	OutputDir    string   `json:"output_dir"`
	FilesCreated []string `json:"files_created"`
}

// ---------- MCP JSON-RPC types ----------

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
	Capabilities    Capabilities `json:"capabilities"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Capabilities struct {
	Tools map[string]bool `json:"tools"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required"`
}

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
}

type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ToolCallResult struct {
	Content []ContentItem `json:"content"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ---------- main ----------

func main() {
	cliMode := flag.Bool("cli", false, "Run in CLI mode")
	name := flag.String("name", "", "Tool name (required in CLI mode)")
	description := flag.String("description", "", "Tool description (required in CLI mode)")
	outputDir := flag.String("output-dir", "", "Output directory (defaults to $HOME/Documents/Code/terse-mcp/<name>)")
	module := flag.String("module", "", "Go module path (defaults to github.com/hegner123/<name>)")
	paramsJSON := flag.String("params", "", `Tool params as JSON array: [{"name":"x","type":"string","description":"...","required":true}]`)
	flag.Parse()

	if *cliMode {
		runCLI(*name, *description, *outputDir, *module, *paramsJSON)
		return
	}

	runMCPServer()
}

func runCLI(name, description, outputDir, module, paramsJSON string) {
	if name == "" {
		fmt.Fprintln(os.Stderr, "Error: --name is required")
		flag.Usage()
		os.Exit(ExitError)
	}
	if description == "" {
		fmt.Fprintln(os.Stderr, "Error: --description is required")
		flag.Usage()
		os.Exit(ExitError)
	}

	var params []Param
	if paramsJSON != "" {
		if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing --params JSON: %v\n", err)
			os.Exit(ExitError)
		}
	}

	if module == "" {
		module = "github.com/hegner123/" + name
	}
	if outputDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting home dir: %v\n", err)
			os.Exit(ExitError)
		}
		outputDir = filepath.Join(home, "Documents", "Code", "terse-mcp", name)
	}

	config := GenerateConfig{
		Name:        name,
		Description: description,
		OutputDir:   outputDir,
		Module:      module,
		Params:      params,
	}

	result, err := generate(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitError)
	}

	output, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling result: %v\n", err)
		os.Exit(ExitError)
	}
	fmt.Println(string(output))
}

// ---------- MCP Server ----------

func runMCPServer() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()
	}()

	scanner := bufio.NewScanner(os.Stdin)
	lineChan := make(chan string)
	errChan := make(chan error, 1)

	go func() {
		for scanner.Scan() {
			lineChan <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			errChan <- err
		}
		close(lineChan)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errChan:
			fmt.Fprintf(os.Stderr, "Scanner error: %v\n", err)
			return
		case line, ok := <-lineChan:
			if !ok {
				return
			}
			if line == "" {
				continue
			}
			var req JSONRPCRequest
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				sendError(nil, -32700, "Parse error")
				continue
			}
			handleRequest(req)
		}
	}
}

func handleRequest(req JSONRPCRequest) {
	isNotification := req.ID == nil

	switch req.Method {
	case "initialize":
		handleInitialize(req)
	case "notifications/initialized":
		return
	case "tools/list":
		handleToolsList(req)
	case "tools/call":
		handleToolsCall(req)
	default:
		if isNotification {
			return
		}
		sendError(req.ID, -32601, "Method not found")
	}
}

func handleInitialize(req JSONRPCRequest) {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		ServerInfo: ServerInfo{
			Name:    "mcpgen",
			Version: "1.0.0",
		},
		Capabilities: Capabilities{
			Tools: map[string]bool{"list": true, "call": true},
		},
	}
	sendResponse(req.ID, result)
}

func handleToolsList(req JSONRPCRequest) {
	result := ToolsListResult{
		Tools: []Tool{
			{
				Name:        "mcpgen",
				Description: "Generate boilerplate for a new MCP tool (Go, stdio transport). Creates main.go with full MCP JSON-RPC server, CLI mode, go.mod, justfile, .mcp.json, .gitignore, and .claude/settings.local.json.",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"name": {
							Type:        "string",
							Description: "Tool name (used for binary, module, MCP server name).",
						},
						"description": {
							Type:        "string",
							Description: "One-line description of what the tool does.",
						},
						"output_dir": {
							Type:        "string",
							Description: "Directory to create the project in. Defaults to $HOME/Documents/Code/terse-mcp/<name>.",
						},
						"module": {
							Type:        "string",
							Description: "Go module path. Defaults to github.com/hegner123/<name>.",
						},
						"params": {
							Type:        "array",
							Description: `Array of parameter objects for the generated tool. Each object: {"name": "param_name", "type": "string|boolean|integer|number|array|object", "description": "...", "required": true/false, "default": <value>}.`,
						},
					},
					Required: []string{"name", "description", "params"},
				},
			},
		},
	}
	sendResponse(req.ID, result)
}

func handleToolsCall(req JSONRPCRequest) {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendError(req.ID, -32602, "Invalid params")
		return
	}

	if params.Name != "mcpgen" {
		sendError(req.ID, -32602, "Unknown tool")
		return
	}

	name, ok := params.Arguments["name"].(string)
	if !ok || name == "" {
		sendError(req.ID, -32602, "Missing or invalid 'name' parameter")
		return
	}

	description, ok := params.Arguments["description"].(string)
	if !ok || description == "" {
		sendError(req.ID, -32602, "Missing or invalid 'description' parameter")
		return
	}

	module, _ := params.Arguments["module"].(string)
	if module == "" {
		module = "github.com/hegner123/" + name
	}

	outputDir, _ := params.Arguments["output_dir"].(string)
	if outputDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			sendError(req.ID, -32603, fmt.Sprintf("Cannot determine home directory: %v", err))
			return
		}
		outputDir = filepath.Join(home, "Documents", "Code", "terse-mcp", name)
	}

	var toolParams []Param
	if rawParams, exists := params.Arguments["params"]; exists {
		paramsBytes, err := json.Marshal(rawParams)
		if err != nil {
			sendError(req.ID, -32602, fmt.Sprintf("Invalid 'params': %v", err))
			return
		}
		if err := json.Unmarshal(paramsBytes, &toolParams); err != nil {
			sendError(req.ID, -32602, fmt.Sprintf("Invalid 'params' structure: %v", err))
			return
		}
	}

	config := GenerateConfig{
		Name:        name,
		Description: description,
		OutputDir:   outputDir,
		Module:      module,
		Params:      toolParams,
	}

	result, err := generate(config)
	if err != nil {
		sendError(req.ID, -32603, fmt.Sprintf("Generation failed: %v", err))
		return
	}

	jsonResult, err := json.Marshal(result)
	if err != nil {
		sendError(req.ID, -32603, "Failed to marshal result")
		return
	}

	callResult := ToolCallResult{
		Content: []ContentItem{
			{Type: "text", Text: string(jsonResult)},
		},
	}
	sendResponse(req.ID, callResult)
}

func sendResponse(id any, result any) {
	resp := JSONRPCResponse{JSONRPC: "2.0", ID: id, Result: result}
	data, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal response: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func sendError(id any, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &Error{Code: code, Message: message},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal error: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

// ---------- Generator ----------

func generate(config GenerateConfig) (*GenerateResult, error) {
	if err := os.MkdirAll(filepath.Join(config.OutputDir, ".claude"), 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	mainPath := filepath.Join(config.OutputDir, "main.go")
	if _, err := os.Stat(mainPath); err == nil {
		return nil, fmt.Errorf("main.go already exists in %s — refusing to overwrite", config.OutputDir)
	}

	files := map[string]string{
		"main.go":                     renderMainGo(config),
		"go.mod":                      renderGoMod(config),
		"justfile":                    renderJustfile(config),
		".mcp.json":                   renderMCPJSON(config),
		".gitignore":                  renderGitignore(config),
		".claude/settings.local.json": renderClaudeSettings(config),
	}

	var created []string
	for name, content := range files {
		path := filepath.Join(config.OutputDir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
		created = append(created, name)
	}

	return &GenerateResult{
		OutputDir:    config.OutputDir,
		FilesCreated: created,
	}, nil
}

// ---------- Template helpers ----------

func titleCase(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

type templateData struct {
	Name          string
	Description   string
	Module        string
	Params        []Param
	RequiredNames []string
}

func newTemplateData(config GenerateConfig) templateData {
	var required []string
	for _, p := range config.Params {
		if p.Required {
			required = append(required, p.Name)
		}
	}
	return templateData{
		Name:          config.Name,
		Description:   config.Description,
		Module:        config.Module,
		Params:        config.Params,
		RequiredNames: required,
	}
}

var funcMap = template.FuncMap{
	"goType": func(jsonType string) string {
		switch jsonType {
		case "boolean":
			return "bool"
		case "integer":
			return "int"
		case "number":
			return "float64"
		case "array":
			return "[]any"
		case "object":
			return "map[string]any"
		default:
			return "string"
		}
	},
	"title": titleCase,
	"jsonDefault": func(v any) string {
		if v == nil {
			return ""
		}
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	},
	"quote": func(s string) string {
		b, _ := json.Marshal(s)
		return string(b)
	},
	"goZero": func(jsonType string) string {
		switch jsonType {
		case "boolean":
			return "false"
		case "integer":
			return "0"
		case "number":
			return "0.0"
		default:
			return `""`
		}
	},
}

// ---------- File renderers ----------

func renderMainGo(config GenerateConfig) string {
	data := newTemplateData(config)
	tmpl := template.Must(template.New("main.go").Delims("<%", "%>").Funcs(funcMap).Parse(mainGoTemplate))
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("template execution failed: %v", err))
	}
	return buf.String()
}

func renderGoMod(config GenerateConfig) string {
	return fmt.Sprintf("module %s\n\ngo 1.23\n", config.Module)
}

func renderJustfile(config GenerateConfig) string {
	return fmt.Sprintf(`binary := "%s"
install_path := "/usr/local/bin"

build:
    go build -o {{binary}}
    codesign -s - {{binary}}

install: build
    sudo cp {{binary}} {{install_path}}/

clean:
    rm -f {{binary}}

test:
    go test -v
`, config.Name)
}

func renderMCPJSON(config GenerateConfig) string {
	return fmt.Sprintf(`{
  "mcpServers": {
    %q: {
      "command": %q
    }
  }
}
`, config.Name, config.Name)
}

func renderGitignore(config GenerateConfig) string {
	return fmt.Sprintf(`# Binaries
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out
coverage.out
go.work

# Built binary
%s

# IDE
.vscode/
.idea/
*.swp
*.swo
*~

# OS
.DS_Store
Thumbs.db

# Claude Code
.claude/
`, config.Name)
}

func renderClaudeSettings(config GenerateConfig) string {
	return fmt.Sprintf(`{
  "permissions": {
    "allow": [
      "Bash(go build:*)",
      "Bash(go test:*)",
      "Bash(go vet:*)",
      "Bash(go fmt:*)",
      "Bash(gofmt:*)",
      "Bash(./%s --cli:*)",
      "Bash(git init:*)",
      "Bash(git add:*)",
      "Bash(git commit:*)"
    ]
  },
  "enableAllProjectMcpServers": true,
  "enabledMcpjsonServers": [
    %q
  ]
}
`, config.Name, config.Name)
}
