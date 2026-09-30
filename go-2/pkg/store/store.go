package store

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type StorageManager struct {
	baseDir       string
	screenshotDir string
	baselineDir   string
	diffDir       string
	reportDir     string
	mu            sync.RWMutex
}

func NewStorageManager(baseDir string) (*StorageManager, error) {
	if baseDir == "" {
		baseDir = "./data"
	}

	sm := &StorageManager{
		baseDir:       baseDir,
		screenshotDir: filepath.Join(baseDir, "screenshots"),
		baselineDir:   filepath.Join(baseDir, "baselines"),
		diffDir:       filepath.Join(baseDir, "diffs"),
		reportDir:     filepath.Join(baseDir, "reports"),
	}

	dirs := []string{sm.screenshotDir, sm.baselineDir, sm.diffDir, sm.reportDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return sm, nil
}

// SaveScreenshot saves PNG image bytes to screenshot directory and returns filename
func (s *StorageManager) SaveScreenshot(testID string, imageBytes []byte) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := fmt.Sprintf("%s_%d.png", sanitizeFilename(testID), time.Now().UnixNano())
	fullPath := filepath.Join(s.screenshotDir, filename)

	if err := os.WriteFile(fullPath, imageBytes, 0644); err != nil {
		return "", "", fmt.Errorf("failed to save screenshot: %w", err)
	}

	return filename, fullPath, nil
}

// SaveBaseline saves or updates the baseline image for visual diffing
func (s *StorageManager) SaveBaseline(testID string, imageBytes []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := fmt.Sprintf("%s_baseline.png", sanitizeFilename(testID))
	fullPath := filepath.Join(s.baselineDir, filename)

	if err := os.WriteFile(fullPath, imageBytes, 0644); err != nil {
		return "", fmt.Errorf("failed to save baseline image: %w", err)
	}

	return fullPath, nil
}

// GetBaseline retrieves baseline file path if exists
func (s *StorageManager) GetBaseline(testID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filename := fmt.Sprintf("%s_baseline.png", sanitizeFilename(testID))
	fullPath := filepath.Join(s.baselineDir, filename)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return "", false
	}
	return fullPath, true
}

// SaveDiffImage saves the rendered diff PNG image
func (s *StorageManager) SaveDiffImage(testID string, imageBytes []byte) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := fmt.Sprintf("%s_diff_%d.png", sanitizeFilename(testID), time.Now().UnixNano())
	fullPath := filepath.Join(s.diffDir, filename)

	if err := os.WriteFile(fullPath, imageBytes, 0644); err != nil {
		return "", "", fmt.Errorf("failed to save diff image: %w", err)
	}

	return filename, fullPath, nil
}

// ReadImageAsBase64 returns file content encoded as base64 string
func (s *StorageManager) ReadImageAsBase64(fullPath string) (string, error) {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// SaveHTML saves HTML string to report directory
func (s *StorageManager) SaveHTML(testID string, htmlContent string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := fmt.Sprintf("%s_%d.html", sanitizeFilename(testID), time.Now().UnixNano())
	fullPath := filepath.Join(s.reportDir, filename)

	if err := os.WriteFile(fullPath, []byte(htmlContent), 0644); err != nil {
		return "", "", fmt.Errorf("failed to save HTML report: %w", err)
	}

	return filename, fullPath, nil
}

// SavePDF saves PDF bytes to report directory
func (s *StorageManager) SavePDF(testID string, pdfBytes []byte) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := fmt.Sprintf("%s_%d.pdf", sanitizeFilename(testID), time.Now().UnixNano())
	fullPath := filepath.Join(s.reportDir, filename)

	if err := os.WriteFile(fullPath, pdfBytes, 0644); err != nil {
		return "", "", fmt.Errorf("failed to save PDF report: %w", err)
	}

	return filename, fullPath, nil
}

func sanitizeFilename(name string) string {
	result := make([]rune, 0, len(name))
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result = append(result, r)
		} else {
			result = append(result, '_')
		}
	}
	return string(result)
}
