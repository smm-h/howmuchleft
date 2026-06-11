package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const OAuthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

// tierNames maps rate limit tier identifiers to display names.
var tierNames = map[string]string{
	"default_claude_pro":        "Pro",
	"default_claude_pro_max_5x": "Max 5x",
	"default_claude_max_5x":     "Max 5x",
	"default_claude_pro_max_20x": "Max 20x",
	"default_claude_max_20x":     "Max 20x",
}

// Credentials holds the minimal token set needed for OAuth operations.
type Credentials struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"` // unix ms
}

// OAuthData represents the claudeAiOauth section of the credentials file.
type OAuthData struct {
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken"`
	ExpiresAt        int64  `json:"expiresAt"`
	RateLimitTier    string `json:"rateLimitTier"`
	IsTeam           bool   `json:"memberOfActiveTeam"`
	SubscriptionName string `json:"-"` // computed, not serialized
}

// CredFile represents the full .credentials.json structure.
// RawFields preserves unknown top-level keys for round-trip fidelity.
type CredFile struct {
	ClaudeAiOauth *OAuthData             `json:"claudeAiOauth"`
	RawFields     map[string]json.RawMessage `json:"-"`
}

func (c *CredFile) UnmarshalJSON(data []byte) error {
	// Decode all top-level fields into RawFields for round-trip preservation.
	if err := json.Unmarshal(data, &c.RawFields); err != nil {
		return err
	}
	// Decode the known field.
	if raw, ok := c.RawFields["claudeAiOauth"]; ok {
		c.ClaudeAiOauth = &OAuthData{}
		if err := json.Unmarshal(raw, c.ClaudeAiOauth); err != nil {
			c.ClaudeAiOauth = nil
		}
	}
	return nil
}

func (c *CredFile) MarshalJSON() ([]byte, error) {
	// Start with existing raw fields (preserves mcpOAuth, etc.)
	out := make(map[string]json.RawMessage)
	for k, v := range c.RawFields {
		out[k] = v
	}
	// Overwrite claudeAiOauth with current state.
	if c.ClaudeAiOauth != nil {
		raw, err := json.Marshal(c.ClaudeAiOauth)
		if err != nil {
			return nil, err
		}
		out["claudeAiOauth"] = raw
	} else {
		delete(out, "claudeAiOauth")
	}
	return json.Marshal(out)
}

// AuthInfo describes the authentication method and subscription tier.
type AuthInfo struct {
	IsOAuth          bool
	SubscriptionName string // "Pro", "Max 5x", "Max 20x", "Team Pro", "Team Max 5x", etc.
}

// credentialsCache is a per-process cache keyed by claudeDir.
var credentialsCache sync.Map

// ReadCredentialsFile reads and parses <claudeDir>/.credentials.json.
// Returns nil on any error (missing file, parse failure).
// Results are cached per claudeDir for the process lifetime.
func ReadCredentialsFile(claudeDir string) *CredFile {
	if v, ok := credentialsCache.Load(claudeDir); ok {
		cf, _ := v.(*CredFile)
		return cf
	}

	credPath := filepath.Join(claudeDir, ".credentials.json")
	data, err := os.ReadFile(credPath)
	if err != nil {
		credentialsCache.Store(claudeDir, (*CredFile)(nil))
		return nil
	}

	var cf CredFile
	if err := json.Unmarshal(data, &cf); err != nil {
		credentialsCache.Store(claudeDir, (*CredFile)(nil))
		return nil
	}

	credentialsCache.Store(claudeDir, &cf)
	return &cf
}

