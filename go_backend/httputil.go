package gobackend

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// getRandomUserAgent generates a random Windows Chrome User-Agent string
// Uses modern Chrome format with build and patch numbers
// Windows 11 still reports as "Windows NT 10.0" for compatibility
func getRandomUserAgent() string {
	// Chrome version 120-145 (modern range)
	chromeVersion := rand.Intn(26) + 120
	chromeBuild := rand.Intn(1500) + 6000
	chromePatch := rand.Intn(200) + 100

	return fmt.Sprintf(
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.%d.%d Safari/537.36",
		chromeVersion,
		chromeBuild,
		chromePatch,
	)
}

const (
	DefaultTimeout    = 60 * time.Second
	DownloadTimeout   = 120 * time.Second
	SongLinkTimeout   = 30 * time.Second
	DefaultMaxRetries = 3
	DefaultRetryDelay = 1 * time.Second
)

// Shared transport with connection pooling to prevent TCP exhaustion
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
	DisableKeepAlives:     false,
	ForceAttemptHTTP2:     true,
	WriteBufferSize:       64 * 1024,
	ReadBufferSize:        64 * 1024,
	DisableCompression:    true,
}

// uTLS transport that mimics Chrome's TLS fingerprint to bypass Cloudflare
// Uses HTTP/2 for optimal performance as uTLS works best with HTTP/2
type utlsTransport struct {
	dialer       *net.Dialer
	mu           sync.Mutex
	h2Transports map[string]*http2.Transport
}

func newUTLSTransport() *utlsTransport {
	return &utlsTransport{
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
		h2Transports: make(map[string]*http2.Transport),
	}
}

func (t *utlsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// For non-HTTPS, use standard transport
	if req.URL.Scheme != "https" {
		return sharedTransport.RoundTrip(req)
	}

	host := req.URL.Hostname()
	port := t.getPort(req.URL)
	addr := net.JoinHostPort(host, port)

	// Dial TCP connection
	conn, err := t.dialer.DialContext(req.Context(), "tcp", addr)
	if err != nil {
		return nil, err
	}

	// Create uTLS connection with Chrome fingerprint (supports HTTP/2 ALPN)
	tlsConn := utls.UClient(conn, &utls.Config{
		ServerName: host,
		NextProtos: []string{"h2", "http/1.1"}, // Prefer HTTP/2
	}, utls.HelloChrome_Auto)

	// Perform TLS handshake
	if err := tlsConn.Handshake(); err != nil {
		conn.Close()
		return nil, err
	}

	// Check if server supports HTTP/2
	negotiatedProto := tlsConn.ConnectionState().NegotiatedProtocol

	if negotiatedProto == "h2" {
		// Use HTTP/2 transport
		h2Transport := &http2.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				return tlsConn, nil
			},
			AllowHTTP:          false,
			DisableCompression: false,
		}
		return h2Transport.RoundTrip(req)
	}

	// Fallback to HTTP/1.1
	transport := &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return tlsConn, nil
		},
		DisableKeepAlives: true,
	}

	return transport.RoundTrip(req)
}

