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
	AppName       = "cwdecoder"
	ConfigType    = "yaml"
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

# Timing
wpm: 15                 # Initial WPM estimate
adaptive_timing: true   # Adapt to sender's speed

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
	Threshold  float64 `mapstructure:"threshold"`
	Hysteresis int     `mapstructure:"hysteresis"`
	AGCEnabled bool    `mapstructure:"agc_enabled"`
	AGCDecay   float64 `mapstructure:"agc_decay"`
	AGCAttack  float64 `mapstructure:"agc_attack"`

	// Timing
	WPM            int  `mapstructure:"wpm"`
	AdaptiveTiming bool `mapstructure:"adaptive_timing"`

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
	viper.SetDefault("wpm", 15)
	viper.SetDefault("adaptive_timing", true)
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
	if s.SampleRate < 8000 || s.SampleRate > 192000 {
		errs = append(errs, fmt.Errorf("sample_rate must be between 8000 and 192000 Hz, got %v", s.SampleRate))
	}
	if s.Channels < 1 || s.Channels > 2 {
		errs = append(errs, fmt.Errorf("channels must be 1 or 2, got %d", s.Channels))
	}
	if s.BufferSize < 64 || s.BufferSize > 8192 {
		errs = append(errs, fmt.Errorf("buffer_size must be between 64 and 8192, got %d", s.BufferSize))
	}
	// Buffer size should be power of 2 for optimal FFT/Goertzel performance
	if s.BufferSize&(s.BufferSize-1) != 0 {
		errs = append(errs, fmt.Errorf("buffer_size should be a power of 2, got %d", s.BufferSize))
	}

	// Tone detection
	if s.ToneFrequency < 100 || s.ToneFrequency > 3000 {
		errs = append(errs, fmt.Errorf("tone_frequency must be between 100 and 3000 Hz, got %v", s.ToneFrequency))
	}
	if s.BlockSize < 32 || s.BlockSize > 4096 {
		errs = append(errs, fmt.Errorf("block_size must be between 32 and 4096, got %d", s.BlockSize))
	}
	if s.BlockSize&(s.BlockSize-1) != 0 {
		errs = append(errs, fmt.Errorf("block_size should be a power of 2, got %d", s.BlockSize))
	}
	if s.OverlapPct < 0 || s.OverlapPct > 99 {
		errs = append(errs, fmt.Errorf("overlap_pct must be between 0 and 99, got %d", s.OverlapPct))
	}

	// Detection thresholds
	if s.Threshold < 0.0 || s.Threshold > 1.0 {
		errs = append(errs, fmt.Errorf("threshold must be between 0.0 and 1.0, got %v", s.Threshold))
	}
	if s.Hysteresis < 1 || s.Hysteresis > 50 {
		errs = append(errs, fmt.Errorf("hysteresis must be between 1 and 50, got %d", s.Hysteresis))
	}
	if s.AGCDecay < 0.99 || s.AGCDecay > 0.99999 {
		errs = append(errs, fmt.Errorf("agc_decay must be between 0.99 and 0.99999, got %v", s.AGCDecay))
	}
	if s.AGCAttack < 0.0 || s.AGCAttack > 1.0 {
		errs = append(errs, fmt.Errorf("agc_attack must be between 0.0 and 1.0, got %v", s.AGCAttack))
	}

	// Timing
	if s.WPM < 5 || s.WPM > 60 {
		errs = append(errs, fmt.Errorf("wpm must be between 5 and 60, got %d", s.WPM))
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
	if s.ToneFrequency >= s.SampleRate/2 {
		errs = append(errs, fmt.Errorf("tone_frequency (%v Hz) must be less than Nyquist frequency (%v Hz)", s.ToneFrequency, s.SampleRate/2))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
