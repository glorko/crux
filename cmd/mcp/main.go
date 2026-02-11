package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// MCP Server for Crux - controls services via Wezterm CLI
// Protocol: JSON-RPC 2.0 over stdio
// Uses wezterm cli commands directly - no pipes needed!

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

// MCP types
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

// Pane represents a Wezterm pane (terminal tab)
type Pane struct {
	WindowID  int
	TabID     int
	PaneID    int
	Workspace string
	Size      string
	Title     string
	CWD       string
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

func handleRequest(req Request) {
	switch req.Method {
	case "initialize":
		result := InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: ServerCapabilities{
				Tools: &ToolsCapability{},
			},
			ServerInfo: ServerInfo{
				Name:    "crux-mcp",
				Version: "0.3.0",
			},
		}
		sendResult(req.ID, result)

	case "notifications/initialized":
		// No response needed

	case "tools/list":
		tools := []Tool{
			{
				Name:        "crux_status",
				Description: "List all running terminal tabs with their numbers and titles.",
				InputSchema: InputSchema{
					Type:       "object",
					Properties: map[string]Property{},
				},
			},
			{
				Name:        "crux_send",
				Description: "Send text/command to a terminal tab. For Flutter: 'r' = hot reload, 'R' = hot restart, 'q' = quit.",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"tab": {
							Type:        "string",
							Description: "Tab number (1, 2, 3...) or partial title match (e.g., 'backend', 'flutter')",
						},
						"text": {
							Type:        "string",
							Description: "Text to send (e.g., 'r' for reload, 'R' for restart, 'q' for quit)",
						},
					},
					Required: []string{"tab", "text"},
				},
			},
			{
				Name:        "crux_logs",
				Description: "Get terminal output (scrollback) from a tab. Returns the last N lines.",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"tab": {
							Type:        "string",
							Description: "Tab number (1, 2, 3...) or partial title match",
						},
						"lines": {
							Type:        "string",
							Description: "Number of lines to get from scrollback (default: 50)",
						},
					},
					Required: []string{"tab"},
				},
			},
			{
				Name:        "crux_focus",
				Description: "Focus/activate a specific terminal tab",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"tab": {
							Type:        "string",
							Description: "Tab number (1, 2, 3...) or partial title match",
						},
					},
					Required: []string{"tab"},
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

	switch params.Name {
	case "crux_status":
		result, isError = getStatus()

	case "crux_send":
		tab, _ := params.Arguments["tab"].(string)
		text, _ := params.Arguments["text"].(string)
		result, isError = sendToTab(tab, text)

	case "crux_logs":
		tab, _ := params.Arguments["tab"].(string)
		lines, _ := params.Arguments["lines"].(string)
		result, isError = getTabLogs(tab, lines)

	case "crux_focus":
		tab, _ := params.Arguments["tab"].(string)
		result, isError = focusTab(tab)

	default:
		result = fmt.Sprintf("Unknown tool: %s", params.Name)
		isError = true
	}

	sendResult(id, ToolResult{
		Content: []ContentItem{{Type: "text", Text: result}},
		IsError: isError,
	})
}

// listPanes returns all wezterm panes
func listPanes() ([]Pane, error) {
	cmd := exec.Command("wezterm", "cli", "list")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("wezterm not running or not available: %v", err)
	}

	var panes []Pane
	lines := strings.Split(string(output), "\n")

	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // skip header
		}

		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		winID, _ := strconv.Atoi(fields[0])
		tabID, _ := strconv.Atoi(fields[1])
		paneID, _ := strconv.Atoi(fields[2])

		// Title might have spaces, get everything after size
		title := ""
		cwd := ""
		if len(fields) >= 6 {
			title = fields[5]
		}
		if len(fields) >= 7 {
			cwd = fields[6]
		}

		panes = append(panes, Pane{
			WindowID:  winID,
			TabID:     tabID,
			PaneID:    paneID,
			Workspace: fields[3],
			Size:      fields[4],
			Title:     title,
			CWD:       cwd,
		})
	}

	return panes, nil
}

