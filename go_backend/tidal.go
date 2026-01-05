package gobackend

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// TidalDownloader handles Tidal downloads
type TidalDownloader struct {
	client         *http.Client
	clientID       string
	clientSecret   string
	apiURL         string
	cachedToken    string
	tokenExpiresAt time.Time
	tokenMu        sync.Mutex
}

var (
	// Global Tidal downloader instance for token reuse
	globalTidalDownloader *TidalDownloader
	tidalDownloaderOnce   sync.Once
)

// TidalTrack represents a Tidal track
type TidalTrack struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	ISRC         string `json:"isrc"`
	AudioQuality string `json:"audioQuality"`
	TrackNumber  int    `json:"trackNumber"`
	VolumeNumber int    `json:"volumeNumber"`
	Duration     int    `json:"duration"`
	Album        struct {
		Title       string `json:"title"`
		Cover       string `json:"cover"`
		ReleaseDate string `json:"releaseDate"`
	} `json:"album"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Artist struct {
		Name string `json:"name"`
	} `json:"artist"`
	MediaMetadata struct {
		Tags []string `json:"tags"`
	} `json:"mediaMetadata"`
}

// TidalAPIResponseV2 is the new API response format (version 2.0)
type TidalAPIResponseV2 struct {
	Version string `json:"version"`
	Data    struct {
		TrackID           int64  `json:"trackId"`
		AssetPresentation string `json:"assetPresentation"`
		AudioMode         string `json:"audioMode"`
		AudioQuality      string `json:"audioQuality"`
		ManifestMimeType  string `json:"manifestMimeType"`
		ManifestHash      string `json:"manifestHash"`
		Manifest          string `json:"manifest"`
		BitDepth          int    `json:"bitDepth"`
		SampleRate        int    `json:"sampleRate"`
	} `json:"data"`
}

// TidalBTSManifest is the BTS (application/vnd.tidal.bts) manifest format
type TidalBTSManifest struct {
	MimeType       string   `json:"mimeType"`
	Codecs         string   `json:"codecs"`
	EncryptionType string   `json:"encryptionType"`
	URLs           []string `json:"urls"`
}

// MPD represents DASH manifest structure
type MPD struct {
	XMLName xml.Name `xml:"MPD"`
	Period  struct {
		AdaptationSet struct {
			Representation struct {
				SegmentTemplate struct {
					Initialization string `xml:"initialization,attr"`
					Media          string `xml:"media,attr"`
					Timeline       struct {
						Segments []struct {
							Duration int `xml:"d,attr"`
							Repeat   int `xml:"r,attr"`
						} `xml:"S"`
					} `xml:"SegmentTimeline"`
				} `xml:"SegmentTemplate"`
			} `xml:"Representation"`
		} `xml:"AdaptationSet"`
	} `xml:"Period"`
}

// NewTidalDownloader creates a new Tidal downloader (returns singleton for token reuse)
func NewTidalDownloader() *TidalDownloader {
	tidalDownloaderOnce.Do(func() {
		clientID, _ := base64.StdEncoding.DecodeString("NkJEU1JkcEs5aHFFQlRnVQ==")
		clientSecret, _ := base64.StdEncoding.DecodeString("eGV1UG1ZN25icFo5SUliTEFjUTkzc2hrYTFWTmhlVUFxTjZJY3N6alRHOD0=")

		globalTidalDownloader = &TidalDownloader{
			client:       NewHTTPClientWithTimeout(DefaultTimeout), // 60s timeout
			clientID:     string(clientID),
			clientSecret: string(clientSecret),
		}

		// Get first available API
		apis := globalTidalDownloader.GetAvailableAPIs()
		if len(apis) > 0 {
			globalTidalDownloader.apiURL = apis[0]
		}
	})
	return globalTidalDownloader
}

// GetAvailableAPIs returns list of available Tidal APIs
func (t *TidalDownloader) GetAvailableAPIs() []string {
	encodedAPIs := []string{
		"dm9nZWwucXFkbC5zaXRl",         // API 1 - vogel.qqdl.site
		"bWF1cy5xcWRsLnNpdGU=",         // API 2 - maus.qqdl.site
		"aHVuZC5xcWRsLnNpdGU=",         // API 3 - hund.qqdl.site
		"a2F0emUucXFkbC5zaXRl",         // API 4 - katze.qqdl.site
		"d29sZi5xcWRsLnNpdGU=",         // API 5 - wolf.qqdl.site
		"dGlkYWwua2lub3BsdXMub25saW5l", // API 6 - tidal.kinoplus.online
		"dGlkYWwtYXBpLmJpbmltdW0ub3Jn", // API 7 - tidal-api.binimum.org
		"dHJpdG9uLnNxdWlkLnd0Zg==",     // API 8 - triton.squid.wtf
	}

	var apis []string
	for _, encoded := range encodedAPIs {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		apis = append(apis, "https://"+string(decoded))
	}

	return apis
}

// GetAccessToken gets Tidal access token (with caching)
func (t *TidalDownloader) GetAccessToken() (string, error) {
	t.tokenMu.Lock()
	defer t.tokenMu.Unlock()

	// Return cached token if still valid (with 60s buffer)
	if t.cachedToken != "" && time.Now().Add(60*time.Second).Before(t.tokenExpiresAt) {
		return t.cachedToken, nil
	}

	data := fmt.Sprintf("client_id=%s&grant_type=client_credentials", t.clientID)

	authURL, _ := base64.StdEncoding.DecodeString("aHR0cHM6Ly9hdXRoLnRpZGFsLmNvbS92MS9vYXV0aDIvdG9rZW4=")
	req, err := http.NewRequest("POST", string(authURL), strings.NewReader(data))
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(t.clientID, t.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := DoRequestWithUserAgent(t.client, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to get access token: HTTP %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Cache the token
	t.cachedToken = result.AccessToken
	if result.ExpiresIn > 0 {
		t.tokenExpiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	} else {
		t.tokenExpiresAt = time.Now().Add(55 * time.Minute) // Default 55 min
	}

	return result.AccessToken, nil
}

// GetTidalURLFromSpotify gets Tidal URL from Spotify track ID using SongLink
func (t *TidalDownloader) GetTidalURLFromSpotify(spotifyTrackID string) (string, error) {
	spotifyBase, _ := base64.StdEncoding.DecodeString("aHR0cHM6Ly9vcGVuLnNwb3RpZnkuY29tL3RyYWNrLw==")
	spotifyURL := fmt.Sprintf("%s%s", string(spotifyBase), spotifyTrackID)

	apiBase, _ := base64.StdEncoding.DecodeString("aHR0cHM6Ly9hcGkuc29uZy5saW5rL3YxLWFscGhhLjEvbGlua3M/dXJsPQ==")
	apiURL := fmt.Sprintf("%s%s", string(apiBase), url.QueryEscape(spotifyURL))

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := DoRequestWithUserAgent(t.client, req)
	if err != nil {
		return "", fmt.Errorf("failed to get Tidal URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("SongLink API returned status %d", resp.StatusCode)
	}

	var songLinkResp struct {
		LinksByPlatform map[string]struct {
			URL string `json:"url"`
		} `json:"linksByPlatform"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&songLinkResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	tidalLink, ok := songLinkResp.LinksByPlatform["tidal"]
	if !ok || tidalLink.URL == "" {
		return "", fmt.Errorf("tidal link not found in SongLink")
	}

	return tidalLink.URL, nil
}