func (t *utlsTransport) getPort(u *url.URL) string {
	if u.Port() != "" {
		return u.Port()
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

// Cloudflare bypass client using uTLS Chrome fingerprint
var cloudflareBypassTransport = newUTLSTransport()

var cloudflareBypassClient = &http.Client{
	Transport: cloudflareBypassTransport,
	Timeout:   DefaultTimeout,
}

var sharedClient = &http.Client{
	Transport: sharedTransport,
	Timeout:   DefaultTimeout,
}

var downloadClient = &http.Client{
	Transport: sharedTransport,
	Timeout:   DownloadTimeout,
}

func NewHTTPClientWithTimeout(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: sharedTransport,
		Timeout:   timeout,
	}
}

func GetSharedClient() *http.Client {
	return sharedClient
}

func GetDownloadClient() *http.Client {
	return downloadClient
}

// GetCloudflareBypassClient returns an HTTP client that mimics Chrome's TLS fingerprint
// Use this when requests are blocked by Cloudflare (common when using VPN)
func GetCloudflareBypassClient() *http.Client {
	return cloudflareBypassClient
}

// CloseIdleConnections closes idle connections in the shared transport
func CloseIdleConnections() {
	sharedTransport.CloseIdleConnections()
}

// Also checks for ISP blocking on errors
func DoRequestWithUserAgent(client *http.Client, req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", getRandomUserAgent())
	resp, err := client.Do(req)
	if err != nil {
		CheckAndLogISPBlocking(err, req.URL.String(), "HTTP")
	}
	return resp, err
}

// DoRequestWithCloudflareBypass attempts request with standard client first,
// then retries with uTLS Chrome fingerprint if Cloudflare blocks it.
// This is useful when using VPN as Cloudflare detects Go's default TLS fingerprint.
func DoRequestWithCloudflareBypass(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", getRandomUserAgent())

	// Try with standard client first
	resp, err := sharedClient.Do(req)
	if err == nil {
		// Check for Cloudflare challenge page (403 with specific markers)
		if resp.StatusCode == 403 || resp.StatusCode == 503 {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()

			if readErr == nil {
				bodyStr := strings.ToLower(string(body))
				cloudflareMarkers := []string{
					"cloudflare", "cf-ray", "checking your browser",
					"please wait", "ddos protection", "ray id",
					"enable javascript", "challenge-platform",
				}

				isCloudflare := false
				for _, marker := range cloudflareMarkers {
					if strings.Contains(bodyStr, marker) {
						isCloudflare = true
						break
					}
				}

				if isCloudflare {
					LogDebug("HTTP", "Cloudflare detected, retrying with Chrome TLS fingerprint...")

					// Clone request for retry
					reqCopy := req.Clone(req.Context())
					reqCopy.Header.Set("User-Agent", getRandomUserAgent())

					// Retry with uTLS Chrome fingerprint
					return cloudflareBypassClient.Do(reqCopy)
				}
			}

			// Not Cloudflare, return original response (recreate body)
			return &http.Response{
				Status:     resp.Status,
				StatusCode: resp.StatusCode,
				Header:     resp.Header,
				Body:       io.NopCloser(strings.NewReader(string(body))),
			}, nil
		}
		return resp, nil
	}

	// Check if error might be TLS-related (Cloudflare blocking)
	errStr := strings.ToLower(err.Error())
	tlsRelated := strings.Contains(errStr, "tls") ||
		strings.Contains(errStr, "handshake") ||
		strings.Contains(errStr, "certificate") ||
		strings.Contains(errStr, "connection reset")

	if tlsRelated {
		LogDebug("HTTP", "TLS error detected, retrying with Chrome TLS fingerprint: %v", err)

		// Clone request for retry
		reqCopy := req.Clone(req.Context())
		reqCopy.Header.Set("User-Agent", getRandomUserAgent())

		// Retry with uTLS Chrome fingerprint
		return cloudflareBypassClient.Do(reqCopy)
	}

	CheckAndLogISPBlocking(err, req.URL.String(), "HTTP")
	return nil, err
}

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
}

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
// Also detects and logs ISP blocking
func DoRequestWithRetry(client *http.Client, req *http.Request, config RetryConfig) (*http.Response, error) {
	var lastErr error
	delay := config.InitialDelay
	requestURL := req.URL.String()

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Clone request for retry (body needs to be re-readable)
		reqCopy := req.Clone(req.Context())
		reqCopy.Header.Set("User-Agent", getRandomUserAgent())

		resp, err := client.Do(reqCopy)
		if err != nil {
			lastErr = err

			// Check for ISP blocking on network errors
			if CheckAndLogISPBlocking(err, requestURL, "HTTP") {
				// Don't retry if ISP blocking is detected - it won't help
				return nil, WrapErrorWithISPCheck(err, requestURL, "HTTP")
			}

			if attempt < config.MaxRetries {
				GoLog("[HTTP] Request failed (attempt %d/%d): %v, retrying in %v...\n",
					attempt+1, config.MaxRetries+1, err, delay)
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
				GoLog("[HTTP] Rate limited, waiting %v before retry...\n", delay)
				time.Sleep(delay)
				delay = calculateNextDelay(delay, config)
			}
			continue
		}

		// Check for ISP blocking via HTTP status codes
		// Some ISPs return 403 or 451 when blocking content
		if resp.StatusCode == 403 || resp.StatusCode == 451 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			bodyStr := strings.ToLower(string(body))

			// Check if response looks like ISP blocking page
			ispBlockingIndicators := []string{
				"blocked", "forbidden", "access denied", "not available in your",
				"restricted", "censored", "unavailable for legal", "blocked by",
			}

			for _, indicator := range ispBlockingIndicators {
				if strings.Contains(bodyStr, indicator) {
					LogError("HTTP", "ISP BLOCKING DETECTED via HTTP %d response", resp.StatusCode)
					LogError("HTTP", "Domain: %s", req.URL.Host)
					LogError("HTTP", "Response contains: %s", indicator)
					LogError("HTTP", "Suggestion: Try using a VPN or changing your DNS to 1.1.1.1 or 8.8.8.8")
					return nil, fmt.Errorf("ISP blocking detected for %s (HTTP %d) - try using VPN or change DNS", req.URL.Host, resp.StatusCode)
				}
			}
		}

		// Server errors (5xx) - retry
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server error: HTTP %d", resp.StatusCode)
			if attempt < config.MaxRetries {
				GoLog("[HTTP] Server error %d, retrying in %v...\n", resp.StatusCode, delay)
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

func calculateNextDelay(currentDelay time.Duration, config RetryConfig) time.Duration {
	nextDelay := time.Duration(float64(currentDelay) * config.BackoffFactor)
	return min(nextDelay, config.MaxDelay)
}

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

type ISPBlockingError struct {
	Domain      string
	Reason      string
	OriginalErr error
}

func (e *ISPBlockingError) Error() string {
	return fmt.Sprintf("ISP blocking detected for %s: %s", e.Domain, e.Reason)
}

// IsISPBlocking checks if an error is likely caused by ISP blocking
// Returns the ISPBlockingError if detected, nil otherwise
func IsISPBlocking(err error, requestURL string) *ISPBlockingError {
	if err == nil {
		return nil
	}

	// Extract domain from URL
	domain := extractDomain(requestURL)
	errStr := strings.ToLower(err.Error())

	// Check for DNS resolution failure (common ISP blocking method)
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound || dnsErr.IsTemporary {
			return &ISPBlockingError{
				Domain:      domain,
				Reason:      "DNS resolution failed - domain may be blocked by ISP",
				OriginalErr: err,
			}
		}
	}

	// Check for connection refused (ISP firewall blocking)
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" {
			// Check for specific syscall errors
			var syscallErr syscall.Errno
			if errors.As(opErr.Err, &syscallErr) {
				switch syscallErr {
				case syscall.ECONNREFUSED:
					return &ISPBlockingError{
						Domain:      domain,
						Reason:      "Connection refused - port may be blocked by ISP/firewall",
						OriginalErr: err,
					}
				case syscall.ECONNRESET:
					return &ISPBlockingError{
						Domain:      domain,
						Reason:      "Connection reset - ISP may be intercepting traffic",
						OriginalErr: err,
					}
				case syscall.ETIMEDOUT:
					return &ISPBlockingError{
						Domain:      domain,
						Reason:      "Connection timed out - ISP may be blocking access",
						OriginalErr: err,
					}
				case syscall.ENETUNREACH:
					return &ISPBlockingError{
						Domain:      domain,
						Reason:      "Network unreachable - ISP may be blocking route",
						OriginalErr: err,
					}
				case syscall.EHOSTUNREACH:
					return &ISPBlockingError{
						Domain:      domain,
						Reason:      "Host unreachable - ISP may be blocking destination",
						OriginalErr: err,
					}
				}
			}
		}
	}

	// Check for TLS handshake failure (ISP MITM or blocking HTTPS)
	var tlsErr *tls.RecordHeaderError
	if errors.As(err, &tlsErr) {
		return &ISPBlockingError{
			Domain:      domain,
			Reason:      "TLS handshake failed - ISP may be intercepting HTTPS traffic",
			OriginalErr: err,
		}
	}

	// Check error message patterns for common ISP blocking indicators
	blockingPatterns := []struct {
		pattern string
		reason  string
	}{
		{"connection reset by peer", "Connection reset - ISP may be intercepting traffic"},
		{"connection refused", "Connection refused - port may be blocked"},
		{"no such host", "DNS lookup failed - domain may be blocked by ISP"},
		{"i/o timeout", "Connection timed out - ISP may be blocking access"},
		{"network is unreachable", "Network unreachable - ISP may be blocking route"},
		{"tls: ", "TLS error - ISP may be intercepting HTTPS traffic"},
		{"certificate", "Certificate error - ISP may be using MITM proxy"},
		{"eof", "Connection closed unexpectedly - ISP may be blocking"},
		{"context deadline exceeded", "Request timed out - ISP may be throttling"},
	}

	for _, bp := range blockingPatterns {
		if strings.Contains(errStr, bp.pattern) {
			return &ISPBlockingError{
				Domain:      domain,
				Reason:      bp.reason,
				OriginalErr: err,
			}
		}
	}

	return nil
}

