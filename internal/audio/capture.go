// internal/audio/capture.go
package audio

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/gen2brain/malgo"
)

var (
	ErrNotInitialized = errors.New("audio capture not initialized")
	ErrAlreadyRunning = errors.New("audio capture already running")
	ErrNotRunning     = errors.New("audio capture not running")
)

// Config holds audio capture configuration
type Config struct {
	DeviceIndex int    // -1 for default device
	SampleRate  uint32 // e.g., 48000
	Channels    uint32 // 1 for mono, 2 for stereo
	BufferSize  uint32 // frames per callback
}

// DefaultConfig returns sensible defaults for CW decoding
func DefaultConfig() Config {
	return Config{
		DeviceIndex: -1,
		SampleRate:  48000,
		Channels:    1,
		BufferSize:  512,
	}
}

// SampleCallback is called directly from the audio thread with new samples.
// Use for low-latency processing. Must be non-blocking and fast.
type SampleCallback func(samples []float32)

// Capture handles real-time audio sampling from a USB audio device
type Capture struct {
	config   Config
	ctx      *malgo.AllocatedContext
	device   *malgo.Device
	running  bool
	mu       sync.RWMutex
	callback SampleCallback

	// Output channel for audio samples (float32 normalized -1.0 to 1.0)
	Samples chan []float32
}

// New creates a new audio capture instance
func New(cfg Config) *Capture {
	return &Capture{
		config:  cfg,
		Samples: make(chan []float32, 64),
	}
}

// SetCallback sets a callback for real-time sample processing.
// The callback is invoked directly from the audio thread - it must be
// non-blocking and fast. Set before calling Start().
func (c *Capture) SetCallback(cb SampleCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callback = cb
}

// Init initializes the audio backend
func (c *Capture) Init() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctxConfig := malgo.ContextConfig{}
	ctx, err := malgo.InitContext(nil, ctxConfig, nil)
	if err != nil {
		return fmt.Errorf("init audio context: %w", err)
	}
	c.ctx = ctx

	return nil
}

// ListDevices returns available capture devices
func (c *Capture) ListDevices() ([]malgo.DeviceInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.ctx == nil {
		return nil, ErrNotInitialized
	}

	infos, err := c.ctx.Devices(malgo.Capture)
	if err != nil {
		return nil, fmt.Errorf("enumerate devices: %w", err)
	}

	return infos, nil
}

// Start begins audio capture
func (c *Capture) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return ErrAlreadyRunning
	}
	if c.ctx == nil {
		c.mu.Unlock()
		return ErrNotInitialized
	}
	c.mu.Unlock()

	deviceConfig := malgo.DeviceConfig{
		DeviceType:         malgo.Capture,
		SampleRate:         c.config.SampleRate,
		PeriodSizeInFrames: c.config.BufferSize,
		Capture: malgo.SubConfig{
			Format:   malgo.FormatF32,
			Channels: c.config.Channels,
		},
	}

	// Select specific device if requested
	var deviceID *malgo.DeviceID
	if c.config.DeviceIndex >= 0 {
		devices, err := c.ListDevices()
		if err != nil {
			return err
		}
		if c.config.DeviceIndex >= len(devices) {
			return fmt.Errorf("device index %d out of range (have %d devices)",
				c.config.DeviceIndex, len(devices))
		}
		deviceID = &devices[c.config.DeviceIndex].ID
	}

	// Callback receives audio data
	onRecvFrames := func(outputSamples, inputSamples []byte, frameCount uint32) {
		if len(inputSamples) == 0 {
			return
		}

		// Convert bytes to float32 samples
		samples := bytesToFloat32(inputSamples)

		// Call real-time callback if registered (for low-latency processing)
		c.mu.RLock()
		cb := c.callback
		c.mu.RUnlock()
		if cb != nil {
			cb(samples)
		}

		// Non-blocking send to prevent callback blocking
		select {
		case c.Samples <- samples:
		default:
			// Drop samples if channel is full (consumer too slow)
		}
	}

	deviceCallbacks := malgo.DeviceCallbacks{
		Data: onRecvFrames,
	}

	device, err := malgo.InitDevice(c.ctx.Context, deviceConfig, deviceCallbacks)
	if err != nil {
		return fmt.Errorf("init device: %w", err)
	}

	// Set device ID if specified
	if deviceID != nil {
		deviceConfig.Capture.DeviceID = deviceID.Pointer()
		// Reinitialize with specific device
		device.Uninit()
		device, err = malgo.InitDevice(c.ctx.Context, deviceConfig, deviceCallbacks)
		if err != nil {
			return fmt.Errorf("init device with ID: %w", err)
		}
	}

	if err := device.Start(); err != nil {
		device.Uninit()
		return fmt.Errorf("start device: %w", err)
	}

	c.mu.Lock()
	c.device = device
	c.running = true
	c.mu.Unlock()

	// Wait for context cancellation
	go func() {
		<-ctx.Done()
		_ = c.Stop()
	}()

	return nil
}

// Stop stops audio capture
func (c *Capture) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return ErrNotRunning
	}

	if c.device != nil {
		_ = c.device.Stop()
		c.device.Uninit()
		c.device = nil
	}

	c.running = false
	return nil
}

// Close releases all audio resources
func (c *Capture) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running && c.device != nil {
		_ = c.device.Stop()
		c.device.Uninit()
		c.device = nil
		c.running = false
	}

	if c.ctx != nil {
		if err := c.ctx.Uninit(); err != nil {
			return fmt.Errorf("uninit context: %w", err)
		}
		c.ctx.Free()
		c.ctx = nil
	}

	close(c.Samples)
	return nil
}

// IsRunning returns true if capture is active
func (c *Capture) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// bytesToFloat32 converts raw bytes to float32 samples
func bytesToFloat32(data []byte) []float32 {
	numSamples := len(data) / 4
	samples := make([]float32, numSamples)

	for i := 0; i < numSamples; i++ {
		offset := i * 4
		// Little-endian float32
		bits := uint32(data[offset]) |
			uint32(data[offset+1])<<8 |
			uint32(data[offset+2])<<16 |
			uint32(data[offset+3])<<24
		samples[i] = float32frombits(bits)
	}

	return samples
}

// float32frombits converts IEEE 754 binary representation to float32
func float32frombits(b uint32) float32 {
	return *(*float32)(unsafe.Pointer(&b))
}
