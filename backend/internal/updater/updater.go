package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Version is set at build time via -ldflags "-X ...updater.Version=1.2.0"
var Version = "dev"

// githubRelease is the subset of the GitHub API response we care about
type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`        // markdown release notes
	HTMLURL     string `json:"html_url"`    // link to release page
	PublishedAt string `json:"published_at"`
}

// UpdateInfo is returned by the /api/version endpoint
type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version,omitempty"`
	UpdateAvailable bool  `json:"update_available"`
	ReleaseNotes   string `json:"release_notes,omitempty"`  // markdown
	ReleaseURL     string `json:"release_url,omitempty"`
	ReleaseName    string `json:"release_name,omitempty"`
	PublishedAt    string `json:"published_at,omitempty"`
	CheckedAt      string `json:"checked_at,omitempty"`
	Error          string `json:"error,omitempty"`
}

// Checker periodically polls GitHub for new releases
type Checker struct {
	repo     string // "owner/repo"
	interval time.Duration

	mu     sync.RWMutex
	latest *UpdateInfo
}

// NewChecker creates a release checker for a GitHub repo
func NewChecker(repo string, interval time.Duration) *Checker {
	return &Checker{
		repo:     repo,
		interval: interval,
		latest: &UpdateInfo{
			CurrentVersion: Version,
		},
	}
}

// Start begins periodic checking in the background
func (c *Checker) Start(ctx context.Context) {
	// Check immediately on startup
	c.check()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.check()
		}
	}
}

// GetUpdateInfo returns the latest cached update info
func (c *Checker) GetUpdateInfo() *UpdateInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy
	info := *c.latest
	info.CurrentVersion = Version
	return &info
}

func (c *Checker) check() {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", c.repo)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Debug("updater: failed to create request", "error", err)
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "pCenter/"+Version)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Debug("updater: failed to fetch latest release", "error", err)
		c.setError("failed to reach GitHub")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		slog.Debug("updater: GitHub API error", "status", resp.StatusCode, "body", string(body))
		c.setError(fmt.Sprintf("GitHub API returned %d", resp.StatusCode))
		return
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		slog.Debug("updater: failed to parse response", "error", err)
		c.setError("failed to parse GitHub response")
		return
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	updateAvailable := isNewer(latestVersion, Version)

	c.mu.Lock()
	c.latest = &UpdateInfo{
		CurrentVersion:  Version,
		LatestVersion:   latestVersion,
		UpdateAvailable: updateAvailable,
		ReleaseNotes:    release.Body,
		ReleaseURL:      release.HTMLURL,
		ReleaseName:     release.Name,
		PublishedAt:     release.PublishedAt,
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	c.mu.Unlock()

	if updateAvailable {
		slog.Info("update available", "current", Version, "latest", latestVersion)
	} else {
		slog.Debug("updater: up to date", "version", Version)
	}
}

func (c *Checker) setError(msg string) {
	c.mu.Lock()
	c.latest = &UpdateInfo{
		CurrentVersion: Version,
		Error:          msg,
		CheckedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	c.mu.Unlock()
}

// isNewer returns true if latest > current using semver comparison
func isNewer(latest, current string) bool {
	if current == "dev" || current == "" {
		return false // dev builds never show updates
	}

	latestParts := parseSemver(latest)
	currentParts := parseSemver(current)

	for i := 0; i < 3; i++ {
		if latestParts[i] > currentParts[i] {
			return true
		}
		if latestParts[i] < currentParts[i] {
			return false
		}
	}
	return false
}

// parseSemver parses "1.2.3" into [1, 2, 3]
func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		// Strip pre-release suffix (e.g., "3-beta")
		clean := strings.SplitN(parts[i], "-", 2)[0]
		result[i], _ = strconv.Atoi(clean)
	}
	return result
}
