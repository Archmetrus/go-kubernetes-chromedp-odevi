package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"k8s-chromedp-synthetics/pkg/security"
	"k8s-chromedp-synthetics/pkg/store"
	"k8s-chromedp-synthetics/pkg/types"
)

type ChromeRunner struct {
	SSRFGuard *security.SSRFGuard
	Storage   *store.StorageManager
}

func NewChromeRunner(ssrf *security.SSRFGuard, storage *store.StorageManager) *ChromeRunner {
	return &ChromeRunner{
		SSRFGuard: ssrf,
		Storage:   storage,
	}
}

// SanitizeHTMLForNoImages neutralizes img/source tags and injects image-blocking CSS rules
func SanitizeHTMLForNoImages(htmlInput string) string {
	if htmlInput == "" {
		return htmlInput
	}

	styleBlock := `<style id="antigravity-block-images">
		img, picture, svg, video, audio, source, canvas, [style*="background-image"], [style*="background"] {
			display: none !important;
			visibility: hidden !important;
			background-image: none !important;
			content: none !important;
		}
	</style>`

	result := htmlInput
	lower := strings.ToLower(htmlInput)
	headIdx := strings.Index(lower, "<head>")
	if headIdx != -1 {
		result = htmlInput[:headIdx+6] + "\n" + styleBlock + "\n" + htmlInput[headIdx+6:]
	} else {
		result = styleBlock + "\n" + htmlInput
	}

	srcRegex := regexp.MustCompile(`(?i)(<(?:img|source|image|picture)[^>]*?\b)(src|srcset)\s*=\s*["']([^"']+)["']`)
	result = srcRegex.ReplaceAllString(result, `$1 data-blocked-$2="$3" src="data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg'/>"`)

	return result
}

// ExecuteScenario executes a list of steps using a hardened chromedp instance
func (r *ChromeRunner) ExecuteScenario(parentCtx context.Context, testID string, spec types.SyntheticTestSpec) ([]types.StepResult, []byte, error) {
	// 1. Initial SSRF Check
	if spec.URL != "" {
		if err := r.SSRFGuard.ValidateURL(spec.URL); err != nil {
			return nil, nil, fmt.Errorf("initial URL SSRF check failed: %w", err)
		}
	}

	// 2. Viewport defaults
	width := spec.Viewport.Width
	if width <= 0 {
		width = 1920
	}
	height := spec.Viewport.Height
	if height <= 0 {
		height = 1080
	}

	// 3. Allocator Options (Hardened chrome settings)
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Headless,
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("ignore-certificate-errors", true),
		chromedp.WindowSize(int(width), int(height)),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(parentCtx, opts...)
	defer cancelAlloc()

	// 4. Create Chromedp Context with Overall Timeout
	timeout := 120 * time.Second
	taskCtx, cancelTask := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancelTask()

	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, timeout)
	defer cancelTimeout()

	var results []types.StepResult

	// Navigate to initial URL if provided
	if spec.URL != "" {
		start := time.Now()
		navErr := chromedp.Run(taskCtx, chromedp.Navigate(spec.URL))
		dur := time.Since(start).Milliseconds()

		res := types.StepResult{
			StepName:   "Initial Navigation",
			Action:     types.ActionNavigate,
			DurationMs: dur,
		}

		if navErr != nil {
			res.Status = "failed"
			res.Error = navErr.Error()
			results = append(results, res)
			return results, nil, fmt.Errorf("initial navigation to %s failed: %w", spec.URL, navErr)
		}
		res.Status = "success"
		results = append(results, res)
	}

	// Execute Custom Steps
	for idx, step := range spec.Steps {
		stepName := step.Name
		if stepName == "" {
			stepName = fmt.Sprintf("Step-%d (%s)", idx+1, step.Action)
		}

		start := time.Now()
		stepRes := types.StepResult{
			StepName: stepName,
			Action:   step.Action,
		}

		stepErr := r.executeSingleStep(taskCtx, step, &stepRes)
		stepRes.DurationMs = time.Since(start).Milliseconds()

		if stepErr != nil {
			stepRes.Status = "failed"
			stepRes.Error = stepErr.Error()
			results = append(results, stepRes)
			return results, nil, fmt.Errorf("step '%s' failed: %w", stepName, stepErr)
		}

		stepRes.Status = "success"
		results = append(results, stepRes)
	}

	// Capture Final Full-Page Screenshot
	var screenshotBuf []byte
	err := chromedp.Run(taskCtx, chromedp.FullScreenshot(&screenshotBuf, 90))
	if err != nil {
		log.Printf("Warning: failed to capture final screenshot for %s: %v", testID, err)
	}

	return results, screenshotBuf, nil
}