// findTab finds a pane by 1-based tab number or title match
// User provides 1, 2, 3 but internally we use 0-based pane IDs
func findTab(tabRef string) (*Pane, int, error) {
	panes, err := listPanes()
	if err != nil {
		return nil, 0, err
	}

	if len(panes) == 0 {
		return nil, 0, fmt.Errorf("no wezterm tabs found")
	}

	// Try numeric tab number first (1-based input -> convert to index)
	if tabNum, err := strconv.Atoi(tabRef); err == nil && tabNum >= 1 {
		idx := tabNum - 1 // Convert to 0-based index
		if idx < len(panes) {
			return &panes[idx], tabNum, nil
		}
	}

	// Try title match (case-insensitive, partial)
	tabRefLower := strings.ToLower(tabRef)
	for i, p := range panes {
		if strings.Contains(strings.ToLower(p.Title), tabRefLower) {
			return &p, i + 1, nil // Return 1-based tab number
		}
	}

	// List available tabs
	var names []string
	for i, p := range panes {
		names = append(names, fmt.Sprintf("%d:%s", i+1, p.Title))
	}
	return nil, 0, fmt.Errorf("tab '%s' not found. Available: %v", tabRef, names)
}

func getStatus() (string, bool) {
	panes, err := listPanes()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), true
	}

	if len(panes) == 0 {
		return "No wezterm tabs found. Is wezterm running?", true
	}

	var result strings.Builder
	result.WriteString("Wezterm Tabs\n")
	result.WriteString("============\n\n")

	for i, p := range panes {
		tabNum := i + 1 // 1-based for display
		result.WriteString(fmt.Sprintf("Tab %d: %s\n", tabNum, p.Title))
		result.WriteString(fmt.Sprintf("  Size: %s\n", p.Size))
		if p.CWD != "" {
			result.WriteString(fmt.Sprintf("  CWD: %s\n", p.CWD))
		}
		result.WriteString("\n")
	}

	result.WriteString("Commands:\n")
	result.WriteString("  r = hot reload (Flutter)\n")
	result.WriteString("  R = hot restart (Flutter)\n")
	result.WriteString("  q = quit\n")

	return result.String(), false
}

func sendToTab(tabRef string, text string) (string, bool) {
	pane, tabNum, err := findTab(tabRef)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), true
	}

	cmd := exec.Command("wezterm", "cli", "send-text",
		"--pane-id", strconv.Itoa(pane.PaneID),
		"--no-paste",
		text)

	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("Failed to send to tab %d: %v", tabNum, err), true
	}

	return fmt.Sprintf("Sent '%s' to tab %d (%s)", text, tabNum, pane.Title), false
}

func getTabLogs(tabRef string, lines string) (string, bool) {
	pane, tabNum, err := findTab(tabRef)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), true
	}

	numLines := 50
	if n, err := strconv.Atoi(lines); err == nil && n > 0 {
		numLines = n
	}
	if numLines > 1000 {
		numLines = 1000
	}

	// Get scrollback - negative start-line means scrollback
	cmd := exec.Command("wezterm", "cli", "get-text",
		"--pane-id", strconv.Itoa(pane.PaneID),
		"--start-line", strconv.Itoa(-numLines))

	output, err := cmd.Output()
	if err != nil {
		return fmt.Sprintf("Failed to get text from tab %d: %v", tabNum, err), true
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("=== Tab %d: %s (last %d lines) ===\n\n", tabNum, pane.Title, numLines))
	result.Write(output)

	return result.String(), false
}

func focusTab(tabRef string) (string, bool) {
	pane, tabNum, err := findTab(tabRef)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), true
	}

	cmd := exec.Command("wezterm", "cli", "activate-pane",
		"--pane-id", strconv.Itoa(pane.PaneID))

	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("Failed to focus tab %d: %v", tabNum, err), true
	}

	// Also activate the wezterm window
	exec.Command("osascript", "-e", `tell application "WezTerm" to activate`).Run()

	return fmt.Sprintf("Focused tab %d (%s)", tabNum, pane.Title), false
}

func sendResult(id interface{}, result interface{}) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	output, _ := json.Marshal(resp)
	fmt.Println(string(output))
}

func sendError(id interface{}, code int, message string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
	output, _ := json.Marshal(resp)
	fmt.Println(string(output))
}
