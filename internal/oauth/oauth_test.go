package oauth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGetAuthInfo_NilOAuth(t *testing.T) {
	info := GetAuthInfo(nil)
	if info.IsOAuth {
		t.Error("expected IsOAuth=false for nil oauth")
	}
	if info.SubscriptionName != "API" {
		t.Errorf("expected SubscriptionName=API, got %q", info.SubscriptionName)
	}
}

func TestGetAuthInfo_EmptyAccessToken(t *testing.T) {
	info := GetAuthInfo(&OAuthData{AccessToken: ""})
	if info.IsOAuth {
		t.Error("expected IsOAuth=false for empty access token")
	}
	if info.SubscriptionName != "API" {
		t.Errorf("expected SubscriptionName=API, got %q", info.SubscriptionName)
	}
}

func TestGetAuthInfo_ProTier(t *testing.T) {
	info := GetAuthInfo(&OAuthData{
		AccessToken:   "tok",
		RateLimitTier: "default_claude_pro",
	})
	if !info.IsOAuth {
		t.Error("expected IsOAuth=true")
	}
	if info.SubscriptionName != "Pro" {
		t.Errorf("expected Pro, got %q", info.SubscriptionName)
	}
}

func TestGetAuthInfo_Max5xTier(t *testing.T) {
	info := GetAuthInfo(&OAuthData{
		AccessToken:   "tok",
		RateLimitTier: "default_claude_pro_max_5x",
	})
	if info.SubscriptionName != "Max 5x" {
		t.Errorf("expected 'Max 5x', got %q", info.SubscriptionName)
	}
}

func TestGetAuthInfo_Max20xTier(t *testing.T) {
	info := GetAuthInfo(&OAuthData{
		AccessToken:   "tok",
		RateLimitTier: "default_claude_pro_max_20x",
	})
	if info.SubscriptionName != "Max 20x" {
		t.Errorf("expected 'Max 20x', got %q", info.SubscriptionName)
	}
}

func TestGetAuthInfo_AlternateTierKeys(t *testing.T) {
	// The JS source uses "default_claude_max_5x" (without _pro_).
	info := GetAuthInfo(&OAuthData{
		AccessToken:   "tok",
		RateLimitTier: "default_claude_max_5x",
	})
	if info.SubscriptionName != "Max 5x" {
		t.Errorf("expected 'Max 5x', got %q", info.SubscriptionName)
	}

	info = GetAuthInfo(&OAuthData{
		AccessToken:   "tok",
		RateLimitTier: "default_claude_max_20x",
	})
	if info.SubscriptionName != "Max 20x" {
		t.Errorf("expected 'Max 20x', got %q", info.SubscriptionName)
	}
}

func TestGetAuthInfo_UnknownTierFallsBackToPro(t *testing.T) {
	info := GetAuthInfo(&OAuthData{
		AccessToken:   "tok",
		RateLimitTier: "unknown_tier_value",
	})
	if info.SubscriptionName != "Pro" {
		t.Errorf("expected fallback Pro, got %q", info.SubscriptionName)
	}
}

func TestGetAuthInfo_TeamPrefix(t *testing.T) {
	info := GetAuthInfo(&OAuthData{
		AccessToken:   "tok",
		RateLimitTier: "default_claude_pro",
		IsTeam:        true,
	})
	if info.SubscriptionName != "Team Pro" {
		t.Errorf("expected 'Team Pro', got %q", info.SubscriptionName)
	}
}

func TestGetAuthInfo_TeamMax5x(t *testing.T) {
	info := GetAuthInfo(&OAuthData{
		AccessToken:   "tok",
		RateLimitTier: "default_claude_pro_max_5x",
		IsTeam:        true,
	})
	if info.SubscriptionName != "Team Max 5x" {
		t.Errorf("expected 'Team Max 5x', got %q", info.SubscriptionName)
	}
}

func TestReadCredentialsFile_Valid(t *testing.T) {
	ResetCredentialsCache()

	dir := t.TempDir()
	credData := map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":  "test-access-token",
			"refreshToken": "test-refresh-token",
			"expiresAt":    float64(9999999999999),
			"rateLimitTier": "default_claude_pro",
		},
		"otherField": "preserved",
	}
	data, _ := json.Marshal(credData)
	os.WriteFile(filepath.Join(dir, ".credentials.json"), data, 0644)

	cf := ReadCredentialsFile(dir)
	if cf == nil {
		t.Fatal("expected non-nil CredFile")
	}
	if cf.ClaudeAiOauth == nil {
		t.Fatal("expected non-nil ClaudeAiOauth")
	}
	if cf.ClaudeAiOauth.AccessToken != "test-access-token" {
		t.Errorf("expected 'test-access-token', got %q", cf.ClaudeAiOauth.AccessToken)
	}
	if cf.ClaudeAiOauth.RefreshToken != "test-refresh-token" {
		t.Errorf("expected 'test-refresh-token', got %q", cf.ClaudeAiOauth.RefreshToken)
	}
	if cf.ClaudeAiOauth.ExpiresAt != 9999999999999 {
		t.Errorf("expected expiresAt=9999999999999, got %d", cf.ClaudeAiOauth.ExpiresAt)
	}

	// Verify round-trip preserves other fields.
	marshaled, err := json.Marshal(cf)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip map[string]json.RawMessage
	json.Unmarshal(marshaled, &roundTrip)
	if _, ok := roundTrip["otherField"]; !ok {
		t.Error("expected otherField to be preserved in round-trip")
	}
}

