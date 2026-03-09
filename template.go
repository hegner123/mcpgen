package main

// mainGoTemplate uses <% %> delimiters to avoid conflicts with Go's {{ }} and [] syntax.
const mainGoTemplate = `package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// Exit codes for CLI mode
const (
	ExitSuccess   = 0
	ExitError     = 1
	ExitNoChanges = 2
)

// Result is the output of the tool's core logic.
// TODO: Replace with your actual result type.
type Result struct {
	Summary string ` + "`" + `json:"summary"` + "`" + `
}

// ---------- MCP JSON-RPC types ----------

type JSONRPCRequest struct {
	JSONRPC string          ` + "`" + `json:"jsonrpc"` + "`" + `
	ID      any             ` + "`" + `json:"id"` + "`" + `
	Method  string          ` + "`" + `json:"method"` + "`" + `
	Params  json.RawMessage ` + "`" + `json:"params,omitempty"` + "`" + `
}

type JSONRPCResponse struct {
	JSONRPC string ` + "`" + `json:"jsonrpc"` + "`" + `
	ID      any    ` + "`" + `json:"id"` + "`" + `
	Result  any    ` + "`" + `json:"result,omitempty"` + "`" + `
	Error   *Error ` + "`" + `json:"error,omitempty"` + "`" + `
}

type Error struct {
	Code    int    ` + "`" + `json:"code"` + "`" + `
	Message string ` + "`" + `json:"message"` + "`" + `
}

type InitializeResult struct {
	ProtocolVersion string       ` + "`" + `json:"protocolVersion"` + "`" + `
	ServerInfo      ServerInfo   ` + "`" + `json:"serverInfo"` + "`" + `
	Capabilities    Capabilities ` + "`" + `json:"capabilities"` + "`" + `
}

type ServerInfo struct {
	Name    string ` + "`" + `json:"name"` + "`" + `
	Version string ` + "`" + `json:"version"` + "`" + `
}

type Capabilities struct {
	Tools map[string]bool ` + "`" + `json:"tools"` + "`" + `
}

type ToolsListResult struct {
	Tools []Tool ` + "`" + `json:"tools"` + "`" + `
}

type Tool struct {
	Name        string      ` + "`" + `json:"name"` + "`" + `
	Description string      ` + "`" + `json:"description"` + "`" + `
	InputSchema InputSchema ` + "`" + `json:"inputSchema"` + "`" + `
}

type InputSchema struct {
	Type       string              ` + "`" + `json:"type"` + "`" + `
	Properties map[string]Property ` + "`" + `json:"properties"` + "`" + `
	Required   []string            ` + "`" + `json:"required"` + "`" + `
}

type Property struct {
	Type        string ` + "`" + `json:"type"` + "`" + `
	Description string ` + "`" + `json:"description"` + "`" + `
	Default     any    ` + "`" + `json:"default,omitempty"` + "`" + `
}

type ToolCallParams struct {
	Name      string         ` + "`" + `json:"name"` + "`" + `
	Arguments map[string]any ` + "`" + `json:"arguments"` + "`" + `
}

type ToolCallResult struct {
	Content []ContentItem ` + "`" + `json:"content"` + "`" + `
}

type ContentItem struct {
	Type string ` + "`" + `json:"type"` + "`" + `
	Text string ` + "`" + `json:"text"` + "`" + `
}

// ---------- main ----------

func main() {
	cliMode := flag.Bool("cli", false, "Run in CLI mode (default is MCP server mode)")
<%- range .Params%>
<%- if eq .Type "boolean"%>
	flag<% title .Name%> := flag.Bool(<% quote .Name%>, false, <% quote .Description%>)
<%- else if eq .Type "integer"%>
	flag<% title .Name%> := flag.Int(<% quote .Name%>, 0, <% quote .Description%>)
<%- else%>
	flag<% title .Name%> := flag.String(<% quote .Name%>, "", <% quote .Description%>)
<%- end%>
<%- end%>
	flag.Parse()

	if *cliMode {
		runCLI(<%- range $i, $p := .Params%><% if $i%>, <% end%>*flag<% title $p.Name%><% end%>)
		return
	}

	runMCPServer()
}

func runCLI(<%- range $i, $p := .Params%><% if $i%>, <% end%><% $p.Name%> <% goType $p.Type%><% end%>) {
<%- range .Params%><% if .Required%>
	if <% .Name%> == <% goZero .Type%> {
		fmt.Fprintln(os.Stderr, "Error: --<% .Name%> is required")
		flag.Usage()
		os.Exit(ExitError)
	}
<%- end%><% end%>

	result, err := execute(<%- range $i, $p := .Params%><% if $i%>, <% end%><% $p.Name%><% end%>)
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
			Name:    <% quote .Name%>,
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
				Name:        <% quote .Name%>,
				Description: <% quote .Description%>,
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
<%- range .Params%>
						<% quote .Name%>: {
							Type:        <% quote .Type%>,
							Description: <% quote .Description%>,
<%- if .Default%>
							Default:     <% jsonDefault .Default%>,
<%- end%>
						},
<%- end%>
					},
					Required: []string{<%- range $i, $n := .RequiredNames%><% if $i%>, <% end%><% quote $n%><% end -%>},
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

	if params.Name != <% quote .Name%> {
		sendError(req.ID, -32602, "Unknown tool")
		return
	}

	// Extract parameters from arguments
<%- range .Params%>
<%- if eq .Type "boolean"%>
	arg<% title .Name%>, _ := params.Arguments[<% quote .Name%>].(bool)
<%- else if eq .Type "integer"%>
	raw<% title .Name%>, _ := params.Arguments[<% quote .Name%>].(float64)
	arg<% title .Name%> := int(raw<% title .Name%>)
<%- else%>
	arg<% title .Name%>, _ := params.Arguments[<% quote .Name%>].(string)
<%- end%>
<%- end%>

	// Validate required parameters
<%- range .Params%><% if .Required%>
<%- if eq .Type "string"%>
	if arg<% title .Name%> == "" {
		sendError(req.ID, -32602, "Missing or invalid '<% .Name%>' parameter")
		return
	}
<%- end%>
<%- end%><% end%>

	result, err := execute(<%- range $i, $p := .Params%><% if $i%>, <% end%>arg<% title $p.Name%><% end%>)
	if err != nil {
		sendError(req.ID, -32603, fmt.Sprintf("Execution failed: %v", err))
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

// ---------- Core logic ----------

// execute is the tool's core logic. Implement your tool here.
// TODO: Implement the actual tool logic.
func execute(<%- range $i, $p := .Params%><% if $i%>, <% end%><% $p.Name%> <% goType $p.Type%><% end%>) (*Result, error) {
	return &Result{
		Summary: "TODO: implement <% .Name%>",
	}, nil
}
`
