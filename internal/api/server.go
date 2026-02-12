package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// WorkerInfo represents a worker's status
type WorkerInfo struct {
	Name     string `json:"name"`
	PID      int    `json:"pid"`
	Alive    bool   `json:"alive"`
	PipePath string `json:"pipe_path"`
}

// StatusResponse is the response for GET /status
type StatusResponse struct {
	Orchestrator string       `json:"orchestrator"`
	Uptime       string       `json:"uptime"`
	Workers      []WorkerInfo `json:"workers"`
}

// CommandResponse is the response for action endpoints
type CommandResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Service string `json:"service,omitempty"`
}

// Worker interface for the API to interact with workers
type Worker interface {
	GetName() string
	GetPID() int
	GetPipePath() string
	SendCommand(cmd string) error
}

// SimpleWorker is a basic implementation of Worker
type SimpleWorker struct {
	Name     string
	PID      int
	PipePath string
}

func (w *SimpleWorker) GetName() string     { return w.Name }
func (w *SimpleWorker) GetPID() int         { return w.PID }
func (w *SimpleWorker) GetPipePath() string { return w.PipePath }
func (w *SimpleWorker) SendCommand(cmd string) error {
	return SendCommandToPipe(w.PipePath, cmd)
}

// Server is the HTTP API server for crux control
type Server struct {
	port         int
	workers      []Worker
	tabCtrl      TabController // for Wezterm mode - MCP uses this via API
	startTime    time.Time
	mu           sync.RWMutex
	server       *http.Server
	onShutdown   func() // callback when shutdown is requested
}

// NewServer creates a new API server
func NewServer(port int) *Server {
	return &Server{
		port:      port,
		workers:   make([]Worker, 0),
		startTime: time.Now(),
	}
}

// SetWorkers sets the list of workers to manage
func (s *Server) SetWorkers(workers []Worker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers = workers
}

// SetOnShutdown sets the callback for shutdown requests
func (s *Server) SetOnShutdown(fn func()) {
	s.onShutdown = fn
}

// SetTabController sets the tab controller for Wezterm mode (MCP uses API, not wezterm)
func (s *Server) SetTabController(tc TabController) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tabCtrl = tc
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Status endpoint
	mux.HandleFunc("/status", s.handleStatus)

	// Reload endpoints
	mux.HandleFunc("/reload", s.handleReloadAll)
	mux.HandleFunc("/reload/", s.handleReload)

	// Restart endpoints
	mux.HandleFunc("/restart", s.handleRestartAll)
	mux.HandleFunc("/restart/", s.handleRestart)

	// Stop endpoints
	mux.HandleFunc("/stop", s.handleStopAll)
	mux.HandleFunc("/stop/", s.handleStop)

	// Health check
	mux.HandleFunc("/health", s.handleHealth)

	// Tab endpoints (Wezterm mode - used by MCP)
	mux.HandleFunc("/tabs", s.handleTabs)
	mux.HandleFunc("/send/", s.handleSend)
	mux.HandleFunc("/logs/", s.handleLogs)
	mux.HandleFunc("/logfile/", s.handleLogfile)
	mux.HandleFunc("/focus/", s.handleFocus)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	fmt.Printf("🌐 API server starting on http://localhost:%d\n", s.port)
	return s.server.ListenAndServe()
}

