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
	"time"
)

// AmazonDownloader handles Amazon Music downloads using DoubleDouble service (same as PC)
type AmazonDownloader struct {
	client  *http.Client
	regions []string // us, eu regions for DoubleDouble service
}

var (
	// Global Amazon downloader instance for connection reuse
	globalAmazonDownloader *AmazonDownloader
	amazonDownloaderOnce   sync.Once
)

// DoubleDoubleSubmitResponse is the response from DoubleDouble submit endpoint
type DoubleDoubleSubmitResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
}

// DoubleDoubleStatusResponse is the response from DoubleDouble status endpoint
type DoubleDoubleStatusResponse struct {
	Status         string `json:"status"`
	FriendlyStatus string `json:"friendlyStatus"`
	URL            string `json:"url"`
	Current        struct {
		Name   string `json:"name"`
		Artist string `json:"artist"`
	} `json:"current"`
}

// amazonArtistsMatch checks if the artist names are similar enough
func amazonArtistsMatch(expectedArtist, foundArtist string) bool {
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
	expectedASCII := amazonIsASCIIString(expectedArtist)
	foundASCII := amazonIsASCIIString(foundArtist)
	if expectedASCII != foundASCII {
		fmt.Printf("[Amazon] Artist names in different scripts, assuming match: '%s' vs '%s'\n", expectedArtist, foundArtist)
		return true
	}
	
	return false
}

