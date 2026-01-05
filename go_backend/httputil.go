package gobackend

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"time"
)

// HTTP utility functions for consistent request handling across all downloaders

// User-Agent pool for Android Chrome browsers
var userAgentTemplates = []string{
	"Mozilla/5.0 (Linux; Android %d; SM-G%d) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.%d.%d Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android %d; Pixel %d) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.%d.%d Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android %d; SM-A%d) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.%d.%d Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android %d; Redmi Note %d) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.%d.%d Mobile Safari/537.36",
}

// getRandomUserAgent generates a random browser-like User-Agent string (Android Chrome format)
func getRandomUserAgent() string {
	template := userAgentTemplates[rand.Intn(len(userAgentTemplates))]

	androidVersion := rand.Intn(5) + 10 // Android 10-14
	deviceModel := rand.Intn(900) + 100 // Random model number
	chromeVersion := rand.Intn(25) + 100 // Chrome 100-124
	chromeBuild := rand.Intn(5000) + 5000
	chromePatch := rand.Intn(200) + 100

	return fmt.Sprintf(template, androidVersion, deviceModel, chromeVersion, chromeBuild, chromePatch)
}

// Default timeout values
const (
	DefaultTimeout     = 60 * time.Second  // Default HTTP timeout
	DownloadTimeout    = 120 * time.Second // Timeout for file downloads
	SongLinkTimeout    = 30 * time.Second  // Timeout for SongLink API
	DefaultMaxRetries  = 3                 // Default retry count
	DefaultRetryDelay  = 1 * time.Second   // Initial retry delay
)

// Shared transport with connection pooling to prevent TCP exhaustion
// Optimized for large file downloads (FLAC ~30-50MB)
var sharedTransport = &http.Transport{
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   10,
	MaxConnsPerHost:       20,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	DisableKeepAlives:     false, // Enable keep-alives for connection reuse
	ForceAttemptHTTP2:     true,
	WriteBufferSize:       64 * 1024,  // 64KB write buffer
	ReadBufferSize:        64 * 1024,  // 64KB read buffer
	DisableCompression:    true,       // FLAC is already compressed
}

// Shared HTTP client for general requests (reuses connections)
var sharedClient = &http.Client{
	Transport: sharedTransport,
	Timeout:   DefaultTimeout,
}

// Shared HTTP client for downloads (longer timeout, reuses connections)
var downloadClient = &http.Client{
	Transport: sharedTransport,
	Timeout:   DownloadTimeout,
}

// NewHTTPClientWithTimeout creates an HTTP client with specified timeout
// Uses shared transport for connection reuse
func NewHTTPClientWithTimeout(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: sharedTransport,
		Timeout:   timeout,
	}
}

// GetSharedClient returns the shared HTTP client for general requests
func GetSharedClient() *http.Client {
	return sharedClient
}

// GetDownloadClient returns the shared HTTP client for downloads
func GetDownloadClient() *http.Client {
	return downloadClient
}

// CloseIdleConnections closes idle connections in the shared transport
// Call this periodically during large batch downloads to prevent connection buildup
func CloseIdleConnections() {
	sharedTransport.CloseIdleConnections()
}

// DoRequestWithUserAgent executes an HTTP request with a random User-Agent header
func DoRequestWithUserAgent(client *http.Client, req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", getRandomUserAgent())
	return client.Do(req)
}

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    DefaultMaxRetries,
		InitialDelay:  DefaultRetryDelay,
		MaxDelay:      16 * time.Second,
		BackoffFactor: 2.0,
	}
}

// DoRequestWithRetry executes an HTTP request with retry logic and exponential backoff
// Handles 429 (Too Many Requests) responses with Retry-After header
func DoRequestWithRetry(client *http.Client, req *http.Request, config RetryConfig) (*http.Response, error) {
	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Clone request for retry (body needs to be re-readable)
		reqCopy := req.Clone(req.Context())
		reqCopy.Header.Set("User-Agent", getRandomUserAgent())

		resp, err := client.Do(reqCopy)
		if err != nil {
			lastErr = err
			if attempt < config.MaxRetries {
				time.Sleep(delay)
				delay = calculateNextDelay(delay, config)
			}
			continue
		}

		// Success
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		// Handle rate limiting (429)
		if resp.StatusCode == 429 {
			resp.Body.Close()
			retryAfter := getRetryAfterDuration(resp)
			if retryAfter > 0 {
				delay = retryAfter
			}
			lastErr = fmt.Errorf("rate limited (429)")
			if attempt < config.MaxRetries {
				time.Sleep(delay)
				delay = calculateNextDelay(delay, config)
			}
			continue
		}

		// Server errors (5xx) - retry
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server error: HTTP %d", resp.StatusCode)
			if attempt < config.MaxRetries {
				time.Sleep(delay)
				delay = calculateNextDelay(delay, config)
			}
			continue
		}

		// Client errors (4xx except 429) - don't retry
		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", config.MaxRetries+1, lastErr)
}

// calculateNextDelay calculates the next delay with exponential backoff
func calculateNextDelay(currentDelay time.Duration, config RetryConfig) time.Duration {
	nextDelay := time.Duration(float64(currentDelay) * config.BackoffFactor)
	if nextDelay > config.MaxDelay {
		nextDelay = config.MaxDelay
	}
	return nextDelay
}

// getRetryAfterDuration parses Retry-After header and returns duration
// Returns 60 seconds as default if header is missing or invalid
func getRetryAfterDuration(resp *http.Response) time.Duration {
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		return 60 * time.Second // Default wait time
	}

	// Try parsing as seconds
	if seconds, err := strconv.Atoi(retryAfter); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP date
	if t, err := http.ParseTime(retryAfter); err == nil {
		duration := time.Until(t)
		if duration > 0 {
			return duration
		}
	}

	return 60 * time.Second // Default
}

// ReadResponseBody reads and returns the response body
// Returns error if body is empty
func ReadResponseBody(resp *http.Response) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("response is nil")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("response body is empty")
	}

	return body, nil
}

// ValidateResponse checks if response is valid (non-nil, status 2xx)
func ValidateResponse(resp *http.Response) error {
	if resp == nil {
		return fmt.Errorf("response is nil")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return nil
}

// BuildErrorMessage creates a detailed error message for API failures
func BuildErrorMessage(apiURL string, statusCode int, responsePreview string) string {
	msg := fmt.Sprintf("API %s failed", apiURL)
	if statusCode > 0 {
		msg += fmt.Sprintf(" (HTTP %d)", statusCode)
	}
	if responsePreview != "" {
		// Truncate preview if too long
		if len(responsePreview) > 100 {
			responsePreview = responsePreview[:100] + "..."
		}
		msg += fmt.Sprintf(": %s", responsePreview)
	}
	return msg
}
