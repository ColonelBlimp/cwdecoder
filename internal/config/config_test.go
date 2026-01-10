package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func resetViper() {
	viper.Reset()
}

func TestInit_WithDefaults(t *testing.T) {
	resetViper()

	// Use a temp directory to avoid polluting real config
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create the config file so Init doesn't try to create one
	configDir := filepath.Join(tmpDir, ".config", AppName)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(DefaultConfig), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Check defaults are set
	tests := []struct {
		key      string
		expected interface{}
	}{
		{"device_index", -1},
		{"sample_rate", 48000},
		{"channels", 1},
		{"tone_frequency", 600},
		{"block_size", 512},
		{"overlap_pct", 50},
		{"threshold", 0.4},
		{"hysteresis", 5},
		{"agc_enabled", true},
		{"wpm", 15},
		{"adaptive_timing", true},
		{"buffer_size", 64},
		{"debug", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := viper.Get(tt.key)
			if got != tt.expected {
				t.Errorf("viper.Get(%q) = %v, want %v", tt.key, got, tt.expected)
			}
		})
	}
}

func TestInit_CreatesConfigIfMissing(t *testing.T) {
	resetViper()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Don't create config - let Init create it
	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Verify config was created
	configPath := filepath.Join(tmpDir, ".config", AppName, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("Init() did not create config file at %s", configPath)
	}
}

func TestInit_ReadsLocalConfigFirst(t *testing.T) {
	resetViper()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create XDG config
	xdgConfigDir := filepath.Join(tmpDir, ".config", AppName)
	if err := os.MkdirAll(xdgConfigDir, 0755); err != nil {
		t.Fatalf("failed to create XDG config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(xdgConfigDir, "config.yaml"), []byte("wpm: 20"), 0644); err != nil {
		t.Fatalf("failed to write XDG config: %v", err)
	}

	// Create local config with different value
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("failed to restore dir: %v", err)
		}
	}()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte("wpm: 25"), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}

	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Local config should take precedence
	if got := viper.GetInt("wpm"); got != 25 {
		t.Errorf("viper.GetInt(wpm) = %d, want 25 (local config)", got)
	}
}

func TestGet_ReturnsSettings(t *testing.T) {
	resetViper()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".config", AppName)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(DefaultConfig), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	settings, err := Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if settings.DeviceIndex != -1 {
		t.Errorf("Settings.DeviceIndex = %d, want -1", settings.DeviceIndex)
	}
	if settings.SampleRate != 48000 {
		t.Errorf("Settings.SampleRate = %f, want 48000", settings.SampleRate)
	}
	if settings.Channels != 1 {
		t.Errorf("Settings.Channels = %d, want 1", settings.Channels)
	}
	if settings.ToneFrequency != 600 {
		t.Errorf("Settings.ToneFrequency = %f, want 600", settings.ToneFrequency)
	}
	if settings.WPM != 15 {
		t.Errorf("Settings.WPM = %d, want 15", settings.WPM)
	}
	if settings.Debug != false {
		t.Errorf("Settings.Debug = %v, want false", settings.Debug)
	}
}

func TestGet_AllFields(t *testing.T) {
	resetViper()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	customConfig := `device_index: 2
sample_rate: 96000
channels: 2
tone_frequency: 700
block_size: 1024
overlap_pct: 75
threshold: 0.6
hysteresis: 10
agc_enabled: false
wpm: 25
adaptive_timing: false
buffer_size: 128
debug: true
`

	configDir := filepath.Join(tmpDir, ".config", AppName)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(customConfig), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	settings, err := Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if settings.DeviceIndex != 2 {
		t.Errorf("Settings.DeviceIndex = %d, want 2", settings.DeviceIndex)
	}
	if settings.SampleRate != 96000 {
		t.Errorf("Settings.SampleRate = %f, want 96000", settings.SampleRate)
	}
	if settings.Channels != 2 {
		t.Errorf("Settings.Channels = %d, want 2", settings.Channels)
	}
	if settings.ToneFrequency != 700 {
		t.Errorf("Settings.ToneFrequency = %f, want 700", settings.ToneFrequency)
	}
	if settings.BlockSize != 1024 {
		t.Errorf("Settings.BlockSize = %d, want 1024", settings.BlockSize)
	}
	if settings.OverlapPct != 75 {
		t.Errorf("Settings.OverlapPct = %d, want 75", settings.OverlapPct)
	}
	if settings.Threshold != 0.6 {
		t.Errorf("Settings.Threshold = %f, want 0.6", settings.Threshold)
	}
	if settings.Hysteresis != 10 {
		t.Errorf("Settings.Hysteresis = %d, want 10", settings.Hysteresis)
	}
	if settings.AGCEnabled != false {
		t.Errorf("Settings.AGCEnabled = %v, want false", settings.AGCEnabled)
	}
	if settings.WPM != 25 {
		t.Errorf("Settings.WPM = %d, want 25", settings.WPM)
	}
	if settings.AdaptiveTiming != false {
		t.Errorf("Settings.AdaptiveTiming = %v, want false", settings.AdaptiveTiming)
	}
	if settings.BufferSize != 128 {
		t.Errorf("Settings.BufferSize = %d, want 128", settings.BufferSize)
	}
	if settings.Debug != true {
		t.Errorf("Settings.Debug = %v, want true", settings.Debug)
	}
}