// amazonIsASCIIString checks if a string contains only ASCII characters
func amazonIsASCIIString(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// NewAmazonDownloader creates a new Amazon downloader (returns singleton for connection reuse)
func NewAmazonDownloader() *AmazonDownloader {
	amazonDownloaderOnce.Do(func() {
		globalAmazonDownloader = &AmazonDownloader{
			client:  NewHTTPClientWithTimeout(120 * time.Second), // 120s timeout like PC
			regions: []string{"us", "eu"},                        // Same regions as PC
		}
	})
	return globalAmazonDownloader
}

// GetAvailableAPIs returns list of available DoubleDouble regions
// Uses same service as PC version (doubledouble.top)
func (a *AmazonDownloader) GetAvailableAPIs() []string {
	// DoubleDouble service regions (same as PC)
	// Format: https://{region}.doubledouble.top
	var apis []string
	for _, region := range a.regions {
		apis = append(apis, fmt.Sprintf("https://%s.doubledouble.top", region))
	}
	return apis
}


// downloadFromDoubleDoubleService downloads a track using DoubleDouble service (same as PC)
// This uses submit → poll → download mechanism
// Internal function - not exported to gomobile
func (a *AmazonDownloader) downloadFromDoubleDoubleService(amazonURL, outputDir string) (string, string, string, error) {
	var lastError error

	for _, region := range a.regions {
		fmt.Printf("[Amazon] Trying region: %s...\n", region)

		// Build base URL for DoubleDouble service
		// Decode base64 service URL (same as PC)
		serviceBase, _ := base64.StdEncoding.DecodeString("aHR0cHM6Ly8=")         // https://
		serviceDomain, _ := base64.StdEncoding.DecodeString("LmRvdWJsZWRvdWJsZS50b3A=") // .doubledouble.top
		baseURL := fmt.Sprintf("%s%s%s", string(serviceBase), region, string(serviceDomain))

		// Step 1: Submit download request
		encodedURL := url.QueryEscape(amazonURL)
		submitURL := fmt.Sprintf("%s/dl?url=%s", baseURL, encodedURL)

		req, err := http.NewRequest("GET", submitURL, nil)
		if err != nil {
			lastError = fmt.Errorf("failed to create request: %w", err)
			continue
		}

		req.Header.Set("User-Agent", getRandomUserAgent())

		fmt.Println("[Amazon] Submitting download request...")
		resp, err := a.client.Do(req)
		if err != nil {
			lastError = fmt.Errorf("failed to submit request: %w", err)
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			lastError = fmt.Errorf("submit failed with status %d", resp.StatusCode)
			continue
		}

		var submitResp DoubleDoubleSubmitResponse
		if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
			resp.Body.Close()
			lastError = fmt.Errorf("failed to decode submit response: %w", err)
			continue
		}
		resp.Body.Close()

		if !submitResp.Success || submitResp.ID == "" {
			lastError = fmt.Errorf("submit request failed")
			continue
		}

		downloadID := submitResp.ID
		fmt.Printf("[Amazon] Download ID: %s\n", downloadID)

		// Step 2: Poll for completion
		statusURL := fmt.Sprintf("%s/dl/%s", baseURL, downloadID)
		fmt.Println("[Amazon] Waiting for download to complete...")

		maxWait := 300 * time.Second // 5 minutes max wait
		elapsed := time.Duration(0)
		pollInterval := 3 * time.Second

		for elapsed < maxWait {
			time.Sleep(pollInterval)
			elapsed += pollInterval

			statusReq, err := http.NewRequest("GET", statusURL, nil)
			if err != nil {
				continue
			}

			statusReq.Header.Set("User-Agent", getRandomUserAgent())

			statusResp, err := a.client.Do(statusReq)
			if err != nil {
				fmt.Printf("\r[Amazon] Status check failed, retrying...")
				continue
			}

			if statusResp.StatusCode != 200 {
				statusResp.Body.Close()
				fmt.Printf("\r[Amazon] Status check failed (status %d), retrying...", statusResp.StatusCode)
				continue
			}

			var status DoubleDoubleStatusResponse
			if err := json.NewDecoder(statusResp.Body).Decode(&status); err != nil {
				statusResp.Body.Close()
				fmt.Printf("\r[Amazon] Invalid JSON response, retrying...")
				continue
			}
			statusResp.Body.Close()

			if status.Status == "done" {
				fmt.Println("\n[Amazon] Download ready!")

				// Build download URL
				fileURL := status.URL
				if strings.HasPrefix(fileURL, "./") {
					fileURL = fmt.Sprintf("%s/%s", baseURL, fileURL[2:])
				} else if strings.HasPrefix(fileURL, "/") {
					fileURL = fmt.Sprintf("%s%s", baseURL, fileURL)
				}

				trackName := status.Current.Name
				artist := status.Current.Artist

				fmt.Printf("[Amazon] Downloading: %s - %s\n", artist, trackName)
				return fileURL, trackName, artist, nil

			} else if status.Status == "error" {
				errorMsg := status.FriendlyStatus
				if errorMsg == "" {
					errorMsg = "Unknown error"
				}
				lastError = fmt.Errorf("processing failed: %s", errorMsg)
				break
			} else {
				// Still processing
				friendlyStatus := status.FriendlyStatus
				if friendlyStatus == "" {
					friendlyStatus = status.Status
				}
				fmt.Printf("\r[Amazon] %s...", friendlyStatus)
			}
		}

		if elapsed >= maxWait {
			lastError = fmt.Errorf("download timeout")
			fmt.Printf("\n[Amazon] Error with %s region: %v\n", region, lastError)
			continue
		}

		if lastError != nil {
			fmt.Printf("\n[Amazon] Error with %s region: %v\n", region, lastError)
		}
	}

	return "", "", "", fmt.Errorf("all regions failed. Last error: %v", lastError)
}


// DownloadFile downloads a file from URL with User-Agent and progress tracking
func (a *AmazonDownloader) DownloadFile(downloadURL, outputPath, itemID string) error {
	// Initialize item progress (required for all downloads)
	if itemID != "" {
		StartItemProgress(itemID)
		defer CompleteItemProgress(itemID)
	}

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", getRandomUserAgent())

	resp, err := a.client.Do(req)
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
	var bytesWritten int64
	if itemID != "" {
		pw := NewItemProgressWriter(bufWriter, itemID)
		bytesWritten, err = io.Copy(pw, resp.Body)
	} else {
		// Fallback: direct copy without progress tracking
		bytesWritten, err = io.Copy(bufWriter, resp.Body)
	}
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("\r[Amazon] Downloaded: %.2f MB (Complete)\n", float64(bytesWritten)/(1024*1024))
	return nil
}

// AmazonDownloadResult contains download result with quality info
type AmazonDownloadResult struct {
	FilePath   string
	BitDepth   int
	SampleRate int
}