// ReadKeychainCredentials reads OAuth credentials from the macOS Keychain.
// Returns nil on non-macOS platforms or any error.
func ReadKeychainCredentials(claudeDir string) *OAuthData {
	if runtime.GOOS != "darwin" {
		return nil
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(claudeDir)))[:8]
	services := []string{
		fmt.Sprintf("Claude Code-credentials-%s", hash),
		"Claude Code-credentials",
	}

	for _, svc := range services {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		out, err := exec.CommandContext(ctx, "security",
			"find-generic-password", "-s", svc, "-w",
		).Output()
		cancel()
		if err != nil {
			continue
		}

		var parsed struct {
			ClaudeAiOauth *OAuthData `json:"claudeAiOauth"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &parsed); err != nil {
			continue
		}
		if parsed.ClaudeAiOauth != nil && parsed.ClaudeAiOauth.AccessToken != "" {
			return parsed.ClaudeAiOauth
		}
	}
	return nil
}

// GetAuthInfo determines auth type and subscription display name from OAuth data.
func GetAuthInfo(oauth *OAuthData) AuthInfo {
	if oauth == nil || oauth.AccessToken == "" {
		return AuthInfo{IsOAuth: false, SubscriptionName: "API"}
	}

	baseName := tierNames[oauth.RateLimitTier]
	if baseName == "" {
		baseName = "Pro"
	}

	subscriptionName := baseName
	if oauth.IsTeam && !strings.HasPrefix(baseName, "Team") {
		subscriptionName = "Team " + baseName
	}

	return AuthInfo{IsOAuth: true, SubscriptionName: subscriptionName}
}

// ResetCredentialsCache clears the per-process credentials cache.
func ResetCredentialsCache() {
	credentialsCache.Range(func(key, _ any) bool {
		credentialsCache.Delete(key)
		return true
	})
}

// tokenResponse is the JSON response from the OAuth token endpoint.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    *int64 `json:"expires_in"`
}

// RefreshToken refreshes an expired OAuth token and writes updated credentials
// back to <claudeDir>/.credentials.json atomically.
func RefreshToken(claudeDir string, credFile *CredFile, oauth *OAuthData) error {
	if oauth == nil || oauth.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {oauth.RefreshToken},
		"client_id":     {OAuthClientID},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://console.anthropic.com/v1/oauth/token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "howmuchleft")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}

	var tokens tokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return fmt.Errorf("parsing token response: %w", err)
	}

	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		return fmt.Errorf("incomplete token response")
	}

	// Validate expires_in: default to 8h if missing/invalid, cap at 24h.
	var ttl int64 = 28800
	if tokens.ExpiresIn != nil && *tokens.ExpiresIn > 0 {
		ttl = *tokens.ExpiresIn
		if ttl > 86400 {
			ttl = 86400
		}
	}

	// Update OAuth data in memory.
	oauth.AccessToken = tokens.AccessToken
	oauth.RefreshToken = tokens.RefreshToken
	oauth.ExpiresAt = time.Now().UnixMilli() + (ttl * 1000)

	// Update the CredFile and write atomically.
	credFile.ClaudeAiOauth = oauth
	data, err := json.MarshalIndent(credFile, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling credentials: %w", err)
	}

	credPath := filepath.Join(claudeDir, ".credentials.json")
	if err := WriteFileAtomic(credPath, data); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}

	// Update cache with refreshed credentials.
	credentialsCache.Store(claudeDir, credFile)

	return nil
}

// GetValidToken returns a valid access token, refreshing if expired.
// Falls back to Keychain on macOS if refresh fails.
func GetValidToken(claudeDir string) (string, error) {
	credFile := ReadCredentialsFile(claudeDir)
	if credFile == nil {
		return "", fmt.Errorf("no credentials file found")
	}

	oauth := credFile.ClaudeAiOauth
	if oauth == nil || oauth.AccessToken == "" {
		return "", fmt.Errorf("no OAuth credentials")
	}

	// Token is still valid (with 60s buffer).
	if oauth.ExpiresAt > 0 && time.Now().UnixMilli() < oauth.ExpiresAt-60_000 {
		return oauth.AccessToken, nil
	}

	// Try to refresh.
	if err := RefreshToken(claudeDir, credFile, oauth); err == nil {
		return oauth.AccessToken, nil
	}

	// Refresh failed: try Keychain as fallback (macOS).
	keychainOAuth := ReadKeychainCredentials(claudeDir)
	if keychainOAuth != nil && keychainOAuth.AccessToken != "" &&
		keychainOAuth.ExpiresAt > 0 && time.Now().UnixMilli() < keychainOAuth.ExpiresAt-60_000 {
		return keychainOAuth.AccessToken, nil
	}

	return "", fmt.Errorf("token expired and refresh failed")
}

// WriteFileAtomic writes data to a temporary file in the same directory as path,
// then renames it atomically. This ensures readers see either the complete old
// or complete new file.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
