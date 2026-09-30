package types

import "time"

// StepAction represents the action to perform in a test or automation step
type StepAction string

const (
	ActionNavigate        StepAction = "navigate"
	ActionClick           StepAction = "click"
	ActionType            StepAction = "type"
	ActionWaitVisible     StepAction = "wait_visible"
	ActionScreenshot      StepAction = "screenshot"
	ActionScrapeText      StepAction = "scrape_text"
	ActionScrapeAttribute StepAction = "scrape_attribute"
	ActionExtractTable    StepAction = "extract_table"
	ActionEvaluateJS      StepAction = "evaluate_js"
	ActionPDFExport       StepAction = "pdf_export"
)

// Viewport defines browser dimension options
type Viewport struct {
	Width  int64 `json:"width" yaml:"width"`
	Height int64 `json:"height" yaml:"height"`
}

// Step defines an individual action in a scenario
type Step struct {
	Name        string     `json:"name" yaml:"name"`
	Action      StepAction `json:"action" yaml:"action"`
	Target      string     `json:"target,omitempty" yaml:"target,omitempty"`           // CSS selector or XPath or URL
	Value       string     `json:"value,omitempty" yaml:"value,omitempty"`             // Input text or JS snippet
	TimeoutSec  int        `json:"timeoutSec,omitempty" yaml:"timeoutSec,omitempty"`   // Step timeout
	Attribute   string     `json:"attribute,omitempty" yaml:"attribute,omitempty"`   // HTML attribute to scrape
	ExportFile  string     `json:"exportFile,omitempty" yaml:"exportFile,omitempty"`   // File path for PDF/Screenshot
}

// SyntheticTestSpec defines Kubernetes CRD Spec for SyntheticTest
type SyntheticTestSpec struct {
	URL              string   `json:"url" yaml:"url"`
	Schedule         string   `json:"schedule,omitempty" yaml:"schedule,omitempty"`         // Cron string e.g. "*/5 * * * *"
	IntervalSeconds  int      `json:"intervalSeconds,omitempty" yaml:"intervalSeconds,omitempty"`
	Viewport         Viewport `json:"viewport,omitempty" yaml:"viewport,omitempty"`
	Steps            []Step   `json:"steps,omitempty" yaml:"steps,omitempty"`
	VisualRegression bool     `json:"visualRegression,omitempty" yaml:"visualRegression,omitempty"`
	DriftThreshold   float64  `json:"driftThreshold,omitempty" yaml:"driftThreshold,omitempty"` // e.g. 2.5 (%)
	WebhookURL       string   `json:"webhookUrl,omitempty" yaml:"webhookUrl,omitempty"`
}

// StepResult stores output for each executed step
type StepResult struct {
	StepName   string        `json:"stepName"`
	Action     StepAction    `json:"action"`
	Status     string        `json:"status"` // "success" or "failed"
	DurationMs int64         `json:"durationMs"`
	Error      string        `json:"error,omitempty"`
	Extracted  interface{}   `json:"extracted,omitempty"`
	Screenshot string        `json:"screenshotUrl,omitempty"`
}

// SyntheticTestStatus defines Kubernetes CRD Status
type SyntheticTestStatus struct {
	Phase           string       `json:"phase"` // "Pending", "Running", "Succeeded", "Failed"
	LastRunTime     time.Time    `json:"lastRunTime"`
	SuccessRate     float64      `json:"successRate"`
	TotalRuns       int64        `json:"totalRuns"`
	VisualDrift     float64      `json:"visualDrift"`
	LastErrorMessage string      `json:"lastErrorMessage,omitempty"`
	StepResults     []StepResult `json:"stepResults,omitempty"`
}

// SyntheticTestResource represents a CRD instance in Go
type SyntheticTestResource struct {
	APIVersion string              `json:"apiVersion" yaml:"apiVersion"`
	Kind       string              `json:"kind" yaml:"kind"`
	Metadata   map[string]interface{} `json:"metadata" yaml:"metadata"`
	Spec       SyntheticTestSpec   `json:"spec" yaml:"spec"`
	Status     SyntheticTestStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// AutomationWorkflow defines Robomotion / n8n JSON payload schema
type AutomationWorkflow struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	WebhookURL  string            `json:"webhookUrl,omitempty"`
	APIKey      string            `json:"apiKey,omitempty"`
	Viewport    *Viewport         `json:"viewport,omitempty"`
	Steps       []Step            `json:"steps"`
	ContextData map[string]string `json:"contextData,omitempty"`
}

// AutomationResponse is returned to external automation software (Robomotion)
type AutomationResponse struct {
	WorkflowID    string                 `json:"workflowId"`
	Status        string                 `json:"status"` // "COMPLETED" or "FAILED"
	ExecutionTime int64                  `json:"executionTimeMs"`
	StepResults   []StepResult           `json:"stepResults"`
	ExtractedData map[string]interface{} `json:"extractedData,omitempty"`
	Screenshot    string                 `json:"screenshotBase64,omitempty"`
	VisualDrift   *float64               `json:"visualDriftPercentage,omitempty"`
	ErrorMessage  string                 `json:"errorMessage,omitempty"`
}

// VisualDiffResult represents screenshot comparison output
type VisualDiffResult struct {
	BaselinePath string  `json:"baselinePath"`
	CurrentPath  string  `json:"currentPath"`
	DiffPath     string  `json:"diffPath"`
	DiffPercent  float64 `json:"diffPercent"`
	IsDrifted    bool    `json:"isDrifted"`
}

// SecurityConfig holds security options
type SecurityConfig struct {
	APIKey           string   `json:"apiKey"`
	EnableSSRFGuard  bool     `json:"enableSSRFGuard"`
	AllowedHosts     []string `json:"allowedHosts"`
	BlockedHosts     []string `json:"blockedHosts"`
	RateLimitRequests float64  `json:"rateLimitRequests"` // Requests per second
	RateLimitBurst   int      `json:"rateLimitBurst"`
}

// ScrapeOptions defines parameters for site scraping & capture
type ScrapeOptions struct {
	URL               string `json:"url"`
	Mode              string `json:"mode,omitempty"`              // "chromedp", "fast_curl", or "scrape_do"
	ScrapeDoToken     string `json:"scrapeDoToken,omitempty"`     // API Token for Scrape.do
	BlockImages       bool   `json:"blockImages,omitempty"`       // Block images for high speed
	ScrollToBottom    bool   `json:"scrollToBottom,omitempty"`    // Scroll down to trigger lazy loading
	WaitUntil         string `json:"waitUntil,omitempty"`         // "load", "domContentLoaded", "networkIdle"
	IncludeScreenshot bool   `json:"includeScreenshot,omitempty"` // Capture PNG
	IncludePDF        bool   `json:"includePDF,omitempty"`        // Capture PDF
	IncludeHTML       bool   `json:"includeHTML,omitempty"`       // Capture HTML
}