// downloadFromAmazon downloads a track using the request parameters
// Uses DoubleDouble service (same as PC version)
func downloadFromAmazon(req DownloadRequest) (AmazonDownloadResult, error) {
	downloader := NewAmazonDownloader()

	// Check for existing file first
	if existingFile, exists := checkISRCExistsInternal(req.OutputDir, req.ISRC); exists {
		return AmazonDownloadResult{FilePath: "EXISTS:" + existingFile}, nil
	}

	// Get Amazon URL from SongLink
	songlink := NewSongLinkClient()
	availability, err := songlink.CheckTrackAvailability(req.SpotifyID, req.ISRC)
	if err != nil {
		return AmazonDownloadResult{}, fmt.Errorf("failed to check Amazon availability via SongLink: %w", err)
	}

	if !availability.Amazon || availability.AmazonURL == "" {
		return AmazonDownloadResult{}, fmt.Errorf("track not available on Amazon Music (SongLink returned no Amazon URL)")
	}

	// Create output directory if needed
	if req.OutputDir != "." {
		if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
			return AmazonDownloadResult{}, fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Download using DoubleDouble service (same as PC)
	downloadURL, trackName, artistName, err := downloader.downloadFromDoubleDoubleService(availability.AmazonURL, req.OutputDir)
	if err != nil {
		return AmazonDownloadResult{}, fmt.Errorf("failed to get download URL: %w", err)
	}

	// Verify artist matches
	if artistName != "" && !amazonArtistsMatch(req.ArtistName, artistName) {
		fmt.Printf("[Amazon] Artist mismatch: expected '%s', got '%s'. Rejecting.\n", req.ArtistName, artistName)
		return AmazonDownloadResult{}, fmt.Errorf("artist mismatch: expected '%s', got '%s'", req.ArtistName, artistName)
	}

	// Log match found
	fmt.Printf("[Amazon] Match found: '%s' by '%s'\n", trackName, artistName)

	// Build filename using Spotify metadata (more accurate)
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
		return AmazonDownloadResult{FilePath: "EXISTS:" + outputPath}, nil
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
		return AmazonDownloadResult{}, fmt.Errorf("download failed: %w", err)
	}

	// Wait for parallel operations to complete
	<-parallelDone

	// Set progress to 100% and status to finalizing (before embedding)
	// This makes the UI show "Finalizing..." while embedding happens
	if req.ItemID != "" {
		SetItemProgress(req.ItemID, 1.0, 0, 0)
		SetItemFinalizing(req.ItemID)
	}

	// Log track info from DoubleDouble (for debugging)
	if trackName != "" && artistName != "" {
		fmt.Printf("[Amazon] DoubleDouble returned: %s - %s\n", artistName, trackName)
	}

	// Embed metadata using Spotify data (more accurate than DoubleDouble)
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
		fmt.Printf("[Amazon] Using parallel-fetched cover (%d bytes)\n", len(coverData))
	}

	if err := EmbedMetadataWithCoverData(outputPath, metadata, coverData); err != nil {
		fmt.Printf("Warning: failed to embed metadata: %v\n", err)
	}

	// Embed lyrics from parallel fetch
	if req.EmbedLyrics && parallelResult != nil && parallelResult.LyricsLRC != "" {
		fmt.Printf("[Amazon] Embedding parallel-fetched lyrics (%d lines)...\n", len(parallelResult.LyricsData.Lines))
		if embedErr := EmbedLyrics(outputPath, parallelResult.LyricsLRC); embedErr != nil {
			fmt.Printf("[Amazon] Warning: failed to embed lyrics: %v\n", embedErr)
		} else {
			fmt.Println("[Amazon] Lyrics embedded successfully")
		}
	} else if req.EmbedLyrics {
		fmt.Println("[Amazon] No lyrics available from parallel fetch")
	}

	fmt.Println("[Amazon] ✓ Downloaded successfully from Amazon Music")
	
	// Read actual quality from the downloaded FLAC file
	// Amazon API doesn't provide quality info, but we can read it from the file itself
	quality, err := GetAudioQuality(outputPath)
	if err != nil {
		fmt.Printf("[Amazon] Warning: couldn't read quality from file: %v\n", err)
		// Return 0 to indicate unknown quality
		return AmazonDownloadResult{
			FilePath:   outputPath,
			BitDepth:   0,
			SampleRate: 0,
		}, nil
	}
	
	fmt.Printf("[Amazon] Actual quality: %d-bit/%dHz\n", quality.BitDepth, quality.SampleRate)
	return AmazonDownloadResult{
		FilePath:   outputPath,
		BitDepth:   quality.BitDepth,
		SampleRate: quality.SampleRate,
	}, nil
}