func TestReadCredentialsFile_Missing(t *testing.T) {
	ResetCredentialsCache()

	dir := t.TempDir()
	cf := ReadCredentialsFile(dir)
	if cf != nil {
		t.Error("expected nil for missing credentials file")
	}
}

func TestReadCredentialsFile_InvalidJSON(t *testing.T) {
	ResetCredentialsCache()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte("not json"), 0644)
	cf := ReadCredentialsFile(dir)
	if cf != nil {
		t.Error("expected nil for invalid JSON")
	}
}

func TestReadCredentialsFile_Cached(t *testing.T) {
	ResetCredentialsCache()

	dir := t.TempDir()
	credData := map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken": "original-token",
		},
	}
	data, _ := json.Marshal(credData)
	os.WriteFile(filepath.Join(dir, ".credentials.json"), data, 0644)

	cf1 := ReadCredentialsFile(dir)
	if cf1 == nil || cf1.ClaudeAiOauth.AccessToken != "original-token" {
		t.Fatal("first read failed")
	}

	// Overwrite the file — cached result should be returned.
	credData["claudeAiOauth"] = map[string]any{"accessToken": "new-token"}
	data, _ = json.Marshal(credData)
	os.WriteFile(filepath.Join(dir, ".credentials.json"), data, 0644)

	cf2 := ReadCredentialsFile(dir)
	if cf2.ClaudeAiOauth.AccessToken != "original-token" {
		t.Error("expected cached value, got fresh read")
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "test-file.json")

	content := []byte(`{"key": "value"}`)
	if err := WriteFileAtomic(target, content); err != nil {
		t.Fatalf("WriteFileAtomic failed: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}

func TestWriteFileAtomic_NoPartialWrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "atomic-test.json")

	// Write initial content.
	initial := []byte("initial")
	os.WriteFile(target, initial, 0644)

	// Write new content atomically.
	newContent := []byte("new content that is longer")
	if err := WriteFileAtomic(target, newContent); err != nil {
		t.Fatalf("WriteFileAtomic failed: %v", err)
	}

	// File should have the new content, not a mix.
	got, _ := os.ReadFile(target)
	if string(got) != string(newContent) {
		t.Errorf("expected new content, got %q", got)
	}

	// No temp files should remain.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "atomic-test.json" {
			t.Errorf("unexpected file remaining: %s", e.Name())
		}
	}
}

func TestResetCredentialsCache(t *testing.T) {
	ResetCredentialsCache()

	dir := t.TempDir()
	credData := map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken": "cached-token",
		},
	}
	data, _ := json.Marshal(credData)
	os.WriteFile(filepath.Join(dir, ".credentials.json"), data, 0644)

	cf1 := ReadCredentialsFile(dir)
	if cf1 == nil {
		t.Fatal("first read returned nil")
	}

	ResetCredentialsCache()

	// After reset, modify file and re-read — should get new value.
	credData["claudeAiOauth"] = map[string]any{"accessToken": "fresh-token"}
	data, _ = json.Marshal(credData)
	os.WriteFile(filepath.Join(dir, ".credentials.json"), data, 0644)

	cf2 := ReadCredentialsFile(dir)
	if cf2 == nil || cf2.ClaudeAiOauth.AccessToken != "fresh-token" {
		t.Error("expected fresh value after cache reset")
	}
}

func TestCredFileMarshal_PreservesExtraFields(t *testing.T) {
	input := `{"claudeAiOauth":{"accessToken":"tok","refreshToken":"rt","expiresAt":123},"mcpOAuth":{"someKey":"someVal"}}`
	var cf CredFile
	if err := json.Unmarshal([]byte(input), &cf); err != nil {
		t.Fatal(err)
	}

	// Modify the oauth data.
	cf.ClaudeAiOauth.AccessToken = "new-tok"

	out, err := json.Marshal(&cf)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]json.RawMessage
	json.Unmarshal(out, &result)

	// mcpOAuth must be preserved.
	if _, ok := result["mcpOAuth"]; !ok {
		t.Error("mcpOAuth field lost during marshal")
	}

	// claudeAiOauth should have updated token.
	var oauth OAuthData
	json.Unmarshal(result["claudeAiOauth"], &oauth)
	if oauth.AccessToken != "new-tok" {
		t.Errorf("expected new-tok, got %q", oauth.AccessToken)
	}
}
