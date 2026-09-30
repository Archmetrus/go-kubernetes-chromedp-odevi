package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"k8s-chromedp-synthetics/pkg/automation"
	"k8s-chromedp-synthetics/pkg/runner"
	"k8s-chromedp-synthetics/pkg/security"
	"k8s-chromedp-synthetics/pkg/store"
	"k8s-chromedp-synthetics/pkg/types"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow UI connections
	},
}

type Server struct {
	Automation *automation.AutomationGateway
	Runner     *runner.ChromeRunner
	Storage    *store.StorageManager
	Security   *security.SecurityMiddleware
	clients    map[*websocket.Conn]bool
	broadcast  chan interface{}
	mu         sync.Mutex
}

func NewServer(ag *automation.AutomationGateway, r *runner.ChromeRunner, sm *store.StorageManager, sec *security.SecurityMiddleware) *Server {
	s := &Server{
		Automation: ag,
		Runner:     r,
		Storage:    sm,
		Security:   sec,
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan interface{}, 100),
	}
	go s.handleBroadcasts()
	return s
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// Protected Endpoints
	mux.Handle("/api/v1/automation/execute", s.Security.WrapMiddleware(http.HandlerFunc(s.handleRobomotionExecute), true))
	mux.Handle("/api/v1/capture", s.Security.WrapMiddleware(http.HandlerFunc(s.handleInstantCapture), true))
	mux.Handle("/api/v1/scrape", s.Security.WrapMiddleware(http.HandlerFunc(s.handleFullScrape), true))

	// Public / Monitoring Endpoints
	mux.Handle("/api/v1/healthz", s.Security.WrapMiddleware(http.HandlerFunc(s.handleHealthz), false))
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/ws/live", s.handleWebSocket)

	// Static UI & Files
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/", fs)
	mux.Handle("/data/", http.StripPrefix("/data/", http.FileServer(http.Dir("./data"))))
}

func (s *Server) handleRobomotionExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"Failed to read request body"}`, http.StatusBadRequest)
		return
	}

	var workflow types.AutomationWorkflow
	if err := json.Unmarshal(body, &workflow); err != nil {
		http.Error(w, `{"error":"Invalid Robomotion JSON workflow schema"}`, http.StatusBadRequest)
		return
	}

	s.broadcastEvent(map[string]interface{}{
		"type":    "WORKFLOW_STARTED",
		"id":      workflow.ID,
		"name":    workflow.Name,
		"stepCount": len(workflow.Steps),
	})

	resp, err := s.Automation.ExecuteWorkflow(r.Context(), workflow)
	if err != nil {
		s.broadcastEvent(map[string]interface{}{
			"type":  "WORKFLOW_FAILED",
			"id":    workflow.ID,
			"error": err.Error(),
		})
	} else {
		s.broadcastEvent(map[string]interface{}{
			"type":   "WORKFLOW_COMPLETED",
			"id":     workflow.ID,
			"status": resp.Status,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleInstantCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, `{"error":"Valid URL required"}`, http.StatusBadRequest)
		return
	}

	spec := types.SyntheticTestSpec{
		URL: req.URL,
	}

	results, screenshotBytes, err := s.Runner.ExecuteScenario(r.Context(), "instant-capture", spec)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
			"steps": results,
		})
		return
	}

	var imgURL string
	if len(screenshotBytes) > 0 {
		filename, _, errSave := s.Storage.SaveScreenshot("instant", screenshotBytes)
		if errSave == nil {
			imgURL = "/data/screenshots/" + filename
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "COMPLETED",
		"screenshotUrl": imgURL,
		"steps":         results,
	})
}

func (s *Server) handleFullScrape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var opts types.ScrapeOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil || opts.URL == "" {
		http.Error(w, `{"error":"Valid URL required in ScrapeOptions"}`, http.StatusBadRequest)
		return
	}

	s.broadcastEvent(map[string]interface{}{
		"type": "SCRAPE_STARTED",
		"url":  opts.URL,
		"mode": opts.Mode,
	})

	screenshotBytes, htmlContent, pdfBytes, err := s.Runner.ExecuteFullScrape(r.Context(), opts)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	var screenshotURL, htmlURL, pdfURL string

	if len(screenshotBytes) > 0 {
		filename, _, errSave := s.Storage.SaveScreenshot("scrape", screenshotBytes)
		if errSave == nil {
			screenshotURL = "/data/screenshots/" + filename
		}
	}

	if htmlContent != "" {
		filename, _, errSave := s.Storage.SaveHTML("scrape", htmlContent)
		if errSave == nil {
			htmlURL = "/data/reports/" + filename
		}
	}

	if len(pdfBytes) > 0 {
		filename, _, errSave := s.Storage.SavePDF("scrape", pdfBytes)
		if errSave == nil {
			pdfURL = "/data/reports/" + filename
		}
	}

	s.broadcastEvent(map[string]interface{}{
		"type":   "SCRAPE_COMPLETED",
		"url":    opts.URL,
		"html":   htmlURL,
		"pdf":    pdfURL,
		"img":    screenshotURL,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "COMPLETED",
		"url":           opts.URL,
		"screenshotUrl": screenshotURL,
		"htmlUrl":       htmlURL,
		"pdfUrl":        pdfURL,
		"htmlContent":   htmlContent,
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy","service":"k8s-chromedp-synthetics"}`))
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	s.mu.Lock()
	s.clients[conn] = true
	s.mu.Unlock()

	log.Println("New WebSocket client connected to dashboard stream")

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			s.mu.Lock()
			delete(s.clients, conn)
			s.mu.Unlock()
			conn.Close()
			break
		}
	}
}

func (s *Server) broadcastEvent(event interface{}) {
	s.broadcast <- event
}

func (s *Server) handleBroadcasts() {
	for event := range s.broadcast {
		s.mu.Lock()
		for client := range s.clients {
			err := client.WriteJSON(event)
			if err != nil {
				client.Close()
				delete(s.clients, client)
			}
		}
		s.mu.Unlock()
	}
}
