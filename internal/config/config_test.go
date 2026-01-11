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
		{"agc_warmup_blocks", 10},
		{"wpm", 15},
		{"adaptive_timing", true},
		{"adaptive_smoothing", 0.1},
		{"dit_dah_boundary", 2.0},
		{"inter_char_boundary", 2.0},
		{"char_word_boundary", 5.0},
		{"farnsworth_wpm", 0},
		{"buffer_size", 1024},
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

func TestInit_LoadsDotConfigYaml(t *testing.T) {
	resetViper()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Change to temp directory
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("failed to restore dir: %v", err)
		}
	}()

	// Create .config.yaml (hidden config file)
	dotConfigContent := `audio_device: "hw:1,0"
sample_rate: 48000
channels: 1
format: "S16_LE"
buffer_size: 1024
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".config.yaml"), []byte(dotConfigContent), 0644); err != nil {
		t.Fatalf("failed to write .config.yaml: %v", err)
	}

	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Verify audio settings are loaded
	tests := []struct {
		key      string
		expected interface{}
	}{
		{"audio_device", "hw:1,0"},
		{"sample_rate", 48000},
		{"channels", 1},
		{"format", "S16_LE"},
		{"buffer_size", 1024},
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

func TestInit_DotConfigTakesPrecedence(t *testing.T) {
	resetViper()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Change to temp directory
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("failed to restore dir: %v", err)
		}
	}()

	// Create both .config.yaml and config.yaml
	if err := os.WriteFile(filepath.Join(tmpDir, ".config.yaml"), []byte("wpm: 30"), 0644); err != nil {
		t.Fatalf("failed to write .config.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte("wpm: 20"), 0644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// .config.yaml should take precedence
	if got := viper.GetInt("wpm"); got != 30 {
		t.Errorf("viper.GetInt(wpm) = %d, want 30 (.config.yaml should take precedence)", got)
	}
}

func TestGet_ReturnsAudioSettings(t *testing.T) {
	resetViper()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Change to temp directory
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("failed to restore dir: %v", err)
		}
	}()

	// Create .config.yaml with audio settings
	configContent := `audio_device: "hw:2,0"
sample_rate: 44100
channels: 2
format: "S32_LE"
buffer_size: 2048
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write .config.yaml: %v", err)
	}

	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	settings, err := Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if settings.AudioDevice != "hw:2,0" {
		t.Errorf("Settings.AudioDevice = %s, want hw:2,0", settings.AudioDevice)
	}
	if settings.SampleRate != 44100 {
		t.Errorf("Settings.SampleRate = %f, want 44100", settings.SampleRate)
	}
	if settings.Channels != 2 {
		t.Errorf("Settings.Channels = %d, want 2", settings.Channels)
	}
	if settings.Format != "S32_LE" {
		t.Errorf("Settings.Format = %s, want S32_LE", settings.Format)
	}
	if settings.BufferSize != 2048 {
		t.Errorf("Settings.BufferSize = %d, want 2048", settings.BufferSize)
	}
}

// Validation tests

func TestSettings_Validate_ValidSettings(t *testing.T) {
	s := &Settings{
		AudioDevice:            "hw:1,0",
		DeviceIndex:            -1,
		SampleRate:             48000,
		Channels:               1,
		Format:                 "S16_LE",
		BufferSize:             1024,
		ToneFrequency:          600,
		BlockSize:              512,
		OverlapPct:             50,
		Threshold:              0.4,
		Hysteresis:             5,
		AGCEnabled:             true,
		AGCDecay:               0.9995,
		AGCAttack:              0.1,
		AGCWarmupBlocks:        10,
		WPM:                    15,
		AdaptiveTiming:         true,
		AdaptiveSmoothing:      0.1,
		DitDahBoundary:         2.0,
		InterCharBoundary:      2.0,
		CharWordBoundary:       5.0,
		FarnsworthWPM:          0,
		AdaptivePatternEnabled: true,
		AdaptiveMinConfidence:  0.7,
		AdaptiveAdjustmentRate: 0.1,
		AdaptiveMinMatches:     3,
		Debug:                  false,
	}

	if err := s.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil for valid settings", err)
	}
}

