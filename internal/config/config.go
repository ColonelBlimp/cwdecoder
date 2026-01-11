// internal/config/config.go
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	AppName    = "cwdecoder"
	ConfigType = "yaml"

	// Validation range constants
	MinSampleRate    = 8000
	MaxSampleRate    = 192000
	MinChannels      = 1
	MaxChannels      = 2
	MinBufferSize    = 64
	MaxBufferSize    = 8192
	MinToneFrequency = 100
	MaxToneFrequency = 3000
	MinBlockSize     = 32
	MaxBlockSize     = 4096
	MinOverlapPct    = 0
	MaxOverlapPct    = 99
	MinThreshold     = 0.0
	MaxThreshold     = 1.0
	MinHysteresis    = 1
	MaxHysteresis    = 50
	MinAGCDecay      = 0.99
	MaxAGCDecay      = 0.99999
	MinAGCAttack     = 0.0
	MaxAGCAttack     = 1.0
	MinWPM           = 5
	MaxWPM           = 60
	NyquistDivisor   = 2.0 // Nyquist frequency = sample_rate / 2

	// CW Decoder validation constants
	MinAdaptiveSmoothing = 0.0
	MaxAdaptiveSmoothing = 1.0
	MinDitDahBoundary    = 1.0  // Must be > 1 to distinguish dit from dah
	MaxDitDahBoundary    = 4.0  // Reasonable upper limit
	MinCharWordBoundary  = 2.0  // Must be > IntraCharSpaceRatio (1.0)
	MaxCharWordBoundary  = 10.0 // Reasonable upper limit

	DefaultConfig = `# CW Decoder Configuration

# Audio device settings
audio_device: "hw:1,0"  # ALSA device (use 'arecord -l' to find)
device_index: -1        # -1 for default device
sample_rate: 48000      # Audio sample rate in Hz
channels: 1             # Number of channels (1=mono)
format: "S16_LE"        # Audio format (S16_LE = 16-bit signed little-endian)
buffer_size: 1024       # Audio buffer size

# Tone detection
tone_frequency: 600     # CW tone frequency in Hz
block_size: 512         # Goertzel block size (samples per detection window)
overlap_pct: 50         # Block overlap percentage (0-99), higher = smoother but more CPU

# Detection thresholds
threshold: 0.4          # Detection threshold (0.0-1.0), tone magnitude must exceed this
hysteresis: 5           # Consecutive blocks required to confirm state change (reduces noise)
agc_enabled: true       # Enable automatic gain control (normalizes input levels)
agc_decay: 0.9995       # AGC peak decay rate per sample (0.999-0.99999)
                        # Lower = faster decay (~0.999 = 20ms), Higher = slower (~0.9999 = 200ms)
                        # At 48kHz: 0.9995 gives ~100ms decay time constant
agc_attack: 0.1         # AGC attack rate (0.0-1.0), how fast to respond to louder signals
                        # Higher = faster response, Lower = more gradual
agc_warmup_blocks: 10   # Number of blocks to process before enabling detection
                        # Allows AGC to calibrate to signal level, preventing false triggers

# CW Timing
wpm: 15                 # Initial WPM estimate (5-60)
adaptive_timing: true   # Adapt to sender's speed automatically
adaptive_smoothing: 0.1 # EMA smoothing factor for timing adaptation (0.0-1.0)
                        # Higher = faster adaptation to speed changes
                        # Lower = more stable, resistant to timing errors
dit_dah_boundary: 2.0   # Threshold ratio between dit and dah (typically 2.0)
                        # Tone > (dit_duration * dit_dah_boundary) is classified as dah
char_word_boundary: 5.0 # Threshold ratio between character and word space (typically 5.0)
                        # Space > (dit_duration * char_word_boundary) is word space
farnsworth_wpm: 0       # Effective WPM for character spacing (0 = same as wpm)
                        # Set lower than wpm to stretch spacing for easier copy

# Output
debug: false            # Enable debug output
`
)

