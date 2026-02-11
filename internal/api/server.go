package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	port      int
	workers   []Worker
	startTime time.Time
	mu        sync.RWMutex
	server    *http.Server
	onShutdown func() // callback when shutdown is requested
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