// Returns true if ISP blocking was detected
func CheckAndLogISPBlocking(err error, requestURL string, tag string) bool {
	ispErr := IsISPBlocking(err, requestURL)
	if ispErr != nil {
		LogError(tag, "ISP BLOCKING DETECTED: %s", ispErr.Error())
		LogError(tag, "Domain: %s", ispErr.Domain)
		LogError(tag, "Reason: %s", ispErr.Reason)
		LogError(tag, "Original error: %v", ispErr.OriginalErr)
		LogError(tag, "Suggestion: Try using a VPN or changing your DNS to 1.1.1.1 or 8.8.8.8")
		return true
	}
	return false
}

// extractDomain extracts the domain from a URL string
func extractDomain(rawURL string) string {
	if rawURL == "" {
		return "unknown"
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		// Try to extract domain manually
		rawURL = strings.TrimPrefix(rawURL, "https://")
		rawURL = strings.TrimPrefix(rawURL, "http://")
		if idx := strings.Index(rawURL, "/"); idx > 0 {
			return rawURL[:idx]
		}
		return rawURL
	}

	if parsed.Host != "" {
		return parsed.Host
	}
	return "unknown"
}

// If ISP blocking is detected, returns a more descriptive error
func WrapErrorWithISPCheck(err error, requestURL string, tag string) error {
	if err == nil {
		return nil
	}

	if CheckAndLogISPBlocking(err, requestURL, tag) {
		domain := extractDomain(requestURL)
		return fmt.Errorf("ISP blocking detected for %s - try using VPN or change DNS to 1.1.1.1/8.8.8.8: %w", domain, err)
	}

	return err
}
