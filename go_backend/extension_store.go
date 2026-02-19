package gobackend

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	CategoryMetadata    = "metadata"
	CategoryDownload    = "download"
	CategoryUtility     = "utility"
	CategoryLyrics      = "lyrics"
	CategoryIntegration = "integration"
)

type StoreExtension struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	DisplayName      string   `json:"display_name,omitempty"`
	Version          string   `json:"version"`
	Author           string   `json:"author"`
	Description      string   `json:"description"`
	DownloadURL      string   `json:"download_url,omitempty"`
	IconURL          string   `json:"icon_url,omitempty"`
	Category         string   `json:"category"`
	Tags             []string `json:"tags,omitempty"`
	Downloads        int      `json:"downloads"`
	UpdatedAt        string   `json:"updated_at"`
	MinAppVersion    string   `json:"min_app_version,omitempty"`
	DisplayNameAlt   string   `json:"displayName,omitempty"`
	DownloadURLAlt   string   `json:"downloadUrl,omitempty"`
	IconURLAlt       string   `json:"iconUrl,omitempty"`
	MinAppVersionAlt string   `json:"minAppVersion,omitempty"`
}

func (e *StoreExtension) getDisplayName() string {
	if e.DisplayName != "" {
		return e.DisplayName
	}
	if e.DisplayNameAlt != "" {
		return e.DisplayNameAlt
	}
	return e.Name
}

func (e *StoreExtension) getDownloadURL() string {
	if e.DownloadURL != "" {
		return e.DownloadURL
	}
	return e.DownloadURLAlt
}

func (e *StoreExtension) getIconURL() string {
	if e.IconURL != "" {
		return e.IconURL
	}
	return e.IconURLAlt
}

func (e *StoreExtension) getMinAppVersion() string {
	if e.MinAppVersion != "" {
		return e.MinAppVersion
	}
	return e.MinAppVersionAlt
}

type StoreRegistry struct {
	Version    int              `json:"version"`
	UpdatedAt  string           `json:"updated_at"`
	Extensions []StoreExtension `json:"extensions"`
}

// StoreExtensionResponse is the normalized response sent to Flutter
type StoreExtensionResponse struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	DisplayName      string   `json:"display_name"`
	Version          string   `json:"version"`
	Author           string   `json:"author"`
	Description      string   `json:"description"`
	DownloadURL      string   `json:"download_url"`
	IconURL          string   `json:"icon_url,omitempty"`
	Category         string   `json:"category"`
	Tags             []string `json:"tags,omitempty"`
	Downloads        int      `json:"downloads"`
	UpdatedAt        string   `json:"updated_at"`
	MinAppVersion    string   `json:"min_app_version,omitempty"`
	IsInstalled      bool     `json:"is_installed"`
	InstalledVersion string   `json:"installed_version,omitempty"`
	HasUpdate        bool     `json:"has_update"`
}

func (e *StoreExtension) ToResponse() StoreExtensionResponse {
	return StoreExtensionResponse{
		ID:            e.ID,
		Name:          e.Name,
		DisplayName:   e.getDisplayName(),
		Version:       e.Version,
		Author:        e.Author,
		Description:   e.Description,
		DownloadURL:   e.getDownloadURL(),
		IconURL:       e.getIconURL(),
		Category:      e.Category,
		Tags:          e.Tags,
		Downloads:     e.Downloads,
		UpdatedAt:     e.UpdatedAt,
		MinAppVersion: e.getMinAppVersion(),
	}
}

type ExtensionStore struct {
	registryURL string
	cacheDir    string
	cache       *StoreRegistry
	cacheMu     sync.RWMutex
	cacheTime   time.Time
	cacheTTL    time.Duration
}

var (
	extensionStore   *ExtensionStore
	extensionStoreMu sync.Mutex
)

const (
	defaultRegistryURL = "https://raw.githubusercontent.com/zarzet/SpotiFLAC-Extension/main/registry.json"
	cacheTTL           = 30 * time.Minute
	cacheFileName      = "store_cache.json"
)

func InitExtensionStore(cacheDir string) *ExtensionStore {
	extensionStoreMu.Lock()
	defer extensionStoreMu.Unlock()

	if extensionStore == nil {
		extensionStore = &ExtensionStore{
			registryURL: defaultRegistryURL,
			cacheDir:    cacheDir,
			cacheTTL:    cacheTTL,
		}
		extensionStore.loadDiskCache()
	}
	return extensionStore
}

func GetExtensionStore() *ExtensionStore {
	extensionStoreMu.Lock()
	defer extensionStoreMu.Unlock()
	return extensionStore
}

func (s *ExtensionStore) loadDiskCache() {
	if s.cacheDir == "" {
		return
	}

	cachePath := filepath.Join(s.cacheDir, cacheFileName)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return
	}

	var cacheData struct {
		Registry  StoreRegistry `json:"registry"`
		CacheTime int64         `json:"cache_time"`
	}

	if err := json.Unmarshal(data, &cacheData); err != nil {
		return
	}

	s.cache = &cacheData.Registry
	s.cacheTime = time.Unix(cacheData.CacheTime, 0)
	LogDebug("ExtensionStore", "Loaded %d extensions from disk cache", len(s.cache.Extensions))
}

func (s *ExtensionStore) saveDiskCache() {
	if s.cacheDir == "" || s.cache == nil {
		return
	}

	cacheData := struct {
		Registry  StoreRegistry `json:"registry"`
		CacheTime int64         `json:"cache_time"`
	}{
		Registry:  *s.cache,
		CacheTime: s.cacheTime.Unix(),
	}

	data, err := json.Marshal(cacheData)
	if err != nil {
		return
	}

	cachePath := filepath.Join(s.cacheDir, cacheFileName)
	os.WriteFile(cachePath, data, 0644)
}

