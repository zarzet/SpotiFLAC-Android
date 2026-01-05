package gobackend

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// SongLinkClient handles song.link API interactions
type SongLinkClient struct {
	client *http.Client
}

// TrackAvailability represents track availability on different platforms
type TrackAvailability struct {
	SpotifyID string `json:"spotify_id"`
	Tidal     bool   `json:"tidal"`
	Amazon    bool   `json:"amazon"`
	Qobuz     bool   `json:"qobuz"`
	TidalURL  string `json:"tidal_url,omitempty"`
	AmazonURL string `json:"amazon_url,omitempty"`
	QobuzURL  string `json:"qobuz_url,omitempty"`
}

var (
	// Global SongLink client instance for connection reuse
	globalSongLinkClient *SongLinkClient
	songLinkClientOnce   sync.Once
)

// NewSongLinkClient creates a new SongLink client (returns singleton for connection reuse)
func NewSongLinkClient() *SongLinkClient {
	songLinkClientOnce.Do(func() {
		globalSongLinkClient = &SongLinkClient{
			client: NewHTTPClientWithTimeout(SongLinkTimeout), // 30s timeout
		}
	})
	return globalSongLinkClient
}

// CheckTrackAvailability checks track availability on streaming platforms
func (s *SongLinkClient) CheckTrackAvailability(spotifyTrackID string, isrc string) (*TrackAvailability, error) {
	// Use global rate limiter - blocks until request is allowed
	songLinkRateLimiter.WaitForSlot()

	// Build API URL
	spotifyBase, _ := base64.StdEncoding.DecodeString("aHR0cHM6Ly9vcGVuLnNwb3RpZnkuY29tL3RyYWNrLw==")
	spotifyURL := fmt.Sprintf("%s%s", string(spotifyBase), spotifyTrackID)

	apiBase, _ := base64.StdEncoding.DecodeString("aHR0cHM6Ly9hcGkuc29uZy5saW5rL3YxLWFscGhhLjEvbGlua3M/dXJsPQ==")
	apiURL := fmt.Sprintf("%s%s", string(apiBase), url.QueryEscape(spotifyURL))

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use retry logic with User-Agent
	retryConfig := DefaultRetryConfig()
	resp, err := DoRequestWithRetry(s.client, req, retryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to check availability: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	body, err := ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var songLinkResp struct {
		LinksByPlatform map[string]struct {
			URL string `json:"url"`
		} `json:"linksByPlatform"`
	}

	if err := json.Unmarshal(body, &songLinkResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	availability := &TrackAvailability{
		SpotifyID: spotifyTrackID,
	}

	// Check Tidal
	if tidalLink, ok := songLinkResp.LinksByPlatform["tidal"]; ok && tidalLink.URL != "" {
		availability.Tidal = true
		availability.TidalURL = tidalLink.URL
	}

	// Check Amazon
	if amazonLink, ok := songLinkResp.LinksByPlatform["amazonMusic"]; ok && amazonLink.URL != "" {
		availability.Amazon = true
		availability.AmazonURL = amazonLink.URL
	}

	// Check Qobuz using ISRC
	if isrc != "" {
		availability.Qobuz = checkQobuzAvailability(isrc)
	}

	return availability, nil
}

// GetStreamingURLs gets streaming URLs for a Spotify track
func (s *SongLinkClient) GetStreamingURLs(spotifyTrackID string) (map[string]string, error) {
	availability, err := s.CheckTrackAvailability(spotifyTrackID, "")
	if err != nil {
		return nil, err
	}

	urls := make(map[string]string)
	if availability.TidalURL != "" {
		urls["tidal"] = availability.TidalURL
	}
	if availability.AmazonURL != "" {
		urls["amazon"] = availability.AmazonURL
	}

	return urls, nil
}

func checkQobuzAvailability(isrc string) bool {
	client := NewHTTPClientWithTimeout(10 * time.Second)
	appID := "798273057"

	apiBase, _ := base64.StdEncoding.DecodeString("aHR0cHM6Ly93d3cucW9idXouY29tL2FwaS5qc29uLzAuMi90cmFjay9zZWFyY2g/cXVlcnk9")
	searchURL := fmt.Sprintf("%s%s&limit=1&app_id=%s", string(apiBase), isrc, appID)

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return false
	}

	resp, err := DoRequestWithUserAgent(client, req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false
	}

	var searchResp struct {
		Tracks struct {
			Total int `json:"total"`
		} `json:"tracks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return false
	}

	return searchResp.Tracks.Total > 0
}
