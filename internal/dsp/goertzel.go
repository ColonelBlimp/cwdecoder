// internal/dsp/goertzel.go
package dsp

import (
	"errors"
	"math"
)

var (
	// ErrInvalidBlockSize indicates block size must be positive
	ErrInvalidBlockSize = errors.New("block size must be positive")
	// ErrInvalidSampleRate indicates sample rate must be positive
	ErrInvalidSampleRate = errors.New("sample rate must be positive")
	// ErrInvalidFrequency indicates frequency must be positive and below Nyquist
	ErrInvalidFrequency = errors.New("target frequency must be positive and less than Nyquist frequency")
	// ErrInsufficientSamples indicates not enough samples for the configured block size
	ErrInsufficientSamples = errors.New("insufficient samples for block size")
)

// GoertzelConfig holds configuration for the Goertzel algorithm.
// All values should come from the application config file.
type GoertzelConfig struct {
	// TargetFrequency is the frequency to detect in Hz (from config: tone_frequency)
	TargetFrequency float64
	// SampleRate is the audio sample rate in Hz (from config: sample_rate)
	SampleRate float64
	// BlockSize is the number of samples per detection window (from config: block_size)
	BlockSize int
}

// Goertzel implements the Goertzel algorithm for efficient single-frequency detection.
// The Goertzel algorithm is more efficient than FFT when detecting only one or a few
// frequencies, as it computes the DFT for a single frequency bin.
type Goertzel struct {
	config      GoertzelConfig
	coefficient float64 // Pre-computed: 2 * cos(2π * k / N)
	normalizer  float64 // Pre-computed: 2.0 / blockSize for magnitude scaling
	sine        float64 // Pre-computed: sin(2π * k / N) for phase calculation
	cosine      float64 // Pre-computed: cos(2π * k / N)
}

// NewGoertzel creates a new Goertzel detector with the given configuration.
// Returns an error if the configuration is invalid.
func NewGoertzel(cfg GoertzelConfig) (*Goertzel, error) {
	if cfg.BlockSize <= 0 {
		return nil, ErrInvalidBlockSize
	}
	if cfg.SampleRate <= 0 {
		return nil, ErrInvalidSampleRate
	}
	nyquist := cfg.SampleRate / 2.0
	if cfg.TargetFrequency <= 0 || cfg.TargetFrequency >= nyquist {
		return nil, ErrInvalidFrequency
	}

	// Compute the normalized frequency index k
	// k = (targetFrequency / sampleRate) * blockSize
	k := (cfg.TargetFrequency / cfg.SampleRate) * float64(cfg.BlockSize)

	// Pre-compute trigonometric values
	omega := (2.0 * math.Pi * k) / float64(cfg.BlockSize)
	cosine := math.Cos(omega)
	sine := math.Sin(omega)

	// Goertzel coefficient: 2 * cos(omega)
	coefficient := 2.0 * cosine

	// Normalizer for magnitude (accounts for block size)
	normalizer := 2.0 / float64(cfg.BlockSize)

	return &Goertzel{
		config:      cfg,
		coefficient: coefficient,
		normalizer:  normalizer,
		sine:        sine,
		cosine:      cosine,
	}, nil
}

// Magnitude computes the magnitude of the target frequency in the given samples.
// Returns the normalized magnitude. For normalized input (-1.0 to 1.0), a pure
// sine wave at the target frequency will return approximately 1.0.
// The samples slice must have at least BlockSize elements.
func (g *Goertzel) Magnitude(samples []float32) (float64, error) {
	if len(samples) < g.config.BlockSize {
		return 0, ErrInsufficientSamples
	}

	return g.computeMagnitude(samples), nil
}

// MagnitudeNoAlloc computes magnitude without bounds checking for hot path usage.
// Caller MUST ensure samples has at least BlockSize elements.
// This method is optimized for real-time audio processing.
func (g *Goertzel) MagnitudeNoAlloc(samples []float32) float64 {
	return g.computeMagnitude(samples)
}

// computeMagnitude is the core Goertzel algorithm implementation.
func (g *Goertzel) computeMagnitude(samples []float32) float64 {
	var s0, s1, s2 float64
	blockSize := g.config.BlockSize
	coeff := g.coefficient

	// Goertzel iteration - processes samples one at a time
	for i := 0; i < blockSize; i++ {
		s0 = float64(samples[i]) + coeff*s1 - s2
		s2 = s1
		s1 = s0
	}

	// Compute power at the target frequency using the final state
	// power = s1² + s2² - coefficient * s1 * s2
	power := s1*s1 + s2*s2 - coeff*s1*s2

	// Guard against floating point errors causing negative values
	if power < 0 {
		power = 0
	}

	return math.Sqrt(power) * g.normalizer
}

// Config returns the current configuration (for testing and inspection)
func (g *Goertzel) Config() GoertzelConfig {
	return g.config
}

// Coefficient returns the pre-computed Goertzel coefficient (for testing)
func (g *Goertzel) Coefficient() float64 {
	return g.coefficient
}

// BlockSize returns the configured block size
func (g *Goertzel) BlockSize() int {
	return g.config.BlockSize
}