// ExecuteFullScrape visits a target URL with configurable options (Fast cURL vs Chromedp vs Scrape.do Proxy API, Image Blocking, Scroll to bottom, Wait strategies)
func (r *ChromeRunner) ExecuteFullScrape(parentCtx context.Context, scrapeOpts types.ScrapeOptions) ([]byte, string, []byte, error) {
	if err := r.SSRFGuard.ValidateURL(scrapeOpts.URL); err != nil {
		return nil, "", nil, fmt.Errorf("SSRF violation for URL %s: %w", scrapeOpts.URL, err)
	}

	// MODE 1: Scrape.do Proxy API Integration (https://scrape.do/documentation/)
	if scrapeOpts.Mode == "scrape_do" {
		token := scrapeOpts.ScrapeDoToken
		if token == "" {
			token = os.Getenv("SCRAPE_DO_TOKEN")
			if token == "" {
				token = os.Getenv("SCRAPE_DO_API_KEY")
			}
		}
		if token == "" {
			return nil, "", nil, fmt.Errorf("Scrape.do API Token (token parameter) is required for scrape_do mode")
		}

		encodedTarget := url.QueryEscape(scrapeOpts.URL)
		var scrapeDoAPI string

		if scrapeOpts.BlockImages {
			// Per Scrape.do documentation: blockResources=true blocks CSS, images, and fonts.
			// Do NOT request screenshot when blockResources=true, as Scrape.do screenshot API forces blockResources=false on their servers.
			scrapeDoAPI = fmt.Sprintf("http://api.scrape.do?token=%s&url=%s&render=true&blockResources=true", token, encodedTarget)
		} else {
			if scrapeOpts.IncludeScreenshot {
				scrapeDoAPI = fmt.Sprintf("http://api.scrape.do?token=%s&url=%s&render=true&blockResources=false&fullScreenShot=true&returnJSON=true", token, encodedTarget)
			} else {
				scrapeDoAPI = fmt.Sprintf("http://api.scrape.do?token=%s&url=%s&render=true&blockResources=false", token, encodedTarget)
			}
		}

		req, err := http.NewRequestWithContext(parentCtx, "GET", scrapeDoAPI, nil)
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to build Scrape.do API request: %w", err)
		}

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", nil, fmt.Errorf("Scrape.do API request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyErr, _ := io.ReadAll(resp.Body)
			return nil, "", nil, fmt.Errorf("Scrape.do API returned HTTP %d: %s", resp.StatusCode, string(bodyErr))
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to read Scrape.do response: %w", err)
		}

		var screenshotBuf []byte
		htmlContent := string(bodyBytes)

		// Check if JSON response was returned (when returnJSON=true)
		if !scrapeOpts.BlockImages && scrapeOpts.IncludeScreenshot && strings.HasPrefix(strings.TrimSpace(htmlContent), "{") {
			var jsonResp struct {
				Body        string `json:"body"`
				HTML        string `json:"html"`
				ScreenShots []struct {
					Image string `json:"image"`
				} `json:"screenShots"`
				ScreenShot string `json:"screenShot"`
			}

			if errUnm := json.Unmarshal(bodyBytes, &jsonResp); errUnm == nil {
				if jsonResp.HTML != "" {
					htmlContent = jsonResp.HTML
				} else if jsonResp.Body != "" {
					htmlContent = jsonResp.Body
				}

				imgB64 := ""
				if len(jsonResp.ScreenShots) > 0 && jsonResp.ScreenShots[0].Image != "" {
					imgB64 = jsonResp.ScreenShots[0].Image
				} else if jsonResp.ScreenShot != "" {
					imgB64 = jsonResp.ScreenShot
				}

				if imgB64 != "" {
					// Clean data URL prefix if present
					if commaIdx := strings.Index(imgB64, ","); commaIdx != -1 {
						imgB64 = imgB64[commaIdx+1:]
					}
					decodedImg, errDec := base64.StdEncoding.DecodeString(imgB64)
					if errDec == nil {
						screenshotBuf = decodedImg
					}
				}
			}
		}

		if scrapeOpts.BlockImages {
			htmlContent = SanitizeHTMLForNoImages(htmlContent)
		}

		return screenshotBuf, htmlContent, nil, nil
	}

	// MODE 2: Fast HTTP cURL Mode (No headless browser overhead, pure static HTML fetch in milliseconds)
	if scrapeOpts.Mode == "fast_curl" {
		req, err := http.NewRequestWithContext(parentCtx, "GET", scrapeOpts.URL, nil)
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to create cURL request: %w", err)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 (k8s-chromedp-synthetics)")

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", nil, fmt.Errorf("fast cURL request failed: %w", err)
		}
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to read cURL response body: %w", err)
		}

		htmlContent := string(bodyBytes)
		if scrapeOpts.BlockImages {
			htmlContent = SanitizeHTMLForNoImages(htmlContent)
		}
		return nil, htmlContent, nil, nil
	}

	// MODE 3: Headless Chromedp Browser Engine (Full JS Rendering)
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Headless,
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("ignore-certificate-errors", true),
		chromedp.WindowSize(1920, 1080),
	)

	// Image Blocking Option
	if scrapeOpts.BlockImages {
		opts = append(opts, chromedp.Flag("blink-settings", "imagesEnabled=false"))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(parentCtx, opts...)
	defer cancelAlloc()

	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()

	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, 90*time.Second)
	defer cancelTimeout()

	var screenshotBuf []byte
	var htmlContent string
	var pdfBuf []byte

	// Build Action Pipeline
	var actions []chromedp.Action

	if scrapeOpts.BlockImages {
		actions = append(actions, network.Enable())
		actions = append(actions, network.SetBlockedURLS([]string{
			"*.png", "*.jpg", "*.jpeg", "*.gif", "*.webp", "*.svg", "*.ico", "*.avif", "*.bmp", "*.tiff",
			"*image*", "*png*", "*jpg*", "*jpeg*", "*gif*", "*webp*",
		}))
	}

	actions = append(actions, chromedp.Navigate(scrapeOpts.URL))
	actions = append(actions, chromedp.WaitReady("body", chromedp.ByQuery))

	// Scroll to bottom option to trigger lazy loading / dynamic SPA rendering
	if scrapeOpts.ScrollToBottom {
		actions = append(actions, chromedp.Evaluate(`
			(function() {
				window.scrollTo(0, document.body.scrollHeight);
			})()
		`, nil))
		actions = append(actions, chromedp.Sleep(1*time.Second))
	}

	// Wait until network idle / extra buffer
	if scrapeOpts.WaitUntil == "networkIdle" {
		actions = append(actions, chromedp.Sleep(2*time.Second))
	}

	if scrapeOpts.BlockImages {
		actions = append(actions, chromedp.Evaluate(`
			(function() {
				document.querySelectorAll('img, picture, source, video, svg, canvas').forEach(el => {
					el.removeAttribute('src');
					el.removeAttribute('srcset');
					el.style.display = 'none';
				});
				document.querySelectorAll('*').forEach(el => {
					el.style.backgroundImage = 'none';
				});
			})()
		`, nil))
	}

	actions = append(actions, chromedp.OuterHTML("html", &htmlContent, chromedp.ByQuery))

	if scrapeOpts.IncludeScreenshot || (!scrapeOpts.IncludeScreenshot && !scrapeOpts.IncludePDF && !scrapeOpts.IncludeHTML) {
		actions = append(actions, chromedp.FullScreenshot(&screenshotBuf, 90))
	}

	if scrapeOpts.IncludePDF {
		actions = append(actions, chromedp.ActionFunc(func(c context.Context) error {
			var err error
			pdfBuf, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithPreferCSSPageSize(true).
				Do(c)
			return err
		}))
	}

	err := chromedp.Run(taskCtx, actions...)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed during scrape of %s: %w", scrapeOpts.URL, err)
	}

	if scrapeOpts.BlockImages {
		htmlContent = SanitizeHTMLForNoImages(htmlContent)
	}

	return screenshotBuf, htmlContent, pdfBuf, nil
}