func TestSettings_Validate_SampleRate(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate float64
		wantErr    bool
	}{
		{"too low", 7999, true},
		{"minimum", 8000, false},
		{"typical 44100", 44100, false},
		{"typical 48000", 48000, false},
		{"high 96000", 96000, false},
		{"maximum", 192000, false},
		{"too high", 192001, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSettings()
			s.SampleRate = tt.sampleRate
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSettings_Validate_Channels(t *testing.T) {
	tests := []struct {
		name     string
		channels int
		wantErr  bool
	}{
		{"zero", 0, true},
		{"mono", 1, false},
		{"stereo", 2, false},
		{"too many", 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSettings()
			s.Channels = tt.channels
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSettings_Validate_BufferSize(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize int
		wantErr    bool
	}{
		{"too small", 32, true},
		{"minimum", 64, false},
		{"typical 512", 512, false},
		{"typical 1024", 1024, false},
		{"maximum", 8192, false},
		{"too large", 8193, true},
		{"not power of 2", 100, true},
		{"not power of 2 large", 1000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSettings()
			s.BufferSize = tt.bufferSize
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSettings_Validate_ToneFrequency(t *testing.T) {
	tests := []struct {
		name          string
		toneFrequency float64
		wantErr       bool
	}{
		{"too low", 99, true},
		{"minimum", 100, false},
		{"typical 600", 600, false},
		{"typical 700", 700, false},
		{"maximum", 3000, false},
		{"too high", 3001, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSettings()
			s.ToneFrequency = tt.toneFrequency
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSettings_Validate_BlockSize(t *testing.T) {
	tests := []struct {
		name      string
		blockSize int
		wantErr   bool
	}{
		{"too small", 16, true},
		{"minimum", 32, false},
		{"typical 256", 256, false},
		{"typical 512", 512, false},
		{"maximum", 4096, false},
		{"too large", 4097, true},
		{"not power of 2", 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSettings()
			s.BlockSize = tt.blockSize
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSettings_Validate_OverlapPct(t *testing.T) {
	tests := []struct {
		name       string
		overlapPct int
		wantErr    bool
	}{
		{"negative", -1, true},
		{"zero", 0, false},
		{"typical 50", 50, false},
		{"high 75", 75, false},
		{"maximum", 99, false},
		{"too high", 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSettings()
			s.OverlapPct = tt.overlapPct
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSettings_Validate_Threshold(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		wantErr   bool
	}{
		{"negative", -0.1, true},
		{"zero", 0.0, false},
		{"typical", 0.4, false},
		{"maximum", 1.0, false},
		{"too high", 1.1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSettings()
			s.Threshold = tt.threshold
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSettings_Validate_Hysteresis(t *testing.T) {
	tests := []struct {
		name       string
		hysteresis int
		wantErr    bool
	}{
		{"zero", 0, true},
		{"minimum", 1, false},
		{"typical", 5, false},
		{"maximum", 50, false},
		{"too high", 51, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSettings()
			s.Hysteresis = tt.hysteresis
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSettings_Validate_AGCDecay(t *testing.T) {
	tests := []struct {
		name     string
		agcDecay float64
		wantErr  bool
	}{
		{"too low", 0.989, true},
		{"minimum", 0.99, false},
		{"typical", 0.9995, false},
		{"maximum", 0.99999, false},
		{"too high", 0.999991, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSettings()
			s.AGCDecay = tt.agcDecay
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSettings_Validate_AGCAttack(t *testing.T) {
	tests := []struct {
		name      string
		agcAttack float64
		wantErr   bool
	}{
		{"negative", -0.1, true},
		{"zero", 0.0, false},
		{"typical", 0.1, false},
		{"maximum", 1.0, false},
		{"too high", 1.1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSettings()
			s.AGCAttack = tt.agcAttack
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSettings_Validate_WPM(t *testing.T) {
	tests := []struct {
		name    string
		wpm     int
		wantErr bool
	}{
		{"too slow", 4, true},
		{"minimum", 5, false},
		{"typical", 15, false},
		{"fast", 30, false},
		{"maximum", 60, false},
		{"too fast", 61, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSettings()
			s.WPM = tt.wpm
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSettings_Validate_Format(t *testing.T) {
	validFormats := []string{"S16_LE", "S16_BE", "S24_LE", "S24_BE", "S32_LE", "S32_BE", "F32_LE", "F32_BE"}
	invalidFormats := []string{"", "invalid", "S8", "U16_LE", "FLOAT"}

	for _, format := range validFormats {
		t.Run("valid_"+format, func(t *testing.T) {
			s := validSettings()
			s.Format = format
			if err := s.Validate(); err != nil {
				t.Errorf("Validate() error = %v for valid format %q", err, format)
			}
		})
	}

	for _, format := range invalidFormats {
		t.Run("invalid_"+format, func(t *testing.T) {
			s := validSettings()
			s.Format = format
			if err := s.Validate(); err == nil {
				t.Errorf("Validate() should error for invalid format %q", format)
			}
		})
	}
}

func TestSettings_Validate_NyquistFrequency(t *testing.T) {
	tests := []struct {
		name          string
		sampleRate    float64
		toneFrequency float64
		wantErr       bool
	}{
		{"well below nyquist", 48000, 600, false},
		{"near max tone freq", 48000, 3000, false},
		{"at nyquist low sample", 8000, 4000, true},
		{"above nyquist low sample", 8000, 5000, true},
		{"low sample rate valid", 8000, 3000, false},
		{"tone above nyquist", 10000, 6000, true}, // 6000 Hz > 5000 Hz (Nyquist for 10kHz)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSettings()
			s.SampleRate = tt.sampleRate
			s.ToneFrequency = tt.toneFrequency
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSettings_Validate_MultipleErrors(t *testing.T) {
	s := &Settings{
		SampleRate:        0,     // invalid
		Channels:          0,     // invalid
		BufferSize:        10,    // invalid
		ToneFrequency:     0,     // invalid
		BlockSize:         10,    // invalid
		OverlapPct:        -1,    // invalid
		Threshold:         2.0,   // invalid
		Hysteresis:        0,     // invalid
		AGCDecay:          0.5,   // invalid
		AGCAttack:         2.0,   // invalid
		WPM:               0,     // invalid
		Format:            "bad", // invalid
		AdaptiveSmoothing: 2.0,   // invalid
		DitDahBoundary:    0.5,   // invalid
		InterCharBoundary: 0.5,   // invalid
		CharWordBoundary:  0.5,   // invalid
		FarnsworthWPM:     100,   // invalid (> WPM)
	}

	err := s.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for multiple invalid fields")
	}

	// Should contain multiple error messages
	errStr := err.Error()
	expectedSubstrings := []string{
		"sample_rate",
		"channels",
		"buffer_size",
		"tone_frequency",
		"block_size",
		"overlap_pct",
		"threshold",
		"hysteresis",
		"agc_decay",
		"agc_attack",
		"wpm",
		"format",
		"adaptive_smoothing",
		"dit_dah_boundary",
		"inter_char_boundary",
		"char_word_boundary",
		"farnsworth_wpm",
	}

	for _, substr := range expectedSubstrings {
		if !contains(errStr, substr) {
			t.Errorf("Validate() error should mention %q, got: %v", substr, errStr)
		}
	}
}

// validSettings returns a Settings struct with all valid values
func validSettings() *Settings {
	return &Settings{
		AudioDevice:            "hw:1,0",
		DeviceIndex:            -1,
		SampleRate:             48000,
		Channels:               1,
		Format:                 "S16_LE",
		BufferSize:             1024,
		ToneFrequency:          600,
		BlockSize:              512,
		OverlapPct:             50,
		Threshold:              0.4,
		Hysteresis:             5,
		AGCEnabled:             true,
		AGCDecay:               0.9995,
		AGCAttack:              0.1,
		AGCWarmupBlocks:        10,
		WPM:                    15,
		AdaptiveTiming:         true,
		AdaptiveSmoothing:      0.1,
		DitDahBoundary:         2.0,
		InterCharBoundary:      2.0,
		CharWordBoundary:       5.0,
		FarnsworthWPM:          0,
		AdaptivePatternEnabled: true,
		AdaptiveMinConfidence:  0.7,
		AdaptiveAdjustmentRate: 0.1,
		AdaptiveMinMatches:     3,
		Debug:                  false,
	}
}