// Stop stops the HTTP server
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// GetPort returns the server port
func (s *Server) GetPort() int {
	return s.port
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Tab-mode handlers (used by MCP - no wezterm knowledge in MCP)
func (s *Server) handleTabs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	tc := s.tabCtrl
	s.mu.RUnlock()
	if tc == nil {
		http.Error(w, "Tab controller not available (is crux running with wezterm?)", http.StatusServiceUnavailable)
		return
	}
	tabs, err := tc.ListTabs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tabs":   tabs,
		"uptime": time.Since(s.startTime).Round(time.Second).String(),
	})
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service := r.URL.Path[len("/send/"):]
	if service == "" {
		http.Error(w, "Service name required", http.StatusBadRequest)
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
		http.Error(w, "JSON body with 'text' required", http.StatusBadRequest)
		return
	}
	s.mu.RLock()
	tc := s.tabCtrl
	s.mu.RUnlock()
	if tc == nil {
		http.Error(w, "Tab controller not available", http.StatusServiceUnavailable)
		return
	}
	err := tc.Send(service, body.Text)
	resp := CommandResponse{Success: err == nil, Service: service, Message: "Sent"}
	if err != nil {
		resp.Message = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service := r.URL.Path[len("/logs/"):]
	if service == "" {
		http.Error(w, "Service name required", http.StatusBadRequest)
		return
	}
	lines := 50
	if n := r.URL.Query().Get("lines"); n != "" {
		if parsed, err := strconv.Atoi(n); err == nil && parsed > 0 {
			lines = parsed
			if lines > 1000 {
				lines = 1000
			}
		}
	}
	s.mu.RLock()
	tc := s.tabCtrl
	s.mu.RUnlock()
	if tc == nil {
		http.Error(w, "Tab controller not available", http.StatusServiceUnavailable)
		return
	}
	content, err := tc.GetLogs(service, lines)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(content))
}

func (s *Server) handleLogfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.Path[len("/logfile/"):]
	run := r.URL.Query().Get("run")
	lines := 100
	if n := r.URL.Query().Get("lines"); n != "" {
		if parsed, err := strconv.Atoi(n); err == nil && parsed > 0 {
			lines = parsed
		}
	}
	content, err := handleLogfilePath(path, run, lines)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(content))
}

func handleLogfilePath(service, run string, lines int) (string, error) {
	baseDir := "/tmp/crux-logs"
	if service == "list" || service == "" {
		return listLogServices(baseDir), nil
	}
	if run == "list" {
		return listLogRuns(baseDir, service), nil
	}
	if run == "" {
		run = "latest"
	}
	return readLogFileContent(baseDir, service, run, lines)
}

func listLogServices(baseDir string) string {
	entries, err := os.ReadDir(baseDir)
	if err != nil || len(entries) == 0 {
		return "No crux logs found. Run 'crux' to create logs.\nLocation: /tmp/crux-logs/<service>/"
	}
	var out strings.Builder
	out.WriteString("=== Crux Log History ===\n\n")
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		svcDir := baseDir + "/" + e.Name()
		logs, _ := filepath.Glob(svcDir + "/*.log")
		var count int
		for _, l := range logs {
			if filepath.Base(l) != "latest.log" {
				count++
			}
		}
		if count > 0 {
			info, _ := os.Stat(svcDir + "/latest.log")
			if info != nil {
				out.WriteString(fmt.Sprintf("  %s: %d runs, latest: %s\n", e.Name(), count, info.ModTime().Format("2006-01-02 15:04:05")))
			} else {
				out.WriteString(fmt.Sprintf("  %s: %d runs\n", e.Name(), count))
			}
		}
	}
	return out.String()
}

func listLogRuns(baseDir, service string) string {
	svcDir := baseDir + "/" + service
	logs, _ := filepath.Glob(svcDir + "/*.log")
	var realLogs []string
	for _, l := range logs {
		if filepath.Base(l) != "latest.log" {
			realLogs = append(realLogs, l)
		}
	}
	if len(realLogs) == 0 {
		return "No log files for " + service
	}
	var out strings.Builder
	out.WriteString(fmt.Sprintf("=== Runs for %s ===\n\n", service))
	for i := len(realLogs) - 1; i >= 0; i-- {
		info, _ := os.Stat(realLogs[i])
		name := filepath.Base(realLogs[i])
		ts := strings.TrimSuffix(name, ".log")
		if info != nil {
			out.WriteString(fmt.Sprintf("  %s (%.1f KB)\n", ts, float64(info.Size())/1024))
		} else {
			out.WriteString("  " + ts + "\n")
		}
	}
	return out.String()
}

func readLogFileContent(baseDir, service, run string, lines int) (string, error) {
	svcDir := baseDir + "/" + service
	var logPath string
	if run == "latest" {
		logPath = svcDir + "/latest.log"
	} else {
		logPath = svcDir + "/" + run + ".log"
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", err
	}
	allLines := strings.Split(string(data), "\n")
	start := len(allLines) - lines
	if start < 0 {
		start = 0
	}
	return strings.Join(allLines[start:], "\n"), nil
}

