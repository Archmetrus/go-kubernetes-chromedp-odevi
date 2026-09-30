package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"k8s-chromedp-synthetics/pkg/diff"
	"k8s-chromedp-synthetics/pkg/runner"
	"k8s-chromedp-synthetics/pkg/store"
	"k8s-chromedp-synthetics/pkg/types"
)

type AutomationGateway struct {
	Runner  *runner.ChromeRunner
	Storage *store.StorageManager
}

func NewAutomationGateway(r *runner.ChromeRunner, sm *store.StorageManager) *AutomationGateway {
	return &AutomationGateway{
		Runner:  r,
		Storage: sm,
	}
}

// ExecuteWorkflow handles Robomotion / n8n automation workflow payload execution
func (g *AutomationGateway) ExecuteWorkflow(ctx context.Context, workflow types.AutomationWorkflow) (*types.AutomationResponse, error) {
	startTime := time.Now()

	testID := workflow.ID
	if testID == "" {
		testID = fmt.Sprintf("robomotion-%d", time.Now().UnixNano())
	}

	spec := types.SyntheticTestSpec{
		Steps: workflow.Steps,
	}
	if workflow.Viewport != nil {
		spec.Viewport = *workflow.Viewport
	}

	// Run Chrome steps
	stepResults, screenshotBytes, err := g.Runner.ExecuteScenario(ctx, testID, spec)
	execTime := time.Since(startTime).Milliseconds()

	response := &types.AutomationResponse{
		WorkflowID:    testID,
		ExecutionTime: execTime,
		StepResults:   stepResults,
		ExtractedData: make(map[string]interface{}),
	}

	if err != nil {
		response.Status = "FAILED"
		response.ErrorMessage = err.Error()
	} else {
		response.Status = "COMPLETED"
	}

	// Consolidate extracted data from steps
	for _, sr := range stepResults {
		if sr.Extracted != nil {
			response.ExtractedData[sr.StepName] = sr.Extracted
		}
	}

	// Save Screenshot & process visual regression if screenshot captured
	if len(screenshotBytes) > 0 {
		_, fullPath, saveErr := g.Storage.SaveScreenshot(testID, screenshotBytes)
		if saveErr == nil {
			base64Img, _ := g.Storage.ReadImageAsBase64(fullPath)
			response.Screenshot = base64Img

			// Check for Visual Regression Baseline
			baselinePath, hasBaseline := g.Storage.GetBaseline(testID)
			if hasBaseline {
				diffResult, diffBytes, compareErr := diff.CompareImages(baselinePath, fullPath, 2.0)
				if compareErr == nil {
					response.VisualDrift = &diffResult.DiffPercent
					g.Storage.SaveDiffImage(testID, diffBytes)
				}
			} else {
				// Set current screenshot as initial baseline
				g.Storage.SaveBaseline(testID, screenshotBytes)
			}
		}
	}

	// Dispatch Asynchronous Webhook to Robomotion if WebhookURL is configured
	if workflow.WebhookURL != "" {
		go g.DispatchWebhook(workflow.WebhookURL, response)
	}

	return response, nil
}

// DispatchWebhook sends an asynchronous POST notification to the target Robomotion URL
func (g *AutomationGateway) DispatchWebhook(webhookURL string, payload *types.AutomationResponse) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling webhook payload for %s: %v", payload.WorkflowID, err)
		return
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Error building webhook request to %s: %v", webhookURL, err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "k8s-chromedp-synthetics-gateway/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Webhook delivery to %s failed: %v", webhookURL, err)
		return
	}
	defer resp.Body.Close()

	log.Printf("Webhook delivered to %s (Status: %s)", webhookURL, resp.Status)
}