func (r *ChromeRunner) executeSingleStep(ctx context.Context, step types.Step, res *types.StepResult) error {
	switch step.Action {
	case types.ActionNavigate:
		if err := r.SSRFGuard.ValidateURL(step.Target); err != nil {
			return fmt.Errorf("step SSRF violation for URL %s: %w", step.Target, err)
		}
		return chromedp.Run(ctx, chromedp.Navigate(step.Target))

	case types.ActionClick:
		return chromedp.Run(ctx,
			chromedp.WaitVisible(step.Target, chromedp.ByQuery),
			chromedp.Click(step.Target, chromedp.ByQuery),
		)

	case types.ActionType:
		return chromedp.Run(ctx,
			chromedp.WaitVisible(step.Target, chromedp.ByQuery),
			chromedp.SendKeys(step.Target, step.Value, chromedp.ByQuery),
		)

	case types.ActionWaitVisible:
		return chromedp.Run(ctx, chromedp.WaitVisible(step.Target, chromedp.ByQuery))

	case types.ActionScrapeText:
		var text string
		err := chromedp.Run(ctx,
			chromedp.WaitVisible(step.Target, chromedp.ByQuery),
			chromedp.Text(step.Target, &text, chromedp.ByQuery),
		)
		if err == nil {
			res.Extracted = strings.TrimSpace(text)
		}
		return err

	case types.ActionScrapeAttribute:
		var val string
		var ok bool
		attr := step.Attribute
		if attr == "" {
			attr = "value"
		}
		err := chromedp.Run(ctx,
			chromedp.WaitVisible(step.Target, chromedp.ByQuery),
			chromedp.AttributeValue(step.Target, attr, &val, &ok, chromedp.ByQuery),
		)
		if err == nil {
			res.Extracted = val
		}
		return err

	case types.ActionExtractTable:
		// Extract table content as JSON 2D array
		jsScript := fmt.Sprintf(`
			(function() {
				var table = document.querySelector(%q);
				if (!table) return null;
				var rows = Array.from(table.querySelectorAll('tr'));
				return rows.map(r => Array.from(r.querySelectorAll('th, td')).map(c => c.innerText.trim()));
			})()
		`, step.Target)
		var tableData [][]string
		err := chromedp.Run(ctx, chromedp.Evaluate(jsScript, &tableData))
		if err == nil {
			res.Extracted = tableData
		}
		return err

	case types.ActionEvaluateJS:
		var jsResult interface{}
		err := chromedp.Run(ctx, chromedp.Evaluate(step.Value, &jsResult))
		if err == nil {
			res.Extracted = jsResult
		}
		return err

	case types.ActionPDFExport:
		var pdfBuf []byte
		err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			var err error
			pdfBuf, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithPreferCSSPageSize(true).
				Do(c)
			return err
		}))
		if err == nil {
			res.Extracted = fmt.Sprintf("PDF generated (%d bytes)", len(pdfBuf))
		}
		return err

	default:
		return fmt.Errorf("unsupported step action: %s", step.Action)
	}
}

// ConvertJsonToSpec converts a generic JSON step list into SyntheticTestSpec
func ConvertJsonToSpec(jsonBytes []byte) (*types.SyntheticTestSpec, error) {
	var spec types.SyntheticTestSpec
	if err := json.Unmarshal(jsonBytes, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}
