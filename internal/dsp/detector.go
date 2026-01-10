// internal/dsp/detector.go
package dsp

import (
	"errors"
	"sync/atomic"
	"time"
)

var (
	// ErrInvalidThreshold indicates threshold must be between 0 and 1
	ErrInvalidThreshold = errors.New("threshold must be between 0.0 and 1.0")
	// ErrInvalidHysteresis indicates hysteresis must be non-negative
	ErrInvalidHysteresis = errors.New("hysteresis must be non-negative")
	// ErrInvalidOverlap indicates overlap percentage must be 0-99
	ErrInvalidOverlap = errors.New("overlap percentage must be between 0 and 99")
	// ErrInvalidAGCDecay indicates AGC decay must be between 0 and 1
	ErrInvalidAGCDecay = errors.New("agc decay must be between 0.0 and 1.0")
	// ErrInvalidAGCAttack indicates AGC attack must be between 0 and 1
	ErrInvalidAGCAttack = errors.New("agc attack must be between 0.0 and 1.0")
	// ErrInvalidAGCWarmup indicates AGC warmup blocks must be non-negative
	ErrInvalidAGCWarmup = errors.New("agc warmup blocks must be non-negative")
	// ErrGoertzelRequired indicates Goertzel instance is required
	ErrGoertzelRequired = errors.New("goertzel instance is required")
)

// ToneEvent represents a tone state change event.
// Used for downstream CW decoding - the Duration field is only valid
// when ToneOn is false (indicates how long the tone was on).
type ToneEvent struct {
	// ToneOn is true when tone starts, false when tone ends
	ToneOn bool
	// Timestamp is when the event occurred
	Timestamp time.Time
	// Duration is the length of the preceding state (only valid when ToneOn changes)
	Duration time.Duration
	// Magnitude is the detected tone magnitude (0.0-1.0 after AGC)
	Magnitude float64
}

// ToneCallback is called when tone state changes.
// Must be non-blocking and fast - called from the audio processing path.
type ToneCallback func(event ToneEvent)

// DetectorConfig holds configuration for the tone detector.
// All values should come from the application config file.
type DetectorConfig struct {
	// Threshold for tone detection (0.0-1.0) (from config: threshold)
	Threshold float64
	// Hysteresis is consecutive blocks required to confirm state change (from config: hysteresis)
	Hysteresis int
	// OverlapPct is the block overlap percentage 0-99 (from config: overlap_pct)
	OverlapPct int
	// AGCEnabled enables automatic gain control (from config: agc_enabled)
	AGCEnabled bool
	// AGCDecay is the peak decay rate per sample (from config: agc_decay)
	AGCDecay float64
	// AGCAttack is how fast to respond to louder signals (from config: agc_attack)
	AGCAttack float64
	// AGCWarmupBlocks is the number of blocks to process before enabling detection (from config: agc_warmup_blocks)
	// Allows AGC to calibrate to signal level, preventing false triggers on startup
	AGCWarmupBlocks int
}

// Detector detects CW tones in audio samples using the Goertzel algorithm.
// It applies AGC and hysteresis to produce clean tone on/off events.
type Detector struct {
	config    DetectorConfig
	goertzel  *Goertzel
	blockSize int

	// Overlap buffer for continuous processing
	overlapBuffer []float32
	overlapSize   int
	hopSize       int // samples to advance between blocks

	// AGC state
	agcPeak       float64
	warmupCounter int // blocks processed, detection disabled until >= AGCWarmupBlocks

	// Hysteresis state
	toneState       bool // current confirmed tone state
	pendingState    bool // state we're transitioning to
	hysteresisCount int  // consecutive blocks in pending state

	// Timing for duration calculation
	lastTransition time.Time

	// Callback for tone events (atomic for thread safety)
	callbackPtr atomic.Pointer[ToneCallback]
}

// NewDetector creates a new tone detector with the given configuration.
func NewDetector(cfg DetectorConfig, goertzel *Goertzel) (*Detector, error) {
	if goertzel == nil {
		return nil, ErrGoertzelRequired
	}
	if cfg.Threshold < 0 || cfg.Threshold > 1 {
		return nil, ErrInvalidThreshold
	}
	if cfg.Hysteresis < 0 {
		return nil, ErrInvalidHysteresis
	}
	if cfg.OverlapPct < 0 || cfg.OverlapPct >= 100 {
		return nil, ErrInvalidOverlap
	}
	if cfg.AGCDecay < 0 || cfg.AGCDecay > 1 {
		return nil, ErrInvalidAGCDecay
	}
	if cfg.AGCAttack < 0 || cfg.AGCAttack > 1 {
		return nil, ErrInvalidAGCAttack
	}
	if cfg.AGCWarmupBlocks < 0 {
		return nil, ErrInvalidAGCWarmup
	}

	blockSize := goertzel.BlockSize()
	overlapSize := (blockSize * cfg.OverlapPct) / 100
	hopSize := blockSize - overlapSize

	return &Detector{
		config:        cfg,
		goertzel:      goertzel,
		blockSize:     blockSize,
		overlapBuffer: make([]float32, 0, blockSize),
		overlapSize:   overlapSize,
		hopSize:       hopSize,
		agcPeak:       1.0, // Initialize to 1.0 to prevent false triggers during warmup
		warmupCounter: 0,
		toneState:     false,
		pendingState:  false,
	}, nil
}

