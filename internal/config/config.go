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

# Audio settings
device_index: -1      # -1 for default device
sample_rate: 48000    # Audio sample rate in Hz
channels: 1           # Number of channels (1=mono)

# Tone detection
tone_frequency: 600   # CW tone frequency in Hz
block_size: 512       # Goertzel block size
overlap_pct: 50       # Block overlap percentage (0-99)

# Detection thresholds
threshold: 0.4        # Detection threshold (0.0-1.0)
hysteresis: 5         # Samples required for state change
agc_enabled: true     # Enable automatic gain control

# Timing
wpm: 15               # Initial WPM estimate
adaptive_timing: true # Adapt to sender's speed

# Output
buffer_size: 64       # Output channel buffer size
debug: false          # Enable debug output
`
)

// Settings holds all application configuration
type Settings struct {
	DeviceIndex    int     `mapstructure:"device_index"`
	SampleRate     float64 `mapstructure:"sample_rate"`
	Channels       int     `mapstructure:"channels"`
	ToneFrequency  float64 `mapstructure:"tone_frequency"`
	BlockSize      int     `mapstructure:"block_size"`
	OverlapPct     int     `mapstructure:"overlap_pct"`
	Threshold      float64 `mapstructure:"threshold"`
	Hysteresis     int     `mapstructure:"hysteresis"`
	AGCEnabled     bool    `mapstructure:"agc_enabled"`
	WPM            int     `mapstructure:"wpm"`
	AdaptiveTiming bool    `mapstructure:"adaptive_timing"`
	BufferSize     int     `mapstructure:"buffer_size"`
	Debug          bool    `mapstructure:"debug"`
}

// Init initializes Viper with defaults and config file
func Init() error {
	// Set defaults
	viper.SetDefault("device_index", -1)
	viper.SetDefault("sample_rate", 48000)
	viper.SetDefault("channels", 1)
	viper.SetDefault("tone_frequency", 600)
	viper.SetDefault("block_size", 512)
	viper.SetDefault("overlap_pct", 50)
	viper.SetDefault("threshold", 0.4)
	viper.SetDefault("hysteresis", 5)
	viper.SetDefault("agc_enabled", true)
	viper.SetDefault("wpm", 15)
	viper.SetDefault("adaptive_timing", true)
	viper.SetDefault("buffer_size", 64)
	viper.SetDefault("debug", false)

	// Config file location
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = os.Getenv("HOME")
	}
	configPath := filepath.Join(configDir, AppName)

	viper.SetConfigName("config")
	viper.SetConfigType(ConfigType)
	viper.AddConfigPath(configPath)
	viper.AddConfigPath(".")

	// Create the default config if it doesn't exist
	if err = ensureConfigExists(configPath); err != nil {
		return err
	}

	// Read the config file
	if err = viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return fmt.Errorf("read config: %w", err)
		}
	}

	return nil
}

func ensureConfigExists(configPath string) error {
	configFile := filepath.Join(configPath, "config.yaml")

	fmt.Println(configFile)

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
