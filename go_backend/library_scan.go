package gobackend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LibraryScanResult represents metadata from a scanned audio file
type LibraryScanResult struct {
	ID          string `json:"id"`
	TrackName   string `json:"trackName"`
	ArtistName  string `json:"artistName"`
	AlbumName   string `json:"albumName"`
	AlbumArtist string `json:"albumArtist,omitempty"`
	FilePath    string `json:"filePath"`
	CoverPath   string `json:"coverPath,omitempty"`
	ScannedAt   string `json:"scannedAt"`
	ISRC        string `json:"isrc,omitempty"`
	TrackNumber int    `json:"trackNumber,omitempty"`
	DiscNumber  int    `json:"discNumber,omitempty"`
	Duration    int    `json:"duration,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	BitDepth    int    `json:"bitDepth,omitempty"`
	SampleRate  int    `json:"sampleRate,omitempty"`
	Genre       string `json:"genre,omitempty"`
	Format      string `json:"format,omitempty"`
}

// LibraryScanProgress reports progress during scan
type LibraryScanProgress struct {
	TotalFiles   int     `json:"total_files"`
	ScannedFiles int     `json:"scanned_files"`
	CurrentFile  string  `json:"current_file"`
	ErrorCount   int     `json:"error_count"`
	ProgressPct  float64 `json:"progress_pct"`
	IsComplete   bool    `json:"is_complete"`
}

var (
	libraryScanProgress   LibraryScanProgress
	libraryScanProgressMu sync.RWMutex
	libraryScanCancel     chan struct{}
	libraryScanCancelMu   sync.Mutex
	libraryCoverCacheDir  string // Directory to cache extracted cover art
	libraryCoverCacheMu   sync.RWMutex
)

// supportedAudioFormats lists file extensions we can read metadata from
var supportedAudioFormats = map[string]bool{
	".flac": true,
	".m4a":  true,
	".mp3":  true,
	".opus": true,
	".ogg":  true,
}

// SetLibraryCoverCacheDir sets the directory to cache extracted cover art
func SetLibraryCoverCacheDir(cacheDir string) {
	libraryCoverCacheMu.Lock()
	libraryCoverCacheDir = cacheDir
	libraryCoverCacheMu.Unlock()
}

// ScanLibraryFolder scans a folder recursively for audio files and reads their metadata
// Returns JSON array of LibraryScanResult
func ScanLibraryFolder(folderPath string) (string, error) {
	if folderPath == "" {
		return "[]", fmt.Errorf("folder path is empty")
	}

	// Check if folder exists
	info, err := os.Stat(folderPath)
	if err != nil {
		return "[]", fmt.Errorf("folder not found: %w", err)
	}
	if !info.IsDir() {
		return "[]", fmt.Errorf("path is not a folder: %s", folderPath)
	}

	// Reset progress
	libraryScanProgressMu.Lock()
	libraryScanProgress = LibraryScanProgress{}
	libraryScanProgressMu.Unlock()

	// Create cancel channel
	libraryScanCancelMu.Lock()
	if libraryScanCancel != nil {
		close(libraryScanCancel)
	}
	libraryScanCancel = make(chan struct{})
	cancelCh := libraryScanCancel
	libraryScanCancelMu.Unlock()

	// First pass: count audio files
	var audioFiles []string
	err = filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}

		select {
		case <-cancelCh:
			return fmt.Errorf("scan cancelled")
		default:
		}

		if !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(path))
			if supportedAudioFormats[ext] {
				audioFiles = append(audioFiles, path)
			}
		}
		return nil
	})

	if err != nil {
		return "[]", err
	}

	totalFiles := len(audioFiles)
	libraryScanProgressMu.Lock()
	libraryScanProgress.TotalFiles = totalFiles
	libraryScanProgressMu.Unlock()

	if totalFiles == 0 {
		libraryScanProgressMu.Lock()
		libraryScanProgress.IsComplete = true
		libraryScanProgressMu.Unlock()
		return "[]", nil
	}

	GoLog("[LibraryScan] Found %d audio files to scan\n", totalFiles)

	// Second pass: read metadata from each file
	results := make([]LibraryScanResult, 0, totalFiles)
	scanTime := time.Now().UTC().Format(time.RFC3339)
	errorCount := 0

	for i, filePath := range audioFiles {
		select {
		case <-cancelCh:
			return "[]", fmt.Errorf("scan cancelled")
		default:
		}

		// Update progress
		libraryScanProgressMu.Lock()
		libraryScanProgress.ScannedFiles = i + 1
		libraryScanProgress.CurrentFile = filepath.Base(filePath)
		libraryScanProgress.ProgressPct = float64(i+1) / float64(totalFiles) * 100
		libraryScanProgressMu.Unlock()

		// Read metadata
		result, err := scanAudioFile(filePath, scanTime)
		if err != nil {
			errorCount++
			GoLog("[LibraryScan] Error scanning %s: %v\n", filePath, err)
			continue
		}

		results = append(results, *result)
	}

	// Mark complete
	libraryScanProgressMu.Lock()
	libraryScanProgress.ErrorCount = errorCount
	libraryScanProgress.IsComplete = true
	libraryScanProgressMu.Unlock()

	GoLog("[LibraryScan] Scan complete: %d tracks found, %d errors\n", len(results), errorCount)

	jsonBytes, err := json.Marshal(results)
	if err != nil {
		return "[]", fmt.Errorf("failed to marshal results: %w", err)
	}

	return string(jsonBytes), nil
}

// scanAudioFile reads metadata from a single audio file
func scanAudioFile(filePath, scanTime string) (*LibraryScanResult, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	result := &LibraryScanResult{
		ID:        generateLibraryID(filePath),
		FilePath:  filePath,
		ScannedAt: scanTime,
		Format:    strings.TrimPrefix(ext, "."),
	}

	// Try to extract and cache cover art
	libraryCoverCacheMu.RLock()
	coverCacheDir := libraryCoverCacheDir
	libraryCoverCacheMu.RUnlock()
	if coverCacheDir != "" && ext != ".m4a" {
		coverPath, err := SaveCoverToCache(filePath, coverCacheDir)
		if err == nil && coverPath != "" {
			result.CoverPath = coverPath
		}
	}

	// Try to read metadata based on format
	switch ext {
	case ".flac":
		return scanFLACFile(filePath, result)
	case ".m4a":
		return scanM4AFile(filePath, result)
	case ".mp3":
		return scanMP3File(filePath, result)
	case ".opus", ".ogg":
		// Opus files often use same container as Ogg Vorbis
		return scanOggFile(filePath, result)
	default:
		// Fallback: use filename as title
		return scanFromFilename(filePath, result)
	}
}

// scanFLACFile reads metadata from FLAC file
func scanFLACFile(filePath string, result *LibraryScanResult) (*LibraryScanResult, error) {
	metadata, err := ReadMetadata(filePath)
	if err != nil {
		// Fallback to filename
		return scanFromFilename(filePath, result)
	}

	result.TrackName = metadata.Title
	result.ArtistName = metadata.Artist
	result.AlbumName = metadata.Album
	result.AlbumArtist = metadata.AlbumArtist
	result.ISRC = metadata.ISRC
	result.TrackNumber = metadata.TrackNumber
	result.DiscNumber = metadata.DiscNumber
	result.ReleaseDate = metadata.Date
	result.Genre = metadata.Genre

	// Read audio quality
	quality, err := GetAudioQuality(filePath)
	if err == nil {
		result.BitDepth = quality.BitDepth
		result.SampleRate = quality.SampleRate
		if quality.SampleRate > 0 && quality.TotalSamples > 0 {
			result.Duration = int(quality.TotalSamples / int64(quality.SampleRate))
		}
	}

	// Ensure we have at least a title
	if result.TrackName == "" {
		result.TrackName = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}
	if result.ArtistName == "" {
		result.ArtistName = "Unknown Artist"
	}
	if result.AlbumName == "" {
		result.AlbumName = "Unknown Album"
	}

	return result, nil
}

// scanM4AFile reads metadata from M4A/AAC file
func scanM4AFile(filePath string, result *LibraryScanResult) (*LibraryScanResult, error) {
	// M4A metadata reading is limited, try audio quality at least
	quality, err := GetM4AQuality(filePath)
	if err == nil {
		result.BitDepth = quality.BitDepth
		result.SampleRate = quality.SampleRate
	}

	// Fallback to filename parsing
	return scanFromFilename(filePath, result)
}

// scanMP3File reads metadata from MP3 file (ID3 tags)
func scanMP3File(filePath string, result *LibraryScanResult) (*LibraryScanResult, error) {
	metadata, err := ReadID3Tags(filePath)
	if err != nil {
		GoLog("[LibraryScan] ID3 read error for %s: %v\n", filePath, err)
		return scanFromFilename(filePath, result)
	}

	result.TrackName = metadata.Title
	result.ArtistName = metadata.Artist
	result.AlbumName = metadata.Album
	result.AlbumArtist = metadata.AlbumArtist
	result.TrackNumber = metadata.TrackNumber
	result.DiscNumber = metadata.DiscNumber
	result.Genre = metadata.Genre
	if metadata.Date != "" {
		result.ReleaseDate = metadata.Date
	} else {
		result.ReleaseDate = metadata.Year
	}
	result.ISRC = metadata.ISRC

	// Get audio quality info
	quality, err := GetMP3Quality(filePath)
	if err == nil {
		result.SampleRate = quality.SampleRate
		result.BitDepth = quality.BitDepth
		result.Duration = quality.Duration
	}

	// Ensure we have at least a title
	if result.TrackName == "" {
		result.TrackName = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}
	if result.ArtistName == "" {
		result.ArtistName = "Unknown Artist"
	}
	if result.AlbumName == "" {
		result.AlbumName = "Unknown Album"
	}

	return result, nil
}

// scanOggFile reads metadata from Ogg Vorbis/Opus file (Vorbis comments)
func scanOggFile(filePath string, result *LibraryScanResult) (*LibraryScanResult, error) {
	metadata, err := ReadOggVorbisComments(filePath)
	if err != nil {
		GoLog("[LibraryScan] Ogg/Opus read error for %s: %v\n", filePath, err)
		return scanFromFilename(filePath, result)
	}

	result.TrackName = metadata.Title
	result.ArtistName = metadata.Artist
	result.AlbumName = metadata.Album
	result.AlbumArtist = metadata.AlbumArtist
	result.ISRC = metadata.ISRC
	result.TrackNumber = metadata.TrackNumber
	result.DiscNumber = metadata.DiscNumber
	result.Genre = metadata.Genre
	result.ReleaseDate = metadata.Date

	// Get audio quality info
	quality, err := GetOggQuality(filePath)
	if err == nil {
		result.SampleRate = quality.SampleRate
		result.BitDepth = quality.BitDepth
		result.Duration = quality.Duration
	}

	// Ensure we have at least a title
	if result.TrackName == "" {
		result.TrackName = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}
	if result.ArtistName == "" {
		result.ArtistName = "Unknown Artist"
	}
	if result.AlbumName == "" {
		result.AlbumName = "Unknown Album"
	}

	return result, nil
}

// scanFromFilename extracts title/artist from filename pattern
func scanFromFilename(filePath string, result *LibraryScanResult) (*LibraryScanResult, error) {
	filename := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))

	// Common patterns:
	// "Artist - Title"
	// "01 - Title"
	// "01. Title"
	// "Title"

	// Try "Artist - Title" pattern
	parts := strings.SplitN(filename, " - ", 2)
	if len(parts) == 2 {
		// Check if first part looks like a track number
		if len(parts[0]) <= 3 && isNumeric(parts[0]) {
			result.TrackName = parts[1]
			result.ArtistName = "Unknown Artist"
		} else {
			result.ArtistName = parts[0]
			result.TrackName = parts[1]
		}
	} else {
		// Try "01. Title" or "01 Title" pattern
		if len(filename) > 3 && isNumeric(filename[:2]) {
			// Skip track number
			title := strings.TrimLeft(filename[2:], " .-")
			result.TrackName = title
		} else {
			result.TrackName = filename
		}
		result.ArtistName = "Unknown Artist"
	}

	// Use parent folder as album name
	dir := filepath.Dir(filePath)
	result.AlbumName = filepath.Base(dir)
	if result.AlbumName == "." || result.AlbumName == "" {
		result.AlbumName = "Unknown Album"
	}

	return result, nil
}

// isNumeric checks if string contains only digits
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// generateLibraryID creates a unique ID for a library item
func generateLibraryID(filePath string) string {
	// Use file path hash as ID
	return fmt.Sprintf("lib_%x", hashString(filePath))
}

// hashString creates a simple hash of a string
func hashString(s string) uint32 {
	var hash uint32 = 5381
	for _, c := range s {
		hash = ((hash << 5) + hash) + uint32(c)
	}
	return hash
}

// GetLibraryScanProgress returns current scan progress
func GetLibraryScanProgress() string {
	libraryScanProgressMu.RLock()
	defer libraryScanProgressMu.RUnlock()

	jsonBytes, _ := json.Marshal(libraryScanProgress)
	return string(jsonBytes)
}

// CancelLibraryScan cancels ongoing library scan
func CancelLibraryScan() {
	libraryScanCancelMu.Lock()
	defer libraryScanCancelMu.Unlock()

	if libraryScanCancel != nil {
		close(libraryScanCancel)
		libraryScanCancel = nil
	}
}

// ReadAudioMetadata reads metadata from any supported audio file
// Returns JSON with track info
func ReadAudioMetadata(filePath string) (string, error) {
	scanTime := time.Now().UTC().Format(time.RFC3339)
	result, err := scanAudioFile(filePath, scanTime)
	if err != nil {
		return "", err
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(jsonBytes), nil
}