func (s *Server) handleFocus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service := r.URL.Path[len("/focus/"):]
	if service == "" {
		http.Error(w, "Service name required", http.StatusBadRequest)
		return
	}
	s.mu.RLock()
	tc := s.tabCtrl
	s.mu.RUnlock()
	if tc == nil {
		http.Error(w, "Tab controller not available", http.StatusServiceUnavailable)
		return
	}
	err := tc.Focus(service)
	resp := CommandResponse{Success: err == nil, Service: service, Message: "Focused"}
	if err != nil {
		resp.Message = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	workers := make([]WorkerInfo, 0, len(s.workers))
	for _, worker := range s.workers {
		workers = append(workers, WorkerInfo{
			Name:     worker.GetName(),
			PID:      worker.GetPID(),
			Alive:    isProcessAlive(worker.GetPID()),
			PipePath: worker.GetPipePath(),
		})
	}

	resp := StatusResponse{
		Orchestrator: "crux",
		Uptime:       time.Since(s.startTime).Round(time.Second).String(),
		Workers:      workers,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleReloadAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	errors := []string{}
	for _, worker := range s.workers {
		if err := worker.SendCommand("r"); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", worker.GetName(), err))
		}
	}

	resp := CommandResponse{
		Success: len(errors) == 0,
		Message: "Reload sent to all workers",
	}
	if len(errors) > 0 {
		resp.Message = fmt.Sprintf("Some errors: %v", errors)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	service := r.URL.Path[len("/reload/"):]
	if service == "" {
		http.Error(w, "Service name required", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	worker := s.findWorker(service)
	if worker == nil {
		http.Error(w, fmt.Sprintf("Service '%s' not found", service), http.StatusNotFound)
		return
	}

	err := worker.SendCommand("r")
	resp := CommandResponse{
		Success: err == nil,
		Service: service,
		Message: "Reload command sent",
	}
	if err != nil {
		resp.Message = fmt.Sprintf("Error: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleRestartAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	errors := []string{}
	for _, worker := range s.workers {
		if err := worker.SendCommand("R"); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", worker.GetName(), err))
		}
	}

	resp := CommandResponse{
		Success: len(errors) == 0,
		Message: "Restart sent to all workers",
	}
	if len(errors) > 0 {
		resp.Message = fmt.Sprintf("Some errors: %v", errors)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	service := r.URL.Path[len("/restart/"):]
	if service == "" {
		http.Error(w, "Service name required", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	worker := s.findWorker(service)
	if worker == nil {
		http.Error(w, fmt.Sprintf("Service '%s' not found", service), http.StatusNotFound)
		return
	}

	err := worker.SendCommand("R")
	resp := CommandResponse{
		Success: err == nil,
		Service: service,
		Message: "Restart command sent",
	}
	if err != nil {
		resp.Message = fmt.Sprintf("Error: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleStopAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := CommandResponse{
		Success: true,
		Message: "Shutdown initiated",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	// Trigger shutdown after response is sent
	if s.onShutdown != nil {
		go func() {
			time.Sleep(100 * time.Millisecond)
			s.onShutdown()
		}()
	}
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	service := r.URL.Path[len("/stop/"):]
	if service == "" {
		http.Error(w, "Service name required", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	worker := s.findWorker(service)
	if worker == nil {
		http.Error(w, fmt.Sprintf("Service '%s' not found", service), http.StatusNotFound)
		return
	}

	err := worker.SendCommand("q")
	resp := CommandResponse{
		Success: err == nil,
		Service: service,
		Message: "Stop command sent",
	}
	if err != nil {
		resp.Message = fmt.Sprintf("Error: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) findWorker(name string) Worker {
	for _, w := range s.workers {
		if w.GetName() == name {
			return w
		}
	}
	return nil
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}

// SendCommandToPipe sends a command to a named pipe with timeout
func SendCommandToPipe(pipePath string, cmd string) error {
	done := make(chan error, 1)
	go func() {
		pipe, err := os.OpenFile(pipePath, os.O_WRONLY, 0)
		if err != nil {
			done <- fmt.Errorf("failed to open pipe: %w", err)
			return
		}
		defer pipe.Close()

		_, err = pipe.WriteString(cmd + "\n")
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		return fmt.Errorf("timeout writing to pipe")
	}
}