// GetTrackIDFromURL extracts track ID from Tidal URL
func (t *TidalDownloader) GetTrackIDFromURL(tidalURL string) (int64, error) {
	parts := strings.Split(tidalURL, "/track/")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid tidal URL format")
	}

	trackIDStr := strings.Split(parts[1], "?")[0]
	trackIDStr = strings.TrimSpace(trackIDStr)

	var trackID int64
	_, err := fmt.Sscanf(trackIDStr, "%d", &trackID)
	if err != nil {
		return 0, fmt.Errorf("failed to parse track ID: %w", err)
	}

	return trackID, nil
}

// GetTrackInfoByID gets track info by Tidal track ID
func (t *TidalDownloader) GetTrackInfoByID(trackID int64) (*TidalTrack, error) {
	token, err := t.GetAccessToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	trackBase, _ := base64.StdEncoding.DecodeString("aHR0cHM6Ly9hcGkudGlkYWwuY29tL3YxL3RyYWNrcy8=")
	trackURL := fmt.Sprintf("%s%d?countryCode=US", string(trackBase), trackID)

	req, err := http.NewRequest("GET", trackURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := DoRequestWithUserAgent(t.client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get track info: HTTP %d", resp.StatusCode)
	}

	var trackInfo TidalTrack
	if err := json.NewDecoder(resp.Body).Decode(&trackInfo); err != nil {
		return nil, err
	}

	return &trackInfo, nil
}


// SearchTrackByISRC searches for a track by ISRC
func (t *TidalDownloader) SearchTrackByISRC(isrc string) (*TidalTrack, error) {
	token, err := t.GetAccessToken()
	if err != nil {
		return nil, err
	}

	searchBase, _ := base64.StdEncoding.DecodeString("aHR0cHM6Ly9hcGkudGlkYWwuY29tL3YxL3NlYXJjaC90cmFja3M/cXVlcnk9")
	searchURL := fmt.Sprintf("%s%s&limit=50&countryCode=US", string(searchBase), url.QueryEscape(isrc))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := DoRequestWithUserAgent(t.client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("search failed: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Items []TidalTrack `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Find exact ISRC match
	for i := range result.Items {
		if result.Items[i].ISRC == isrc {
			return &result.Items[i], nil
		}
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("no tracks found for ISRC: %s", isrc)
	}

	return nil, fmt.Errorf("no exact ISRC match found for: %s", isrc)
}

// normalizeTitle normalizes a track title for comparison (kept for potential future use)
func normalizeTitle(title string) string {
	normalized := strings.ToLower(strings.TrimSpace(title))
	
	// Remove common suffixes in parentheses or brackets
	suffixPatterns := []string{
		" (remaster)", " (remastered)", " (deluxe)", " (deluxe edition)",
		" (bonus track)", " (single)", " (album version)", " (radio edit)",
		" [remaster]", " [remastered]", " [deluxe]", " [bonus track]",
	}
	for _, suffix := range suffixPatterns {
		normalized = strings.TrimSuffix(normalized, suffix)
	}
	
	// Remove multiple spaces
	for strings.Contains(normalized, "  ") {
		normalized = strings.ReplaceAll(normalized, "  ", " ")
	}
	
	return normalized
}

// SearchTrackByMetadataWithISRC searches for a track with ISRC matching priority
func (t *TidalDownloader) SearchTrackByMetadataWithISRC(trackName, artistName, spotifyISRC string, expectedDuration int) (*TidalTrack, error) {
	token, err := t.GetAccessToken()
	if err != nil {
		return nil, err
	}

	// Build search queries - multiple strategies
	queries := []string{}

	// Strategy 1: Artist + Track name (original)
	if artistName != "" && trackName != "" {
		queries = append(queries, artistName+" "+trackName)
	}

	// Strategy 2: Track name only
	if trackName != "" {
		queries = append(queries, trackName)
	}

	// Strategy 3: Artist only as last resort
	if artistName != "" {
		queries = append(queries, artistName)
	}

	searchBase, _ := base64.StdEncoding.DecodeString("aHR0cHM6Ly9hcGkudGlkYWwuY29tL3YxL3NlYXJjaC90cmFja3M/cXVlcnk9")

	// Collect all search results from all queries
	var allTracks []TidalTrack
	searchedQueries := make(map[string]bool)

	for _, query := range queries {
		cleanQuery := strings.TrimSpace(query)
		if cleanQuery == "" || searchedQueries[cleanQuery] {
			continue
		}
		searchedQueries[cleanQuery] = true

		searchURL := fmt.Sprintf("%s%s&limit=100&countryCode=US", string(searchBase), url.QueryEscape(cleanQuery))

		req, err := http.NewRequest("GET", searchURL, nil)
		if err != nil {
			continue
		}

		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := DoRequestWithUserAgent(t.client, req)
		if err != nil {
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			continue
		}

		var result struct {
			Items []TidalTrack `json:"items"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		if len(result.Items) > 0 {
			allTracks = append(allTracks, result.Items...)
		}
	}

	if len(allTracks) == 0 {
		return nil, fmt.Errorf("no tracks found for any search query")
	}

	// Priority 1: Match by ISRC (exact match) WITH title verification
	if spotifyISRC != "" {
		var isrcMatches []*TidalTrack
		for i := range allTracks {
			track := &allTracks[i]
			if track.ISRC == spotifyISRC {
				isrcMatches = append(isrcMatches, track)
			}
		}
		
		if len(isrcMatches) > 0 {
			// Verify duration first (most important check)
			if expectedDuration > 0 {
				var durationVerifiedMatches []*TidalTrack
				for _, track := range isrcMatches {
					durationDiff := track.Duration - expectedDuration
					if durationDiff < 0 {
						durationDiff = -durationDiff
					}
					// Allow 30 seconds tolerance for duration
					if durationDiff <= 30 {
						durationVerifiedMatches = append(durationVerifiedMatches, track)
					}
				}
				
				if len(durationVerifiedMatches) > 0 {
					// Return first duration-verified match
					fmt.Printf("[Tidal] ISRC match with duration verification: '%s' (expected %ds, found %ds)\n", 
						durationVerifiedMatches[0].Title, expectedDuration, durationVerifiedMatches[0].Duration)
					return durationVerifiedMatches[0], nil
				}
				
				// ISRC matches but duration doesn't - this is likely wrong version
				fmt.Printf("[Tidal] WARNING: ISRC %s found but duration mismatch. Expected=%ds, Found=%ds. Rejecting.\n", 
					spotifyISRC, expectedDuration, isrcMatches[0].Duration)
				return nil, fmt.Errorf("ISRC found but duration mismatch: expected %ds, found %ds (likely different version/edit)", 
					expectedDuration, isrcMatches[0].Duration)
			}
			
			// No duration to verify, just return first ISRC match
			fmt.Printf("[Tidal] ISRC match (no duration verification): '%s'\n", isrcMatches[0].Title)
			return isrcMatches[0], nil
		}
		
		// If ISRC was provided but no match found, return error
		return nil, fmt.Errorf("ISRC mismatch: no track found with ISRC %s on Tidal", spotifyISRC)
	}

	// Priority 2: Match by duration (within tolerance) + prefer best quality
	if expectedDuration > 0 {
		tolerance := 3 // 3 seconds tolerance
		var durationMatches []*TidalTrack

		for i := range allTracks {
			track := &allTracks[i]
			durationDiff := track.Duration - expectedDuration
			if durationDiff < 0 {
				durationDiff = -durationDiff
			}
			if durationDiff <= tolerance {
				durationMatches = append(durationMatches, track)
			}
		}

		if len(durationMatches) > 0 {
			// Find best quality among duration matches
			bestMatch := durationMatches[0]
			for _, track := range durationMatches {
				for _, tag := range track.MediaMetadata.Tags {
					if tag == "HIRES_LOSSLESS" {
						bestMatch = track
						break
					}
				}
			}
			return bestMatch, nil
		}
	}

	// Priority 3: Just take the best quality from first results
	bestMatch := &allTracks[0]
	for i := range allTracks {
		track := &allTracks[i]
		for _, tag := range track.MediaMetadata.Tags {
			if tag == "HIRES_LOSSLESS" {
				bestMatch = track
				break
			}
		}
		if bestMatch != &allTracks[0] {
			break
		}
	}

	return bestMatch, nil
}

// SearchTrackByMetadata searches for a track using artist name and track name
func (t *TidalDownloader) SearchTrackByMetadata(trackName, artistName string) (*TidalTrack, error) {
	return t.SearchTrackByMetadataWithISRC(trackName, artistName, "", 0)
}


// TidalDownloadInfo contains download URL and quality info
type TidalDownloadInfo struct {
	URL        string
	BitDepth   int
	SampleRate int
}

// getDownloadURLSequential requests download URL from APIs sequentially
// Returns the first successful result (supports both v1 and v2 API formats)
func getDownloadURLSequential(apis []string, trackID int64, quality string) (string, TidalDownloadInfo, error) {
	if len(apis) == 0 {
		return "", TidalDownloadInfo{}, fmt.Errorf("no APIs available")
	}

	client := NewHTTPClientWithTimeout(DefaultTimeout)
	retryConfig := DefaultRetryConfig()
	var errors []string

	for _, apiURL := range apis {
		reqURL := fmt.Sprintf("%s/track/?id=%d&quality=%s", apiURL, trackID, quality)

		req, err := http.NewRequest("GET", reqURL, nil)
		if err != nil {
			errors = append(errors, BuildErrorMessage(apiURL, 0, err.Error()))
			continue
		}

		resp, err := DoRequestWithRetry(client, req, retryConfig)
		if err != nil {
			errors = append(errors, BuildErrorMessage(apiURL, 0, err.Error()))
			continue
		}

		body, err := ReadResponseBody(resp)
		resp.Body.Close()
		if err != nil {
			errors = append(errors, BuildErrorMessage(apiURL, resp.StatusCode, err.Error()))
			continue
		}

		// Try v2 format first (object with manifest)
		var v2Response TidalAPIResponseV2
		if err := json.Unmarshal(body, &v2Response); err == nil && v2Response.Data.Manifest != "" {
			info := TidalDownloadInfo{
				URL:        "MANIFEST:" + v2Response.Data.Manifest,
				BitDepth:   v2Response.Data.BitDepth,
				SampleRate: v2Response.Data.SampleRate,
			}
			return apiURL, info, nil
		}

		// Fallback to v1 format (array with OriginalTrackUrl)
		var v1Responses []struct {
			OriginalTrackURL string `json:"OriginalTrackUrl"`
		}
		if err := json.Unmarshal(body, &v1Responses); err == nil {
			for _, item := range v1Responses {
				if item.OriginalTrackURL != "" {
					// v1 format doesn't have quality info, assume 16-bit/44.1kHz
					info := TidalDownloadInfo{
						URL:        item.OriginalTrackURL,
						BitDepth:   16,
						SampleRate: 44100,
					}
					return apiURL, info, nil
				}
			}
		}

		errors = append(errors, BuildErrorMessage(apiURL, resp.StatusCode, "no download URL or manifest in response"))
	}

	return "", TidalDownloadInfo{}, fmt.Errorf("all %d Tidal APIs failed. Errors: %v", len(apis), errors)
}

// GetDownloadURL gets download URL for a track - tries APIs sequentially
func (t *TidalDownloader) GetDownloadURL(trackID int64, quality string) (TidalDownloadInfo, error) {
	apis := t.GetAvailableAPIs()
	if len(apis) == 0 {
		return TidalDownloadInfo{}, fmt.Errorf("no API URL configured")
	}

	_, info, err := getDownloadURLSequential(apis, trackID, quality)
	if err != nil {
		return TidalDownloadInfo{}, fmt.Errorf("failed to get download URL: %w", err)
	}

	return info, nil
}

// parseManifest parses Tidal manifest (supports both BTS and DASH formats)
func parseManifest(manifestB64 string) (directURL string, initURL string, mediaURLs []string, err error) {
	manifestBytes, err := base64.StdEncoding.DecodeString(manifestB64)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	manifestStr := string(manifestBytes)

	// Check if it's BTS format (JSON) or DASH format (XML)
	if strings.HasPrefix(manifestStr, "{") {
		// BTS format - JSON with direct URLs
		var btsManifest TidalBTSManifest
		if err := json.Unmarshal(manifestBytes, &btsManifest); err != nil {
			return "", "", nil, fmt.Errorf("failed to parse BTS manifest: %w", err)
		}

		if len(btsManifest.URLs) == 0 {
			return "", "", nil, fmt.Errorf("no URLs in BTS manifest")
		}

		return btsManifest.URLs[0], "", nil, nil
	}

	// DASH format - XML with segments
	var mpd MPD
	if err := xml.Unmarshal(manifestBytes, &mpd); err != nil {
		return "", "", nil, fmt.Errorf("failed to parse manifest XML: %w", err)
	}

	segTemplate := mpd.Period.AdaptationSet.Representation.SegmentTemplate
	initURL = segTemplate.Initialization
	mediaTemplate := segTemplate.Media

	if initURL == "" || mediaTemplate == "" {
		// Fallback: try regex extraction
		initRe := regexp.MustCompile(`initialization="([^"]+)"`)
		mediaRe := regexp.MustCompile(`media="([^"]+)"`)

		if match := initRe.FindStringSubmatch(manifestStr); len(match) > 1 {
			initURL = match[1]
		}
		if match := mediaRe.FindStringSubmatch(manifestStr); len(match) > 1 {
			mediaTemplate = match[1]
		}
	}

	if initURL == "" {
		return "", "", nil, fmt.Errorf("no initialization URL found in manifest")
	}

	// Unescape HTML entities in URLs
	initURL = strings.ReplaceAll(initURL, "&amp;", "&")
	mediaTemplate = strings.ReplaceAll(mediaTemplate, "&amp;", "&")

	// Calculate segment count from timeline
	segmentCount := 0
	for _, seg := range segTemplate.Timeline.Segments {
		segmentCount += seg.Repeat + 1
	}

	// If no segments found via XML, try regex
	if segmentCount == 0 {
		segRe := regexp.MustCompile(`<S d="\d+"(?: r="(\d+)")?`)
		matches := segRe.FindAllStringSubmatch(manifestStr, -1)
		for _, match := range matches {
			repeat := 0
			if len(match) > 1 && match[1] != "" {
				fmt.Sscanf(match[1], "%d", &repeat)
			}
			segmentCount += repeat + 1
		}
	}

	// Generate media URLs for each segment
	for i := 1; i <= segmentCount; i++ {
		mediaURL := strings.ReplaceAll(mediaTemplate, "$Number$", fmt.Sprintf("%d", i))
		mediaURLs = append(mediaURLs, mediaURL)
	}

	return "", initURL, mediaURLs, nil
}


// DownloadFile downloads a file from URL with progress tracking
func (t *TidalDownloader) DownloadFile(downloadURL, outputPath, itemID string) error {
	// Handle manifest-based download
	if strings.HasPrefix(downloadURL, "MANIFEST:") {
		return t.downloadFromManifest(strings.TrimPrefix(downloadURL, "MANIFEST:"), outputPath, itemID)
	}

	// Initialize item progress (required for all downloads)
	if itemID != "" {
		StartItemProgress(itemID)
		defer CompleteItemProgress(itemID)
	}

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := DoRequestWithUserAgent(t.client, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// Set total bytes if available
	if resp.ContentLength > 0 && itemID != "" {
		SetItemBytesTotal(itemID, resp.ContentLength)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Use buffered writer for better performance (256KB buffer)
	bufWriter := bufio.NewWriterSize(out, 256*1024)
	defer bufWriter.Flush()

	// Use item progress writer with buffered output
	if itemID != "" {
		progressWriter := NewItemProgressWriter(bufWriter, itemID)
		_, err = io.Copy(progressWriter, resp.Body)
	} else {
		// Fallback: direct copy without progress tracking
		_, err = io.Copy(bufWriter, resp.Body)
	}
	return err
}

func (t *TidalDownloader) downloadFromManifest(manifestB64, outputPath, itemID string) error {
	directURL, initURL, mediaURLs, err := parseManifest(manifestB64)
	if err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	// If we have a direct URL (BTS format), download directly with progress tracking
	if directURL != "" {
		// Initialize item progress (required for all downloads)
		if itemID != "" {
			StartItemProgress(itemID)
			defer CompleteItemProgress(itemID)
		}

		req, err := http.NewRequest("GET", directURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to download file: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return fmt.Errorf("download failed with status %d", resp.StatusCode)
		}

		// Set total bytes for progress tracking
		if resp.ContentLength > 0 && itemID != "" {
			SetItemBytesTotal(itemID, resp.ContentLength)
		}

		out, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}
		defer out.Close()

		// Use item progress writer
		if itemID != "" {
			progressWriter := NewItemProgressWriter(out, itemID)
			_, err = io.Copy(progressWriter, resp.Body)
		} else {
			// Fallback: direct copy without progress tracking
			_, err = io.Copy(out, resp.Body)
		}
		return err
	}

	// DASH format - download segments to temporary file
	// Note: On Android, we can't use ffmpeg, so we'll try to download as M4A
	// and hope the player can handle it, or we save as .m4a instead of .flac
	tempPath := outputPath + ".m4a.tmp"
	out, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// Download initialization segment
	resp, err := client.Get(initURL)
	if err != nil {
		out.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to download init segment: %w", err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		out.Close()
		os.Remove(tempPath)
		return fmt.Errorf("init segment download failed with status %d", resp.StatusCode)
	}
	_, err = io.Copy(out, resp.Body)
	resp.Body.Close()
	if err != nil {
		out.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to write init segment: %w", err)
	}

	// Download media segments
	for i, mediaURL := range mediaURLs {
		resp, err := client.Get(mediaURL)
		if err != nil {
			out.Close()
			os.Remove(tempPath)
			return fmt.Errorf("failed to download segment %d: %w", i+1, err)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			out.Close()
			os.Remove(tempPath)
			return fmt.Errorf("segment %d download failed with status %d", i+1, resp.StatusCode)
		}
		_, err = io.Copy(out, resp.Body)
		resp.Body.Close()
		if err != nil {
			out.Close()
			os.Remove(tempPath)
			return fmt.Errorf("failed to write segment %d: %w", i+1, err)
		}
	}

	out.Close()

	// For Android, we'll save as M4A since we can't use ffmpeg
	// Rename temp file to final output (change extension to .m4a if needed)
	m4aPath := strings.TrimSuffix(outputPath, ".flac") + ".m4a"
	if err := os.Rename(tempPath, m4aPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	// If the original output was .flac, we need to indicate this is actually m4a
	// For now, we'll just keep it as m4a
	return nil
}

// TidalDownloadResult contains download result with quality info
type TidalDownloadResult struct {
	FilePath   string
	BitDepth   int
	SampleRate int
}

// artistsMatch checks if the artist names are similar enough
func artistsMatch(spotifyArtist, tidalArtist string) bool {
	normSpotify := strings.ToLower(strings.TrimSpace(spotifyArtist))
	normTidal := strings.ToLower(strings.TrimSpace(tidalArtist))
	
	// Exact match
	if normSpotify == normTidal {
		return true
	}
	
	// Check if one contains the other (for cases like "Artist" vs "Artist feat. Someone")
	if strings.Contains(normSpotify, normTidal) || strings.Contains(normTidal, normSpotify) {
		return true
	}
	
	// Check first artist (before comma or feat)
	spotifyFirst := strings.Split(normSpotify, ",")[0]
	spotifyFirst = strings.Split(spotifyFirst, " feat")[0]
	spotifyFirst = strings.Split(spotifyFirst, " ft.")[0]
	spotifyFirst = strings.TrimSpace(spotifyFirst)
	
	tidalFirst := strings.Split(normTidal, ",")[0]
	tidalFirst = strings.Split(tidalFirst, " feat")[0]
	tidalFirst = strings.Split(tidalFirst, " ft.")[0]
	tidalFirst = strings.TrimSpace(tidalFirst)
	
	if spotifyFirst == tidalFirst {
		return true
	}
	
	// Check if first artist is contained in the other
	if strings.Contains(spotifyFirst, tidalFirst) || strings.Contains(tidalFirst, spotifyFirst) {
		return true
	}
	
	// If scripts are different (one is ASCII, one is non-ASCII like Japanese/Chinese/Korean),
	// assume they're the same artist with different transliteration
	// This handles cases like "鈴木雅之" vs "Masayuki Suzuki"
	spotifyASCII := isASCIIString(spotifyArtist)
	tidalASCII := isASCIIString(tidalArtist)
	if spotifyASCII != tidalASCII {
		fmt.Printf("[Tidal] Artist names in different scripts, assuming match: '%s' vs '%s'\n", spotifyArtist, tidalArtist)
		return true
	}
	
	return false
}

// isASCIIString checks if a string contains only ASCII characters
func isASCIIString(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// downloadFromTidal downloads a track using the request parameters
func downloadFromTidal(req DownloadRequest) (TidalDownloadResult, error) {
	downloader := NewTidalDownloader()

	// Check for existing file first
	if existingFile, exists := checkISRCExistsInternal(req.OutputDir, req.ISRC); exists {
		return TidalDownloadResult{FilePath: "EXISTS:" + existingFile}, nil
	}

	// Convert expected duration from ms to seconds
	expectedDurationSec := req.DurationMS / 1000

	var track *TidalTrack
	var err error

	// OPTIMIZATION: Check cache first for track ID
	if req.ISRC != "" {
		if cached := GetTrackIDCache().Get(req.ISRC); cached != nil && cached.TidalTrackID > 0 {
			fmt.Printf("[Tidal] Cache hit! Using cached track ID: %d\n", cached.TidalTrackID)
			track, err = downloader.GetTrackInfoByID(cached.TidalTrackID)
			if err != nil {
				fmt.Printf("[Tidal] Cache hit but failed to get track info: %v\n", err)
				track = nil // Fall through to normal search
			}
		}
	}

	// OPTIMIZED: Try ISRC search first (faster than SongLink API)
	// Strategy 1: Search by ISRC with duration verification (FASTEST)
	if track == nil && req.ISRC != "" {
		fmt.Printf("[Tidal] Trying ISRC search first (faster): %s\n", req.ISRC)
		track, err = downloader.SearchTrackByMetadataWithISRC(req.TrackName, req.ArtistName, req.ISRC, expectedDurationSec)
		// Verify artist for ISRC match
		if track != nil {
			tidalArtist := track.Artist.Name
			if len(track.Artists) > 0 {
				var artistNames []string
				for _, a := range track.Artists {
					artistNames = append(artistNames, a.Name)
				}
				tidalArtist = strings.Join(artistNames, ", ")
			}
			if !artistsMatch(req.ArtistName, tidalArtist) {
				fmt.Printf("[Tidal] Artist mismatch from ISRC search: expected '%s', got '%s'. Rejecting.\n", 
					req.ArtistName, tidalArtist)
				track = nil
			}
		}
	}

	// Strategy 2: Try SongLink only if ISRC search failed (slower but more accurate)
	if track == nil && req.SpotifyID != "" {
		fmt.Printf("[Tidal] ISRC search failed, trying SongLink...\n")
		tidalURL, slErr := downloader.GetTidalURLFromSpotify(req.SpotifyID)
		if slErr == nil && tidalURL != "" {
			// Extract track ID and get track info
			trackID, idErr := downloader.GetTrackIDFromURL(tidalURL)
			if idErr == nil {
				track, err = downloader.GetTrackInfoByID(trackID)
				if track != nil {
					// Get artist name from track
					tidalArtist := track.Artist.Name
					if len(track.Artists) > 0 {
						var artistNames []string
						for _, a := range track.Artists {
							artistNames = append(artistNames, a.Name)
						}
						tidalArtist = strings.Join(artistNames, ", ")
					}
					
					// Verify artist matches
					if !artistsMatch(req.ArtistName, tidalArtist) {
						fmt.Printf("[Tidal] Artist mismatch from SongLink: expected '%s', got '%s'. Rejecting.\n", 
							req.ArtistName, tidalArtist)
						track = nil
					}
					
					// Verify duration if we have expected duration
					if track != nil && expectedDurationSec > 0 {
						durationDiff := track.Duration - expectedDurationSec
						if durationDiff < 0 {
							durationDiff = -durationDiff
						}
						// Allow 30 seconds tolerance
						if durationDiff > 30 {
							fmt.Printf("[Tidal] Duration mismatch from SongLink: expected %ds, got %ds. Rejecting.\n", 
								expectedDurationSec, track.Duration)
							track = nil // Reject this match
						}
					}
				}
			}
		}
	}

	// Strategy 3: Search by metadata only (no ISRC requirement) - last resort
	if track == nil {
		fmt.Printf("[Tidal] Trying metadata search as last resort...\n")
		track, err = downloader.SearchTrackByMetadataWithISRC(req.TrackName, req.ArtistName, "", expectedDurationSec)
		// Verify artist for metadata search too
		if track != nil {
			tidalArtist := track.Artist.Name
			if len(track.Artists) > 0 {
				var artistNames []string
				for _, a := range track.Artists {
					artistNames = append(artistNames, a.Name)
				}
				tidalArtist = strings.Join(artistNames, ", ")
			}
			if !artistsMatch(req.ArtistName, tidalArtist) {
				fmt.Printf("[Tidal] Artist mismatch from metadata search: expected '%s', got '%s'. Rejecting.\n", 
					req.ArtistName, tidalArtist)
				track = nil
			}
		}
	}

	if track == nil {
		errMsg := "could not find matching track on Tidal (artist/duration mismatch)"
		if err != nil {
			errMsg = err.Error()
		}
		return TidalDownloadResult{}, fmt.Errorf("tidal search failed: %s", errMsg)
	}

	// Final verification logging
	tidalArtist := track.Artist.Name
	if len(track.Artists) > 0 {
		var artistNames []string
		for _, a := range track.Artists {
			artistNames = append(artistNames, a.Name)
		}
		tidalArtist = strings.Join(artistNames, ", ")
	}
	fmt.Printf("[Tidal] Match found: '%s' by '%s' (duration: %ds)\n", track.Title, tidalArtist, track.Duration)

	// Cache the track ID for future use
	if req.ISRC != "" {
		GetTrackIDCache().SetTidal(req.ISRC, track.ID)
	}

	// Build filename
	filename := buildFilenameFromTemplate(req.FilenameFormat, map[string]interface{}{
		"title":  req.TrackName,
		"artist": req.ArtistName,
		"album":  req.AlbumName,
		"track":  req.TrackNumber,
		"year":   extractYear(req.ReleaseDate),
		"disc":   req.DiscNumber,
	})
	filename = sanitizeFilename(filename) + ".flac"
	outputPath := filepath.Join(req.OutputDir, filename)

	// Check if file already exists
	if fileInfo, statErr := os.Stat(outputPath); statErr == nil && fileInfo.Size() > 0 {
		return TidalDownloadResult{FilePath: "EXISTS:" + outputPath}, nil
	}

	// Determine quality to use (default to LOSSLESS if not specified)
	quality := req.Quality
	if quality == "" {
		quality = "LOSSLESS"
	}
	fmt.Printf("[Tidal] Using quality: %s\n", quality)

	// Get download URL using parallel API requests
	downloadInfo, err := downloader.GetDownloadURL(track.ID, quality)
	if err != nil {
		return TidalDownloadResult{}, fmt.Errorf("failed to get download URL: %w", err)
	}

	// Log actual quality received
	fmt.Printf("[Tidal] Actual quality: %d-bit/%dHz\n", downloadInfo.BitDepth, downloadInfo.SampleRate)

	// START PARALLEL: Fetch cover and lyrics while downloading audio
	var parallelResult *ParallelDownloadResult
	parallelDone := make(chan struct{})
	go func() {
		defer close(parallelDone)
		parallelResult = FetchCoverAndLyricsParallel(
			req.CoverURL,
			req.EmbedMaxQualityCover,
			req.SpotifyID,
			req.TrackName,
			req.ArtistName,
			req.EmbedLyrics,
		)
	}()

	// Download audio file with item ID for progress tracking
	if err := downloader.DownloadFile(downloadInfo.URL, outputPath, req.ItemID); err != nil {
		return TidalDownloadResult{}, fmt.Errorf("download failed: %w", err)
	}

	// Wait for parallel operations to complete
	<-parallelDone

	// Set progress to 100% and status to finalizing (before embedding)
	// This makes the UI show "Finalizing..." while embedding happens
	if req.ItemID != "" {
		SetItemProgress(req.ItemID, 1.0, 0, 0)
		SetItemFinalizing(req.ItemID)
	}

	// Check if file was saved as M4A (DASH stream) instead of FLAC
	// downloadFromManifest saves DASH streams as .m4a
	actualOutputPath := outputPath
	m4aPath := strings.TrimSuffix(outputPath, ".flac") + ".m4a"
	if _, err := os.Stat(m4aPath); err == nil {
		// File was saved as M4A, use that path
		actualOutputPath = m4aPath
		fmt.Printf("[Tidal] File saved as M4A (DASH stream): %s\n", actualOutputPath)
	} else if _, err := os.Stat(outputPath); err != nil {
		// Neither FLAC nor M4A exists
		return TidalDownloadResult{}, fmt.Errorf("download completed but file not found at %s or %s", outputPath, m4aPath)
	}

	// Embed metadata using parallel-fetched cover data
	metadata := Metadata{
		Title:       req.TrackName,
		Artist:      req.ArtistName,
		Album:       req.AlbumName,
		AlbumArtist: req.AlbumArtist,
		Date:        req.ReleaseDate,
		TrackNumber: req.TrackNumber,
		TotalTracks: req.TotalTracks,
		DiscNumber:  req.DiscNumber,
		ISRC:        req.ISRC,
	}

	// Use cover data from parallel fetch
	var coverData []byte
	if parallelResult != nil && parallelResult.CoverData != nil {
		coverData = parallelResult.CoverData
		fmt.Printf("[Tidal] Using parallel-fetched cover (%d bytes)\n", len(coverData))
	}

	// Only embed metadata to FLAC files (M4A will be converted by Flutter)
	if strings.HasSuffix(actualOutputPath, ".flac") {
		if err := EmbedMetadataWithCoverData(actualOutputPath, metadata, coverData); err != nil {
			fmt.Printf("Warning: failed to embed metadata: %v\n", err)
		}

		// Embed lyrics from parallel fetch
		if req.EmbedLyrics && parallelResult != nil && parallelResult.LyricsLRC != "" {
			fmt.Printf("[Tidal] Embedding parallel-fetched lyrics (%d lines)...\n", len(parallelResult.LyricsData.Lines))
			if embedErr := EmbedLyrics(actualOutputPath, parallelResult.LyricsLRC); embedErr != nil {
				fmt.Printf("[Tidal] Warning: failed to embed lyrics: %v\n", embedErr)
			} else {
				fmt.Println("[Tidal] Lyrics embedded successfully")
			}
		} else if req.EmbedLyrics {
			fmt.Println("[Tidal] No lyrics available from parallel fetch")
		}
	} else {
		fmt.Printf("[Tidal] Skipping metadata embed for M4A file (will be handled after conversion): %s\n", actualOutputPath)
	}

	return TidalDownloadResult{
		FilePath:   actualOutputPath,
		BitDepth:   downloadInfo.BitDepth,
		SampleRate: downloadInfo.SampleRate,
	}, nil
}
