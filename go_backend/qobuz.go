package gobackend

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// QobuzDownloader handles Qobuz downloads
type QobuzDownloader struct {
	client *http.Client
	appID  string
	apiURL string
}

var (
	// Global Qobuz downloader instance for connection reuse
	globalQobuzDownloader *QobuzDownloader
	qobuzDownloaderOnce   sync.Once
)

// QobuzTrack represents a Qobuz track
type QobuzTrack struct {
	ID                  int64   `json:"id"`
	Title               string  `json:"title"`
	ISRC                string  `json:"isrc"`
	Duration            int     `json:"duration"`
	TrackNumber         int     `json:"track_number"`
	MaximumBitDepth     int     `json:"maximum_bit_depth"`
	MaximumSamplingRate float64 `json:"maximum_sampling_rate"`
	Album               struct {
		Title       string `json:"title"`
		ReleaseDate string `json:"release_date_original"`
		Image       struct {
			Large string `json:"large"`
		} `json:"image"`
	} `json:"album"`
	Performer struct {
		Name string `json:"name"`
	} `json:"performer"`
}

// qobuzArtistsMatch checks if the artist names are similar enough
func qobuzArtistsMatch(expectedArtist, foundArtist string) bool {
	normExpected := strings.ToLower(strings.TrimSpace(expectedArtist))
	normFound := strings.ToLower(strings.TrimSpace(foundArtist))
	
	// Exact match
	if normExpected == normFound {
		return true
	}
	
	// Check if one contains the other
	if strings.Contains(normExpected, normFound) || strings.Contains(normFound, normExpected) {
		return true
	}
	
	// Check first artist (before comma or feat)
	expectedFirst := strings.Split(normExpected, ",")[0]
	expectedFirst = strings.Split(expectedFirst, " feat")[0]
	expectedFirst = strings.Split(expectedFirst, " ft.")[0]
	expectedFirst = strings.TrimSpace(expectedFirst)
	
	foundFirst := strings.Split(normFound, ",")[0]
	foundFirst = strings.Split(foundFirst, " feat")[0]
	foundFirst = strings.Split(foundFirst, " ft.")[0]
	foundFirst = strings.TrimSpace(foundFirst)
	
	if expectedFirst == foundFirst {
		return true
	}
	
	// Check if first artist is contained in the other
	if strings.Contains(expectedFirst, foundFirst) || strings.Contains(foundFirst, expectedFirst) {
		return true
	}
	
	// If scripts are different (one is ASCII, one is non-ASCII like Japanese/Chinese/Korean),
	// assume they're the same artist with different transliteration
	expectedASCII := qobuzIsASCIIString(expectedArtist)
	foundASCII := qobuzIsASCIIString(foundArtist)
	if expectedASCII != foundASCII {
		fmt.Printf("[Qobuz] Artist names in different scripts, assuming match: '%s' vs '%s'\n", expectedArtist, foundArtist)
		return true
	}
	
	return false
}

