package main

import (
	"fmt"
	"strings"

	"github.com/glorko/crux/internal/api"
	"github.com/glorko/crux/internal/terminal"
)

// weztermTabController implements api.TabController using WeztermLauncher.
// MCP calls crux API; crux uses this - MCP never touches wezterm.
type weztermTabController struct {
	launcher *terminal.WeztermLauncher
}

func newWeztermTabController(launcher *terminal.WeztermLauncher) *weztermTabController {
	return &weztermTabController{launcher: launcher}
}

func (c *weztermTabController) ListTabs() ([]api.TabInfo, error) {
	panes, err := c.launcher.ListPanesWithTitles()
	if err != nil {
		return nil, err
	}
	tabs := make([]api.TabInfo, 0, len(panes))
	for _, p := range panes {
		tabs = append(tabs, api.TabInfo{
			Name:    p.Title,
			PaneID:  p.PaneID,
			LogDir:  p.LogDir,
			LogPath: p.LogPath,
		})
	}
	return tabs, nil
}

func (c *weztermTabController) Send(service string, text string) error {
	paneID := c.resolvePane(service)
	if paneID == "" {
		return fmt.Errorf("service %q not found", service)
	}
	return c.launcher.SendTextToPane(paneID, text)
}

func (c *weztermTabController) GetLogs(service string, lines int) (string, error) {
	paneID := c.resolvePane(service)
	if paneID == "" {
		return "", fmt.Errorf("service %q not found", service)
	}
	return c.launcher.GetPaneScrollback(paneID, lines)
}

func (c *weztermTabController) Focus(service string) error {
	paneID := c.resolvePane(service)
	if paneID == "" {
		return fmt.Errorf("service %q not found", service)
	}
	if err := c.launcher.FocusPane(paneID); err != nil {
		return err
	}
	return c.launcher.ActivateWindow()
}

func (c *weztermTabController) SpawnTab(title, workDir, command string, args []string) error {
	_, err := c.launcher.SpawnTab(title, workDir, command, args)
	return err
}

func (c *weztermTabController) resolvePane(service string) string {
	// Try stored map first (fast path)
	if id := c.launcher.GetServicePane(service); id != "" {
		return id
	}
	// Fallback: refresh from wezterm (handles start-one, external changes)
	panes, err := c.launcher.ListPanesWithTitles()
	if err != nil {
		return ""
	}
	serviceLower := strings.ToLower(service)
	for _, p := range panes {
		if strings.EqualFold(p.Title, service) || strings.Contains(strings.ToLower(p.Title), serviceLower) {
			return p.PaneID
		}
	}
	return ""
}