func (s *ExtensionStore) FetchRegistry(forceRefresh bool) (*StoreRegistry, error) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	if !forceRefresh && s.cache != nil && time.Since(s.cacheTime) < s.cacheTTL {
		LogDebug("ExtensionStore", "Using cached registry (%d extensions)", len(s.cache.Extensions))
		return s.cache, nil
	}

	if err := requireHTTPSURL(s.registryURL, "registry"); err != nil {
		return nil, err
	}

	LogInfo("ExtensionStore", "Fetching registry from %s", s.registryURL)

	client := NewHTTPClientWithTimeout(30 * time.Second)
	resp, err := client.Get(s.registryURL)
	if err != nil {
		if s.cache != nil {
			LogWarn("ExtensionStore", "Network error, using cached registry: %v", err)
			return s.cache, nil
		}
		return nil, fmt.Errorf("failed to fetch registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if s.cache != nil {
			LogWarn("ExtensionStore", "HTTP %d, using cached registry", resp.StatusCode)
			return s.cache, nil
		}
		return nil, fmt.Errorf("registry returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read registry: %w", err)
	}

	var registry StoreRegistry
	if err := json.Unmarshal(body, &registry); err != nil {
		return nil, fmt.Errorf("failed to parse registry: %w", err)
	}

	s.cache = &registry
	s.cacheTime = time.Now()
	s.saveDiskCache()

	LogInfo("ExtensionStore", "Fetched %d extensions from registry", len(registry.Extensions))
	return &registry, nil
}

func (s *ExtensionStore) GetExtensionsWithStatus() ([]StoreExtensionResponse, error) {
	registry, err := s.FetchRegistry(false)
	if err != nil {
		return nil, err
	}

	manager := GetExtensionManager()
	installed := make(map[string]string) // id -> version

	if manager != nil {
		for _, ext := range manager.GetAllExtensions() {
			installed[ext.ID] = ext.Manifest.Version
		}
	}

	result := make([]StoreExtensionResponse, len(registry.Extensions))
	for i, ext := range registry.Extensions {
		resp := ext.ToResponse()

		if installedVersion, ok := installed[ext.ID]; ok {
			resp.IsInstalled = true
			resp.InstalledVersion = installedVersion
			resp.HasUpdate = compareVersions(ext.Version, installedVersion) > 0
		}

		result[i] = resp
	}

	return result, nil
}

func (s *ExtensionStore) DownloadExtension(extensionID string, destPath string) error {
	registry, err := s.FetchRegistry(false)
	if err != nil {
		return err
	}

	var ext *StoreExtension
	for _, e := range registry.Extensions {
		if e.ID == extensionID {
			ext = &e
			break
		}
	}

	if ext == nil {
		return fmt.Errorf("extension %s not found in store", extensionID)
	}

	if err := requireHTTPSURL(ext.getDownloadURL(), "extension download"); err != nil {
		return err
	}

	LogInfo("ExtensionStore", "Downloading %s from %s", ext.getDisplayName(), ext.getDownloadURL())

	client := NewHTTPClientWithTimeout(5 * time.Minute)
	resp, err := client.Get(ext.getDownloadURL())
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		os.Remove(destPath)
		return fmt.Errorf("failed to write file: %w", err)
	}

	LogInfo("ExtensionStore", "Downloaded %s to %s", ext.getDisplayName(), destPath)
	return nil
}

func requireHTTPSURL(rawURL string, context string) error {
	if rawURL == "" {
		return fmt.Errorf("%s URL is empty", context)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%s URL is invalid: %s", context, rawURL)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("%s URL must use https: %s", context, rawURL)
	}
	return nil
}

func (s *ExtensionStore) GetCategories() []string {
	return []string{
		CategoryMetadata,
		CategoryDownload,
		CategoryUtility,
		CategoryLyrics,
		CategoryIntegration,
	}
}

func (s *ExtensionStore) SearchExtensions(query string, category string) ([]StoreExtensionResponse, error) {
	extensions, err := s.GetExtensionsWithStatus()
	if err != nil {
		return nil, err
	}

	if query == "" && category == "" {
		return extensions, nil
	}

	var result []StoreExtensionResponse
	queryLower := toLower(query)

	for _, ext := range extensions {
		// Filter by category
		if category != "" && ext.Category != category {
			continue
		}

		// Filter by query
		if query != "" {
			if !containsIgnoreCase(ext.Name, queryLower) &&
				!containsIgnoreCase(ext.DisplayName, queryLower) &&
				!containsIgnoreCase(ext.Description, queryLower) &&
				!containsIgnoreCase(ext.Author, queryLower) {
				// Check tags
				found := false
				for _, tag := range ext.Tags {
					if containsIgnoreCase(tag, queryLower) {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
		}

		result = append(result, ext)
	}

	return result, nil
}

func (s *ExtensionStore) ClearCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	s.cache = nil
	s.cacheTime = time.Time{}

	if s.cacheDir != "" {
		cachePath := filepath.Join(s.cacheDir, cacheFileName)
		os.Remove(cachePath)
	}

	LogInfo("ExtensionStore", "Cache cleared")
}

// Helper: case-insensitive contains
func containsIgnoreCase(s, substr string) bool {
	return containsStr(toLower(s), substr)
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func containsStr(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && findSubstring(s, substr) >= 0)
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
