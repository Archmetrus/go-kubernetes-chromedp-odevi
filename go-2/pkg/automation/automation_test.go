package automation

import (
	"encoding/json"
	"testing"

	"k8s-chromedp-synthetics/pkg/types"
)

func TestAutomationWorkflowUnmarshal(t *testing.T) {
	jsonPayload := `{
		"id": "test-flow-01",
		"name": "Robomotion Demo Test",
		"webhookUrl": "https://company.com/webhook",
		"viewport": { "width": 1280, "height": 800 },
		"steps": [
			{ "name": "Go to Page", "action": "navigate", "target": "https://example.com" },
			{ "name": "Extract Header", "action": "scrape_text", "target": "h1" }
		]
	}`

	var wf types.AutomationWorkflow
	err := json.Unmarshal([]byte(jsonPayload), &wf)
	if err != nil {
		t.Fatalf("failed to unmarshal Robomotion workflow JSON: %v", err)
	}

	if wf.ID != "test-flow-01" {
		t.Errorf("expected ID 'test-flow-01', got %s", wf.ID)
	}

	if len(wf.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(wf.Steps))
	}

	if wf.Steps[1].Action != types.ActionScrapeText {
		t.Errorf("expected ActionScrapeText, got %s", wf.Steps[1].Action)
	}
}