func TestEnsureConfigExists_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "subdir", "config")

	if err := ensureConfigExists(configPath); err != nil {
		t.Fatalf("ensureConfigExists() error = %v", err)
	}

	configFile := filepath.Join(configPath, "config.yaml")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Errorf("ensureConfigExists() did not create %s", configFile)
	}

	// Verify content
	content, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if string(content) != DefaultConfig {
		t.Errorf("config content does not match DefaultConfig")
	}
}

func TestEnsureConfigExists_DoesNotOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir

	configFile := filepath.Join(configPath, "config.yaml")
	existingContent := "existing: true"
	if err := os.WriteFile(configFile, []byte(existingContent), 0644); err != nil {
		t.Fatalf("failed to write existing config: %v", err)
	}

	if err := ensureConfigExists(configPath); err != nil {
		t.Fatalf("ensureConfigExists() error = %v", err)
	}

	// Verify content was not overwritten
	content, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if string(content) != existingContent {
		t.Errorf("ensureConfigExists() overwrote existing config")
	}
}

func TestConstants(t *testing.T) {
	if AppName != "cwdecoder" {
		t.Errorf("AppName = %q, want %q", AppName, "cwdecoder")
	}
	if ConfigType != "yaml" {
		t.Errorf("ConfigType = %q, want %q", ConfigType, "yaml")
	}
}

func TestDefaultConfig_ContainsExpectedKeys(t *testing.T) {
	expectedKeys := []string{
		"device_index",
		"sample_rate",
		"channels",
		"tone_frequency",
		"block_size",
		"overlap_pct",
		"threshold",
		"hysteresis",
		"agc_enabled",
		"wpm",
		"adaptive_timing",
		"buffer_size",
		"debug",
	}

	for _, key := range expectedKeys {
		if !contains(DefaultConfig, key) {
			t.Errorf("DefaultConfig missing key: %s", key)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsString(s, substr))
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSettings_Struct(t *testing.T) {
	s := Settings{
		DeviceIndex:    1,
		SampleRate:     96000,
		Channels:       2,
		ToneFrequency:  700,
		BlockSize:      1024,
		OverlapPct:     75,
		Threshold:      0.5,
		Hysteresis:     10,
		AGCEnabled:     false,
		WPM:            20,
		AdaptiveTiming: false,
		BufferSize:     128,
		Debug:          true,
	}

	if s.DeviceIndex != 1 {
		t.Errorf("Settings.DeviceIndex = %d, want 1", s.DeviceIndex)
	}
	if s.SampleRate != 96000 {
		t.Errorf("Settings.SampleRate = %f, want 96000", s.SampleRate)
	}
	if s.ToneFrequency != 700 {
		t.Errorf("Settings.ToneFrequency = %f, want 700", s.ToneFrequency)
	}
	if s.Debug != true {
		t.Errorf("Settings.Debug = %v, want true", s.Debug)
	}
}

func TestInit_InvalidConfigFile(t *testing.T) {
	resetViper()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create invalid YAML config
	configDir := filepath.Join(tmpDir, ".config", AppName)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	invalidYAML := "invalid: yaml: content: [[["
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("failed to write invalid config: %v", err)
	}

	err := Init()
	if err == nil {
		t.Error("Init() should return error for invalid YAML")
	}
}

func TestEnsureConfigExists_WriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping test when running as root")
	}

	tmpDir := t.TempDir()

	// Create a read-only directory
	configPath := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(configPath, 0555); err != nil {
		t.Fatalf("failed to create readonly dir: %v", err)
	}
	defer func() {
		// Restore write permission for cleanup
		if err := os.Chmod(configPath, 0755); err != nil {
			t.Logf("failed to restore permissions: %v", err)
		}
	}()

	// Try to create config in a subdirectory of the read-only directory
	err := ensureConfigExists(filepath.Join(configPath, "subdir"))
	if err == nil {
		t.Error("ensureConfigExists() should return error for read-only directory")
	}
}