// Settings holds all application configuration
type Settings struct {
	// Audio device settings
	AudioDevice string  `mapstructure:"audio_device"`
	DeviceIndex int     `mapstructure:"device_index"`
	SampleRate  float64 `mapstructure:"sample_rate"`
	Channels    int     `mapstructure:"channels"`
	Format      string  `mapstructure:"format"`
	BufferSize  int     `mapstructure:"buffer_size"`

	// Tone detection
	ToneFrequency float64 `mapstructure:"tone_frequency"`
	BlockSize     int     `mapstructure:"block_size"`
	OverlapPct    int     `mapstructure:"overlap_pct"`

	// Detection thresholds
	Threshold       float64 `mapstructure:"threshold"`
	Hysteresis      int     `mapstructure:"hysteresis"`
	AGCEnabled      bool    `mapstructure:"agc_enabled"`
	AGCDecay        float64 `mapstructure:"agc_decay"`
	AGCAttack       float64 `mapstructure:"agc_attack"`
	AGCWarmupBlocks int     `mapstructure:"agc_warmup_blocks"`

	// CW Timing
	WPM               int     `mapstructure:"wpm"`
	AdaptiveTiming    bool    `mapstructure:"adaptive_timing"`
	AdaptiveSmoothing float64 `mapstructure:"adaptive_smoothing"`
	DitDahBoundary    float64 `mapstructure:"dit_dah_boundary"`
	CharWordBoundary  float64 `mapstructure:"char_word_boundary"`
	FarnsworthWPM     int     `mapstructure:"farnsworth_wpm"`

	// Output
	Debug bool `mapstructure:"debug"`
}

// Init initializes Viper with defaults and config file.
// Config file search order: current directory, then ~/.config/cwdecoder/
func Init() error {
	// Set defaults
	viper.SetDefault("audio_device", "hw:1,0")
	viper.SetDefault("device_index", -1)
	viper.SetDefault("sample_rate", 48000)
	viper.SetDefault("channels", 1)
	viper.SetDefault("format", "S16_LE")
	viper.SetDefault("buffer_size", 1024)
	viper.SetDefault("tone_frequency", 600)
	viper.SetDefault("block_size", 512)
	viper.SetDefault("overlap_pct", 50)
	viper.SetDefault("threshold", 0.4)
	viper.SetDefault("hysteresis", 5)
	viper.SetDefault("agc_enabled", true)
	viper.SetDefault("agc_decay", 0.9995)
	viper.SetDefault("agc_attack", 0.1)
	viper.SetDefault("agc_warmup_blocks", 10)
	viper.SetDefault("wpm", 15)
	viper.SetDefault("adaptive_timing", true)
	viper.SetDefault("adaptive_smoothing", 0.1)
	viper.SetDefault("dit_dah_boundary", 2.0)
	viper.SetDefault("char_word_boundary", 5.0)
	viper.SetDefault("farnsworth_wpm", 0)
	viper.SetDefault("debug", false)

	// Support both config.yaml and .config.yaml
	viper.SetConfigType(ConfigType)

	// Priority order: current directory first, then XDG config
	viper.AddConfigPath(".")

	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	viper.AddConfigPath(filepath.Join(configDir, AppName))

	// Try .config.yaml first (hidden file), then config.yaml
	viper.SetConfigName(".config")
	if err = viper.ReadInConfig(); err != nil {
		// Try config.yaml as fallback
		viper.SetConfigName("config")
		err = viper.ReadInConfig()
	}

	// Read config file - if not found, create default in XDG config dir
	if err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			// No config found - create default in ~/.config/cwdecoder/
			xdgConfigPath := filepath.Join(configDir, AppName)
			if err = ensureConfigExists(xdgConfigPath); err != nil {
				return err
			}
			// Read the newly created config
			if err = viper.ReadInConfig(); err != nil {
				return fmt.Errorf("read config: %w", err)
			}
		} else {
			return fmt.Errorf("read config: %w", err)
		}
	}

	return nil
}

func ensureConfigExists(configPath string) error {
	configFile := filepath.Join(configPath, "config.yaml")

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		if err = os.MkdirAll(configPath, 0755); err != nil {
			return fmt.Errorf("create config dir: %w", err)
		}
		if err = os.WriteFile(configFile, []byte(DefaultConfig), 0644); err != nil {
			return fmt.Errorf("write default config: %w", err)
		}
	}
	return nil
}

// Get returns the current settings
func Get() (*Settings, error) {
	var s Settings
	if err := viper.Unmarshal(&s); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &s, nil
}

