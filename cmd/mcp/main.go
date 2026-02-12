package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// MCP Server for Crux - controls services via Crux API only.
// MCP has no knowledge of wezterm or terminal - all control goes through crux.

const defaultAPIURL = "http://localhost:9876"

// JSON-RPC types
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type ToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			sendError(nil, -32700, "Parse error")
			continue
		}

		handleRequest(req)
	}
}

func getAPIURL() string {
	if u := os.Getenv("CRUX_API_URL"); u != "" {
		return u
	}
	return defaultAPIURL
}

func handleRequest(req Request) {
	switch req.Method {
	case "initialize":
		result := InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities:    ServerCapabilities{Tools: &ToolsCapability{}},
			ServerInfo:      ServerInfo{Name: "crux-mcp", Version: "0.4.0"},
		}
		sendResult(req.ID, result)

	case "notifications/initialized":
		// No response

	case "tools/list":
		tools := []Tool{
			{
				Name:        "crux_status",
				Description: "List running service tabs. Requires crux to be running.",
				InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
			},
			{
				Name:        "crux_send",
				Description: "Send text to a tab (e.g. 'r'=reload, 'R'=restart, 'q'=quit)",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"tab":  {Type: "string", Description: "Service name or tab number"},
						"text": {Type: "string", Description: "Text to send"},
					},
					Required: []string{"tab", "text"},
				},
			},
			{
				Name:        "crux_logs",
				Description: "Get terminal scrollback from a tab",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"tab":   {Type: "string", Description: "Service name or tab number"},
						"lines": {Type: "string", Description: "Number of lines (default 50)"},
					},
					Required: []string{"tab"},
				},
			},
			{
				Name:        "crux_focus",
				Description: "Focus/activate a tab",
				InputSchema: InputSchema{
					Type:       "object",
					Properties: map[string]Property{"tab": {Type: "string", Description: "Service name or tab number"}},
					Required:   []string{"tab"},
				},
			},
			{
				Name:        "crux_logfile",
				Description: "Read log files for crashed/closed tabs. Logs at /tmp/crux-logs/<service>/",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"service": {Type: "string", Description: "Service name or 'list' for all"},
						"run":     {Type: "string", Description: "'latest', 'list', or timestamp"},
						"lines":   {Type: "string", Description: "Lines to read (default 100)"},
					},
					Required: []string{"service"},
				},
			},
		}
		sendResult(req.ID, ToolsListResult{Tools: tools})

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			sendError(req.ID, -32602, "Invalid params")
			return
		}
		handleToolCall(req.ID, params)

	default:
		sendError(req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func handleToolCall(id interface{}, params CallToolParams) {
	var result string
	var isError bool

	args := params.Arguments
	if args == nil {
		args = make(map[string]interface{})
	}

	switch params.Name {
	case "crux_status":
		result, isError = apiGetTabs()
	case "crux_send":
		tab, _ := args["tab"].(string)
		text, _ := args["text"].(string)
		result, isError = apiSend(tab, text)
	case "crux_logs":
		tab, _ := args["tab"].(string)
		lines, _ := args["lines"].(string)
		result, isError = apiLogs(tab, lines)
	case "crux_focus":
		tab, _ := args["tab"].(string)
		result, isError = apiFocus(tab)
	case "crux_logfile":
		service, _ := args["service"].(string)
		run, _ := args["run"].(string)
		lines, _ := args["lines"].(string)
		result, isError = apiLogfile(service, run, lines)
	default:
		result = "Unknown tool: " + params.Name
		isError = true
	}

	sendResult(id, ToolResult{
		Content: []ContentItem{{Type: "text", Text: result}},
		IsError: isError,
	})
}

func apiRequest(method, path string, body []byte) (*http.Response, error) {
	url := getAPIURL() + path
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return http.DefaultClient.Do(req)
}

func apiGet(path string) (string, error) {
	resp, err := apiRequest(http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return string(data), nil
}

func apiPost(path string, body interface{}) (string, error) {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	resp, err := apiRequest(http.MethodPost, path, b)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("API %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return string(data), nil
}

func apiGetTabs() (string, bool) {
	data, err := apiGet("/tabs")
	if err != nil {
		return "Crux API not available. Is crux running? " + err.Error(), true
	}
	var out struct {
		Tabs   []struct {
			Name    string `json:"name"`
			LogPath string `json:"log_path"`
		} `json:"tabs"`
		Uptime string `json:"uptime"`
	}
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		return "Failed to parse API response: " + data, true
	}
	if len(out.Tabs) == 0 {
		return "No tabs. Run 'crux' to start services.", false
	}
	var b strings.Builder
	b.WriteString("Crux Tabs\n")
	b.WriteString("=========\n\n")
	for i, t := range out.Tabs {
		b.WriteString(fmt.Sprintf("Tab %d: %s\n", i+1, t.Name))
		b.WriteString(fmt.Sprintf("  Log: %s\n\n", t.LogPath))
	}
	b.WriteString("Commands: r=reload, R=restart, q=quit\n")
	return b.String(), false
}

func resolveTabRef(tabRef string) (string, error) {
	// Try numeric (1-based tab number)
	if idx, err := strconv.Atoi(tabRef); err == nil && idx >= 1 {
		data, err := apiGet("/tabs")
		if err != nil {
			return "", err
		}
		var out struct {
			Tabs []struct{ Name string } `json:"tabs"`
		}
		if err := json.Unmarshal([]byte(data), &out); err != nil {
			return "", err
		}
		if idx <= len(out.Tabs) {
			return out.Tabs[idx-1].Name, nil
		}
	}
	return tabRef, nil // use as service name
}

func apiSend(tab, text string) (string, bool) {
	service, err := resolveTabRef(tab)
	if err != nil {
		return "Failed: " + err.Error(), true
	}
	_, err = apiPost("/send/"+service, map[string]string{"text": text})
	if err != nil {
		return "Failed: " + err.Error(), true
	}
	return fmt.Sprintf("Sent '%s' to %s", text, service), false
}

func apiLogs(tab, lines string) (string, bool) {
	service, err := resolveTabRef(tab)
	if err != nil {
		return "Failed: " + err.Error(), true
	}
	path := "/logs/" + service
	if lines != "" {
		path += "?lines=" + lines
	}
	data, err := apiGet(path)
	if err != nil {
		return "Failed: " + err.Error(), true
	}
	return fmt.Sprintf("=== %s (scrollback) ===\n\n%s", service, data), false
}

func apiFocus(tab string) (string, bool) {
	service, err := resolveTabRef(tab)
	if err != nil {
		return "Failed: " + err.Error(), true
	}
	_, err = apiPost("/focus/"+service, nil)
	if err != nil {
		return "Failed: " + err.Error(), true
	}
	return "Focused " + service, false
}

func apiLogfile(service, run, lines string) (string, bool) {
	if run == "" {
		run = "latest"
	}
	path := "/logfile/" + service + "?run=" + run
	if lines != "" {
		path += "&lines=" + lines
	}
	data, err := apiGet(path)
	if err != nil {
		return "Failed: " + err.Error(), true
	}
	if service == "list" || service == "" {
		return data, false
	}
	return fmt.Sprintf("=== %s / %s ===\n\n%s", service, run, data), false
}

func sendResult(id interface{}, result interface{}) {
	resp := Response{JSONRPC: "2.0", ID: id, Result: result}
	output, _ := json.Marshal(resp)
	fmt.Println(string(output))
}

func sendError(id interface{}, code int, message string) {
	resp := Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: message}}
	output, _ := json.Marshal(resp)
	fmt.Println(string(output))
}