// SetCallback sets the callback for tone events.
// The callback is invoked from the processing goroutine - it must be fast and non-blocking.
func (d *Detector) SetCallback(cb ToneCallback) {
	if cb == nil {
		d.callbackPtr.Store(nil)
	} else {
		d.callbackPtr.Store(&cb)
	}
}

// Process processes incoming audio samples and detects tones.
// Samples should be float32 normalized to -1.0 to 1.0.
// This method handles buffering for overlap processing.
func (d *Detector) Process(samples []float32) {
	// Append new samples to overlap buffer
	d.overlapBuffer = append(d.overlapBuffer, samples...)

	// Process complete blocks
	for len(d.overlapBuffer) >= d.blockSize {
		d.processBlock(d.overlapBuffer[:d.blockSize])

		// Slide the buffer by hopSize
		if d.hopSize > 0 && d.hopSize < len(d.overlapBuffer) {
			copy(d.overlapBuffer, d.overlapBuffer[d.hopSize:])
			d.overlapBuffer = d.overlapBuffer[:len(d.overlapBuffer)-d.hopSize]
		} else {
			d.overlapBuffer = d.overlapBuffer[:0]
		}
	}
}

// processBlock processes a single block of samples
func (d *Detector) processBlock(block []float32) {
	// Compute raw magnitude using Goertzel
	magnitude := d.goertzel.MagnitudeNoAlloc(block)

	// During warmup, calibrate AGC to actual signal level without triggering detection
	if d.warmupCounter < d.config.AGCWarmupBlocks {
		d.warmupCounter++
		if d.config.AGCEnabled && magnitude > 0.001 {
			// During warmup, directly track the maximum signal level for calibration
			// This ensures AGC is properly calibrated before detection starts
			if magnitude > d.agcPeak {
				d.agcPeak = magnitude
			} else if d.warmupCounter == 1 {
				// On first block, initialize to actual signal level
				d.agcPeak = magnitude
			}
			// After first block, keep peak at max seen during warmup
			// (no decay during warmup to ensure stable calibration)
		}
		return
	}

	// Apply AGC if enabled (normal operation after warmup)
	if d.config.AGCEnabled {
		magnitude = d.applyAGC(magnitude)
	}

	// Determine if tone is present based on threshold
	tonePresent := magnitude > d.config.Threshold

	// Apply hysteresis
	d.updateHysteresis(tonePresent, magnitude)
}

// applyAGC applies automatic gain control to normalize the magnitude
func (d *Detector) applyAGC(magnitude float64) float64 {
	// Update peak tracker
	if magnitude > d.agcPeak {
		// Attack: fast response to louder signals
		d.agcPeak = d.agcPeak + d.config.AGCAttack*(magnitude-d.agcPeak)
	} else {
		// Decay: gradual decrease when signal is quieter
		d.agcPeak = d.agcPeak * d.config.AGCDecay
	}

	// Ensure minimum peak to avoid division issues
	if d.agcPeak < 0.001 {
		d.agcPeak = 0.001
	}

	// Normalize magnitude by peak
	normalized := magnitude / d.agcPeak

	// Clamp to 0-1 range
	if normalized > 1.0 {
		normalized = 1.0
	}

	return normalized
}

// updateHysteresis applies hysteresis to debounce tone detection
func (d *Detector) updateHysteresis(tonePresent bool, magnitude float64) {
	now := time.Now()

	if tonePresent == d.toneState {
		// State matches, reset hysteresis counter
		d.pendingState = d.toneState
		d.hysteresisCount = 0
		return
	}

	// State differs from current confirmed state
	if tonePresent == d.pendingState {
		// Continuing in the pending state direction
		d.hysteresisCount++
	} else {
		// Changed direction, start new pending state
		d.pendingState = tonePresent
		d.hysteresisCount = 1
	}

	// Check if we've reached the hysteresis threshold
	if d.hysteresisCount >= d.config.Hysteresis {
		// Confirm the state change
		duration := time.Duration(0)
		if !d.lastTransition.IsZero() {
			duration = now.Sub(d.lastTransition)
		}

		d.toneState = d.pendingState
		d.lastTransition = now
		d.hysteresisCount = 0

		// Emit event via callback
		d.emitEvent(ToneEvent{
			ToneOn:    d.toneState,
			Timestamp: now,
			Duration:  duration,
			Magnitude: magnitude,
		})
	}
}

// emitEvent calls the registered callback if set
func (d *Detector) emitEvent(event ToneEvent) {
	cbPtr := d.callbackPtr.Load()
	if cbPtr != nil {
		(*cbPtr)(event)
	}
}

// ToneState returns the current confirmed tone state
func (d *Detector) ToneState() bool {
	return d.toneState
}

// AGCPeak returns the current AGC peak value (for debugging/monitoring)
func (d *Detector) AGCPeak() float64 {
	return d.agcPeak
}

// Reset resets the detector state
func (d *Detector) Reset() {
	d.overlapBuffer = d.overlapBuffer[:0]
	d.agcPeak = 0.001
	d.toneState = false
	d.pendingState = false
	d.hysteresisCount = 0
	d.lastTransition = time.Time{}
}

// Config returns the current configuration
func (d *Detector) Config() DetectorConfig {
	return d.config
}
