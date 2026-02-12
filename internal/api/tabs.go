package api

// TabInfo represents a service tab in the crux session
type TabInfo struct {
	Name    string `json:"name"`
	PaneID  string `json:"pane_id,omitempty"`
	LogDir  string `json:"log_dir"`
	LogPath string `json:"log_path"` // latest.log path
}

// TabController provides tab control for terminal-based sessions (e.g. Wezterm).
// Crux implements this; MCP calls the API which uses this - no terminal knowledge in MCP.
type TabController interface {
	// ListTabs returns current tabs with name, log path
	ListTabs() ([]TabInfo, error)
	// Send sends text to a tab (e.g. "r" for reload)
	Send(service string, text string) error
	// GetLogs returns scrollback from a tab
	GetLogs(service string, lines int) (string, error)
	// Focus activates a tab
	Focus(service string) error
}