// Validate checks that all settings are within acceptable ranges
func (s *Settings) Validate() error {
	var errs []error

	// Audio device settings
	if s.SampleRate < MinSampleRate || s.SampleRate > MaxSampleRate {
		errs = append(errs, fmt.Errorf("sample_rate must be between %d and %d Hz, got %v", MinSampleRate, MaxSampleRate, s.SampleRate))
	}
	if s.Channels < MinChannels || s.Channels > MaxChannels {
		errs = append(errs, fmt.Errorf("channels must be %d or %d, got %d", MinChannels, MaxChannels, s.Channels))
	}
	if s.BufferSize < MinBufferSize || s.BufferSize > MaxBufferSize {
		errs = append(errs, fmt.Errorf("buffer_size must be between %d and %d, got %d", MinBufferSize, MaxBufferSize, s.BufferSize))
	}
	// Buffer size should be power of 2 for optimal FFT/Goertzel performance
	if s.BufferSize&(s.BufferSize-1) != 0 {
		errs = append(errs, fmt.Errorf("buffer_size should be a power of 2, got %d", s.BufferSize))
	}

	// Tone detection
	if s.ToneFrequency < MinToneFrequency || s.ToneFrequency > MaxToneFrequency {
		errs = append(errs, fmt.Errorf("tone_frequency must be between %d and %d Hz, got %v", MinToneFrequency, MaxToneFrequency, s.ToneFrequency))
	}
	if s.BlockSize < MinBlockSize || s.BlockSize > MaxBlockSize {
		errs = append(errs, fmt.Errorf("block_size must be between %d and %d, got %d", MinBlockSize, MaxBlockSize, s.BlockSize))
	}
	if s.BlockSize&(s.BlockSize-1) != 0 {
		errs = append(errs, fmt.Errorf("block_size should be a power of 2, got %d", s.BlockSize))
	}
	if s.OverlapPct < MinOverlapPct || s.OverlapPct > MaxOverlapPct {
		errs = append(errs, fmt.Errorf("overlap_pct must be between %d and %d, got %d", MinOverlapPct, MaxOverlapPct, s.OverlapPct))
	}

	// Detection thresholds
	if s.Threshold < MinThreshold || s.Threshold > MaxThreshold {
		errs = append(errs, fmt.Errorf("threshold must be between %.1f and %.1f, got %v", MinThreshold, MaxThreshold, s.Threshold))
	}
	if s.Hysteresis < MinHysteresis || s.Hysteresis > MaxHysteresis {
		errs = append(errs, fmt.Errorf("hysteresis must be between %d and %d, got %d", MinHysteresis, MaxHysteresis, s.Hysteresis))
	}
	if s.AGCDecay < MinAGCDecay || s.AGCDecay > MaxAGCDecay {
		errs = append(errs, fmt.Errorf("agc_decay must be between %.2f and %.5f, got %v", MinAGCDecay, MaxAGCDecay, s.AGCDecay))
	}
	if s.AGCAttack < MinAGCAttack || s.AGCAttack > MaxAGCAttack {
		errs = append(errs, fmt.Errorf("agc_attack must be between %.1f and %.1f, got %v", MinAGCAttack, MaxAGCAttack, s.AGCAttack))
	}

	// Timing
	if s.WPM < MinWPM || s.WPM > MaxWPM {
		errs = append(errs, fmt.Errorf("wpm must be between %d and %d, got %d", MinWPM, MaxWPM, s.WPM))
	}
	if s.AdaptiveSmoothing < MinAdaptiveSmoothing || s.AdaptiveSmoothing > MaxAdaptiveSmoothing {
		errs = append(errs, fmt.Errorf("adaptive_smoothing must be between %.1f and %.1f, got %v", MinAdaptiveSmoothing, MaxAdaptiveSmoothing, s.AdaptiveSmoothing))
	}
	if s.DitDahBoundary < MinDitDahBoundary || s.DitDahBoundary > MaxDitDahBoundary {
		errs = append(errs, fmt.Errorf("dit_dah_boundary must be between %.1f and %.1f, got %v", MinDitDahBoundary, MaxDitDahBoundary, s.DitDahBoundary))
	}
	if s.CharWordBoundary < MinCharWordBoundary || s.CharWordBoundary > MaxCharWordBoundary {
		errs = append(errs, fmt.Errorf("char_word_boundary must be between %.1f and %.1f, got %v", MinCharWordBoundary, MaxCharWordBoundary, s.CharWordBoundary))
	}
	if s.FarnsworthWPM < 0 || s.FarnsworthWPM > s.WPM {
		errs = append(errs, fmt.Errorf("farnsworth_wpm must be between 0 and wpm (%d), got %d", s.WPM, s.FarnsworthWPM))
	}

	// Validate audio format
	validFormats := map[string]bool{
		"S16_LE": true,
		"S16_BE": true,
		"S24_LE": true,
		"S24_BE": true,
		"S32_LE": true,
		"S32_BE": true,
		"F32_LE": true,
		"F32_BE": true,
	}
	if !validFormats[s.Format] {
		errs = append(errs, fmt.Errorf("format must be one of S16_LE, S16_BE, S24_LE, S24_BE, S32_LE, S32_BE, F32_LE, F32_BE, got %q", s.Format))
	}

	// Nyquist check: tone frequency must be less than half the sample rate
	if s.ToneFrequency >= s.SampleRate/NyquistDivisor {
		errs = append(errs, fmt.Errorf("tone_frequency (%v Hz) must be less than Nyquist frequency (%v Hz)", s.ToneFrequency, s.SampleRate/NyquistDivisor))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
