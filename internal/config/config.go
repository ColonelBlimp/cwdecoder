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
	return &s, nil
}