// qobuzIsASCIIString checks if a string contains only ASCII characters
func qobuzIsASCIIString(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// NewQobuzDownloader creates a new Qobuz downloader (returns singleton for connection reuse)
func NewQobuzDownloader() *QobuzDownloader {
	qobuzDownloaderOnce.Do(func() {
		globalQobuzDownloader = &QobuzDownloader{
			client: NewHTTPClientWithTimeout(DefaultTimeout), // 60s timeout
			appID:  "798273057",
		}
	})
	return globalQobuzDownloader
}

// GetAvailableAPIs returns list of available Qobuz APIs
// Uses same APIs as PC version for compatibility
func (q *QobuzDownloader) GetAvailableAPIs() []string {
	// Same APIs as PC version (referensi/backend/qobuz.go)
	// Primary: dab.yeet.su, Fallback: dabmusic.xyz
	encodedAPIs := []string{
		"ZGFiLnllZXQuc3UvYXBpL3N0cmVhbT90cmFja0lkPQ==",     // dab.yeet.su/api/stream?trackId= (PRIMARY - same as PC)
		"ZGFibXVzaWMueHl6L2FwaS9zdHJlYW0/dHJhY2tJZD0=",     // dabmusic.xyz/api/stream?trackId= (FALLBACK - same as PC)
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

// SearchTrackByISRC searches for a track by ISRC
func (q *QobuzDownloader) SearchTrackByISRC(isrc string) (*QobuzTrack, error) {
	apiBase, _ := base64.StdEncoding.DecodeString("aHR0cHM6Ly93d3cucW9idXouY29tL2FwaS5qc29uLzAuMi90cmFjay9zZWFyY2g/cXVlcnk9")
	searchURL := fmt.Sprintf("%s%s&limit=50&app_id=%s", string(apiBase), url.QueryEscape(isrc), q.appID)

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := DoRequestWithUserAgent(q.client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("search failed: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Tracks struct {
			Items []QobuzTrack `json:"items"`
		} `json:"tracks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Find exact ISRC match
	for i := range result.Tracks.Items {
		if result.Tracks.Items[i].ISRC == isrc {
			return &result.Tracks.Items[i], nil
		}
	}

	if len(result.Tracks.Items) == 0 {
		return nil, fmt.Errorf("no tracks found for ISRC: %s", isrc)
	}

	return nil, fmt.Errorf("no exact ISRC match found for: %s", isrc)
}

// SearchTrackByISRCWithTitle searches for a track by ISRC with duration verification
// expectedDurationSec is the expected duration in seconds (0 to skip verification)
func (q *QobuzDownloader) SearchTrackByISRCWithDuration(isrc string, expectedDurationSec int) (*QobuzTrack, error) {
	apiBase, _ := base64.StdEncoding.DecodeString("aHR0cHM6Ly93d3cucW9idXouY29tL2FwaS5qc29uLzAuMi90cmFjay9zZWFyY2g/cXVlcnk9")
	searchURL := fmt.Sprintf("%s%s&limit=50&app_id=%s", string(apiBase), url.QueryEscape(isrc), q.appID)

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := DoRequestWithUserAgent(q.client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("search failed: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Tracks struct {
			Items []QobuzTrack `json:"items"`
		} `json:"tracks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Find ISRC matches
	var isrcMatches []*QobuzTrack
	for i := range result.Tracks.Items {
		if result.Tracks.Items[i].ISRC == isrc {
			isrcMatches = append(isrcMatches, &result.Tracks.Items[i])
		}
	}

	if len(isrcMatches) > 0 {
		// Verify duration if provided
		if expectedDurationSec > 0 {
			var durationVerifiedMatches []*QobuzTrack
			for _, track := range isrcMatches {
				durationDiff := track.Duration - expectedDurationSec
				if durationDiff < 0 {
					durationDiff = -durationDiff
				}
				// Allow 30 seconds tolerance
				if durationDiff <= 30 {
					durationVerifiedMatches = append(durationVerifiedMatches, track)
				}
			}
			
			if len(durationVerifiedMatches) > 0 {
				fmt.Printf("[Qobuz] ISRC match with duration verification: '%s' (expected %ds, found %ds)\n", 
					durationVerifiedMatches[0].Title, expectedDurationSec, durationVerifiedMatches[0].Duration)
				return durationVerifiedMatches[0], nil
			}
			
			// ISRC matches but duration doesn't
			fmt.Printf("[Qobuz] WARNING: ISRC %s found but duration mismatch. Expected=%ds, Found=%ds. Rejecting.\n", 
				isrc, expectedDurationSec, isrcMatches[0].Duration)
			return nil, fmt.Errorf("ISRC found but duration mismatch: expected %ds, found %ds (likely different version)", 
				expectedDurationSec, isrcMatches[0].Duration)
		}
		
		// No duration to verify, return first match
		fmt.Printf("[Qobuz] ISRC match (no duration verification): '%s'\n", isrcMatches[0].Title)
		return isrcMatches[0], nil
	}

	if len(result.Tracks.Items) == 0 {
		return nil, fmt.Errorf("no tracks found for ISRC: %s", isrc)
	}

	return nil, fmt.Errorf("no exact ISRC match found for: %s", isrc)
}

// SearchTrackByISRCWithTitle is deprecated, use SearchTrackByISRCWithDuration instead
func (q *QobuzDownloader) SearchTrackByISRCWithTitle(isrc, expectedTitle string) (*QobuzTrack, error) {
	return q.SearchTrackByISRCWithDuration(isrc, 0)
}

// SearchTrackByMetadata searches for a track using artist name and track name
func (q *QobuzDownloader) SearchTrackByMetadata(trackName, artistName string) (*QobuzTrack, error) {
	return q.SearchTrackByMetadataWithDuration(trackName, artistName, 0)
}

// SearchTrackByMetadataWithDuration searches for a track with duration verification
func (q *QobuzDownloader) SearchTrackByMetadataWithDuration(trackName, artistName string, expectedDurationSec int) (*QobuzTrack, error) {
	apiBase, _ := base64.StdEncoding.DecodeString("aHR0cHM6Ly93d3cucW9idXouY29tL2FwaS5qc29uLzAuMi90cmFjay9zZWFyY2g/cXVlcnk9")

	// Try multiple search strategies
	queries := []string{}

	// Strategy 1: Artist + Track name
	if artistName != "" && trackName != "" {
		queries = append(queries, artistName+" "+trackName)
	}

	// Strategy 2: Track name only
	if trackName != "" {
		queries = append(queries, trackName)
	}

	var allTracks []QobuzTrack

	for _, query := range queries {
		searchURL := fmt.Sprintf("%s%s&limit=50&app_id=%s", string(apiBase), url.QueryEscape(query), q.appID)

		req, err := http.NewRequest("GET", searchURL, nil)
		if err != nil {
			continue
		}

		resp, err := DoRequestWithUserAgent(q.client, req)
		if err != nil {
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			continue
		}

		var result struct {
			Tracks struct {
				Items []QobuzTrack `json:"items"`
			} `json:"tracks"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		if len(result.Tracks.Items) > 0 {
			allTracks = append(allTracks, result.Tracks.Items...)
		}
	}

	if len(allTracks) == 0 {
		return nil, fmt.Errorf("no tracks found for: %s - %s", artistName, trackName)
	}

	// If duration verification is requested
	if expectedDurationSec > 0 {
		var durationMatches []*QobuzTrack
		for i := range allTracks {
			track := &allTracks[i]
			durationDiff := track.Duration - expectedDurationSec
			if durationDiff < 0 {
				durationDiff = -durationDiff
			}
			if durationDiff <= 30 {
				durationMatches = append(durationMatches, track)
			}
		}

		if len(durationMatches) > 0 {
			// Return best quality among duration matches
			for _, track := range durationMatches {
				if track.MaximumBitDepth >= 24 {
					return track, nil
				}
			}
			return durationMatches[0], nil
		}

		// No duration match found
		return nil, fmt.Errorf("no tracks found with matching duration (expected %ds)", expectedDurationSec)
	}

	// No duration verification, return best quality
	for i := range allTracks {
		track := &allTracks[i]
		if track.MaximumBitDepth >= 24 {
			return track, nil
		}
	}
	return &allTracks[0], nil
}

// getQobuzDownloadURLSequential requests download URL from APIs sequentially
// Uses same URL format as PC version: /api/stream?trackId={id}&quality={quality}
func getQobuzDownloadURLSequential(apis []string, trackID int64, quality string) (string, string, error) {
	if len(apis) == 0 {
		return "", "", fmt.Errorf("no APIs available")
	}

	client := NewHTTPClientWithTimeout(DefaultTimeout)
	retryConfig := DefaultRetryConfig()
	var errors []string

	for _, apiURL := range apis {
		// All APIs now use same format: https://domain/api/stream?trackId={id}&quality={quality}
		// The apiURL already includes the path, just append trackID and quality
		reqURL := fmt.Sprintf("%s%d&quality=%s", apiURL, trackID, quality)

		fmt.Printf("[Qobuz] Trying: %s\n", reqURL)

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

		// Check if response is HTML (error page)
		if len(body) > 0 && body[0] == '<' {
			errors = append(errors, BuildErrorMessage(apiURL, resp.StatusCode, "received HTML instead of JSON"))
			continue
		}

		// Check for error in JSON response
		var errorResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errorResp) == nil && errorResp.Error != "" {
			errors = append(errors, BuildErrorMessage(apiURL, resp.StatusCode, errorResp.Error))
			continue
		}

		var result struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			errors = append(errors, BuildErrorMessage(apiURL, resp.StatusCode, "invalid JSON: "+err.Error()))
			continue
		}

		if result.URL != "" {
			fmt.Printf("[Qobuz] Got download URL from: %s\n", apiURL)
			return apiURL, result.URL, nil
		}

		errors = append(errors, BuildErrorMessage(apiURL, resp.StatusCode, "no download URL in response"))
	}

	return "", "", fmt.Errorf("all %d Qobuz APIs failed. Errors: %v", len(apis), errors)
}

// GetDownloadURL gets download URL for a track - tries APIs sequentially
func (q *QobuzDownloader) GetDownloadURL(trackID int64, quality string) (string, error) {
	apis := q.GetAvailableAPIs()
	if len(apis) == 0 {
		return "", fmt.Errorf("no Qobuz API available")
	}

	_, downloadURL, err := getQobuzDownloadURLSequential(apis, trackID, quality)
	if err != nil {
		return "", err
	}

	return downloadURL, nil
}

// DownloadFile downloads a file from URL with User-Agent and progress tracking
func (q *QobuzDownloader) DownloadFile(downloadURL, outputPath, itemID string) error {
	// Initialize item progress (required for all downloads)
	if itemID != "" {
		StartItemProgress(itemID)
		defer CompleteItemProgress(itemID)
	}

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := DoRequestWithUserAgent(q.client, req)
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

// QobuzDownloadResult contains download result with quality info
type QobuzDownloadResult struct {
	FilePath   string
	BitDepth   int
	SampleRate int
}

// downloadFromQobuz downloads a track using the request parameters
func downloadFromQobuz(req DownloadRequest) (QobuzDownloadResult, error) {
	downloader := NewQobuzDownloader()

	// Check for existing file first
	if existingFile, exists := checkISRCExistsInternal(req.OutputDir, req.ISRC); exists {
		return QobuzDownloadResult{FilePath: "EXISTS:" + existingFile}, nil
	}

	// Convert expected duration from ms to seconds
	expectedDurationSec := req.DurationMS / 1000

	var track *QobuzTrack
	var err error

	// OPTIMIZATION: Check cache first for track ID
	if req.ISRC != "" {
		if cached := GetTrackIDCache().Get(req.ISRC); cached != nil && cached.QobuzTrackID > 0 {
			fmt.Printf("[Qobuz] Cache hit! Using cached track ID: %d\n", cached.QobuzTrackID)
			// For Qobuz we need to search again to get full track info, but we can use the ID
			track, err = downloader.SearchTrackByISRC(req.ISRC)
			if err != nil {
				fmt.Printf("[Qobuz] Cache hit but search failed: %v\n", err)
				track = nil
			}
		}
	}

	// Strategy 1: Search by ISRC with duration verification
	if track == nil && req.ISRC != "" {
		track, err = downloader.SearchTrackByISRCWithDuration(req.ISRC, expectedDurationSec)
		// Verify artist
		if track != nil && !qobuzArtistsMatch(req.ArtistName, track.Performer.Name) {
			fmt.Printf("[Qobuz] Artist mismatch from ISRC search: expected '%s', got '%s'. Rejecting.\n", 
				req.ArtistName, track.Performer.Name)
			track = nil
		}
	}

	// Strategy 2: Search by metadata with duration verification
	if track == nil {
		track, err = downloader.SearchTrackByMetadataWithDuration(req.TrackName, req.ArtistName, expectedDurationSec)
		// Verify artist
		if track != nil && !qobuzArtistsMatch(req.ArtistName, track.Performer.Name) {
			fmt.Printf("[Qobuz] Artist mismatch from metadata search: expected '%s', got '%s'. Rejecting.\n", 
				req.ArtistName, track.Performer.Name)
			track = nil
		}
	}

	if track == nil {
		errMsg := "could not find matching track on Qobuz (artist/duration mismatch)"
		if err != nil {
			errMsg = err.Error()
		}
		return QobuzDownloadResult{}, fmt.Errorf("qobuz search failed: %s", errMsg)
	}

	// Log match found and cache the track ID
	fmt.Printf("[Qobuz] Match found: '%s' by '%s' (duration: %ds)\n", track.Title, track.Performer.Name, track.Duration)
	if req.ISRC != "" {
		GetTrackIDCache().SetQobuz(req.ISRC, track.ID)
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
		return QobuzDownloadResult{FilePath: "EXISTS:" + outputPath}, nil
	}

	// Map quality from Tidal format to Qobuz format
	// Tidal: LOSSLESS (16-bit), HI_RES (24-bit), HI_RES_LOSSLESS (24-bit hi-res)
	// Qobuz: 5 (MP3 320), 6 (16-bit), 7 (24-bit 96kHz), 27 (24-bit 192kHz)
	qobuzQuality := "27" // Default to highest quality
	switch req.Quality {
	case "LOSSLESS":
		qobuzQuality = "6" // 16-bit FLAC
	case "HI_RES":
		qobuzQuality = "7" // 24-bit 96kHz
	case "HI_RES_LOSSLESS":
		qobuzQuality = "27" // 24-bit 192kHz
	}
	fmt.Printf("[Qobuz] Using quality: %s (mapped from %s)\n", qobuzQuality, req.Quality)

	// Get actual quality from track metadata
	actualBitDepth := track.MaximumBitDepth
	actualSampleRate := int(track.MaximumSamplingRate * 1000) // Convert kHz to Hz
	fmt.Printf("[Qobuz] Actual quality: %d-bit/%.1fkHz\n", actualBitDepth, track.MaximumSamplingRate)

	// Get download URL using parallel API requests
	downloadURL, err := downloader.GetDownloadURL(track.ID, qobuzQuality)
	if err != nil {
		return QobuzDownloadResult{}, fmt.Errorf("failed to get download URL: %w", err)
	}

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
	if err := downloader.DownloadFile(downloadURL, outputPath, req.ItemID); err != nil {
		return QobuzDownloadResult{}, fmt.Errorf("download failed: %w", err)
	}

	// Wait for parallel operations to complete
	<-parallelDone

	// Set progress to 100% and status to finalizing (before embedding)
	// This makes the UI show "Finalizing..." while embedding happens
	if req.ItemID != "" {
		SetItemProgress(req.ItemID, 1.0, 0, 0)
		SetItemFinalizing(req.ItemID)
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
		fmt.Printf("[Qobuz] Using parallel-fetched cover (%d bytes)\n", len(coverData))
	}

	if err := EmbedMetadataWithCoverData(outputPath, metadata, coverData); err != nil {
		fmt.Printf("Warning: failed to embed metadata: %v\n", err)
	}

	// Embed lyrics from parallel fetch
	if req.EmbedLyrics && parallelResult != nil && parallelResult.LyricsLRC != "" {
		fmt.Printf("[Qobuz] Embedding parallel-fetched lyrics (%d lines)...\n", len(parallelResult.LyricsData.Lines))
		if embedErr := EmbedLyrics(outputPath, parallelResult.LyricsLRC); embedErr != nil {
			fmt.Printf("[Qobuz] Warning: failed to embed lyrics: %v\n", embedErr)
		} else {
			fmt.Println("[Qobuz] Lyrics embedded successfully")
		}
	} else if req.EmbedLyrics {
		fmt.Println("[Qobuz] No lyrics available from parallel fetch")
	}

	return QobuzDownloadResult{
		FilePath:   outputPath,
		BitDepth:   actualBitDepth,
		SampleRate: actualSampleRate,
	}, nil
}
