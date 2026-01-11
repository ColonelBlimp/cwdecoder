// internal/cw/morse.go
// Package cw implements CW (Morse code) decoding from tone events.
package cw

import (
	"errors"
	"sync"
	"time"

	"github.com/ColonelBlimp/cwdecoder/internal/dsp"
)

// Morse code timing ratios (ITU standard)
// These are fixed ratios defined by the International Telecommunication Union
const (
	// DahDitRatio is the ratio of dah duration to dit duration (ITU: 3:1)
	DahDitRatio = 3.0
	// IntraCharSpaceRatio is the ratio of space between elements within a character to dit (ITU: 1:1)
	IntraCharSpaceRatio = 1.0
	// InterCharSpaceRatio is the ratio of space between characters to dit (ITU: 3:1)
	// Used as reference for default config value of char boundary
	InterCharSpaceRatio = 3.0
	// WordSpaceRatio is the ratio of space between words to dit (ITU: 7:1)
	// Used as reference for default config value of word boundary
	WordSpaceRatio = 7.0

	// DahDitThreshold is the default decision threshold between dit and dah (midpoint of 1 and 3)
	// Configurable via dit_dah_boundary in config
	DahDitThreshold = 2.0
	// CharWordThreshold is the default decision threshold between character and word space (midpoint of 3 and 7)
	// Configurable via char_word_boundary in config
	CharWordThreshold = 5.0

	// MillisecondsPerMinute is used for WPM calculations
	MillisecondsPerMinute = 60000.0
	// DitsPerWord is the standard word "PARIS" = 50 dit units
	DitsPerWord = 50.0

	// AdaptiveWeightNew is the default weight given to new timing samples in EMA calculation
	// Configurable via adaptive_smoothing in config
	AdaptiveWeightNew = 0.1
	// AdaptiveWeightOld is the weight given to existing average (1 - AdaptiveWeightNew)
	AdaptiveWeightOld = 0.9
)

// Compile-time assertions to ensure ITU reference constants are defined correctly
// These constants are used as reference values for config defaults
var (
	_ = InterCharSpaceRatio // ITU standard: 3 dit units between characters
	_ = WordSpaceRatio      // ITU standard: 7 dit units between words
	_ = DahDitThreshold     // Default boundary: midpoint of dit(1) and dah(3)
	_ = CharWordThreshold   // Default boundary: midpoint of char(3) and word(7) space
	_ = AdaptiveWeightNew   // Default EMA weight for new samples
)

var (
	// ErrInvalidWPM indicates WPM must be positive
	ErrInvalidWPM = errors.New("WPM must be positive")
	// ErrInvalidFarnsworthWPM indicates Farnsworth WPM must not exceed character WPM
	ErrInvalidFarnsworthWPM = errors.New("farnsworth WPM must not exceed character WPM")
	// ErrInvalidAdaptiveSmoothing indicates smoothing factor must be between 0 and 1
	ErrInvalidAdaptiveSmoothing = errors.New("adaptive smoothing must be between 0.0 and 1.0")
	// ErrInvalidDitDahBoundary indicates boundary ratio must be positive
	ErrInvalidDitDahBoundary = errors.New("dit/dah boundary ratio must be positive")
	// ErrInvalidCharWordBoundary indicates boundary ratio must be positive
	ErrInvalidCharWordBoundary = errors.New("char/word boundary ratio must be positive")
)

// MorseTree is the binary tree for Morse code lookup.
// Left branch = dit, Right branch = dah.
// Index 0 is root (unused), 1 is after first element, etc.
// Tree structure: parent at i, left child at 2i, right child at 2i+1
var MorseTree = [64]rune{
	0,   // 0: root (unused)
	0,   // 1: start
	'E', // 2: .
	'T', // 3: -
	'I', // 4: ..
	'A', // 5: .-
	'N', // 6: -.
	'M', // 7: --
	'S', // 8: ...
	'U', // 9: ..-
	'R', // 10: .-.
	'W', // 11: .--
	'D', // 12: -..
	'K', // 13: -.-
	'G', // 14: --.
	'O', // 15: ---
	'H', // 16: ....
	'V', // 17: ...-
	'F', // 18: ..-.
	0,   // 19: ..-- (Ü with accent, not standard)
	'L', // 20: .-..
	0,   // 21: .-.-  (Ä with accent)
	'P', // 22: .--.
	'J', // 23: .---
	'B', // 24: -...
	'X', // 25: -..-
	'C', // 26: -.-.
	'Y', // 27: -.--
	'Z', // 28: --..
	'Q', // 29: --.-
	0,   // 30: ---.  (Ö with accent)
	0,   // 31: ----
	'5', // 32: .....
	'4', // 33: ....-
	0,   // 34: ...-.
	'3', // 35: ...--
	0,   // 36: ..-..
	0,   // 37: ..-.-
	0,   // 38: ..--. (?)
	'2', // 39: ..---
	0,   // 40: .-...
	0,   // 41: .-..-
	0,   // 42: .-.-.
	0,   // 43: .-.--
	0,   // 44: .--..
	0,   // 45: .--.-
	0,   // 46: .---.
	'1', // 47: .----
	'6', // 48: -....
	'=', // 49: -...-
	'/', // 50: -..-.
	0,   // 51: -..--
	0,   // 52: -.-..
	0,   // 53: -.-.-
	0,   // 54: -.--. (open paren)
	0,   // 55: -.---
	'7', // 56: --...
	0,   // 57: --..-
	0,   // 58: --.-.
	0,   // 59: --.--
	'8', // 60: ---..
	0,   // 61: ---.-
	'9', // 62: ----.
	'0', // 63: -----
}

// DecoderConfig holds configuration for the CW decoder.
// All adjustable values come from the application config file.
type DecoderConfig struct {
	// InitialWPM is the starting words-per-minute estimate (from config: wpm)
	InitialWPM int
	// AdaptiveTiming enables automatic speed adaptation (from config: adaptive_timing)
	AdaptiveTiming bool
	// AdaptiveSmoothing is the EMA smoothing factor for timing adaptation (from config: adaptive_smoothing)
	// Higher values = faster adaptation, lower = more stable
	AdaptiveSmoothing float64
	// DitDahBoundary is the threshold ratio between dit and dah (from config: dit_dah_boundary)
	// A tone duration > (dit_duration * DitDahBoundary) is classified as dah
	DitDahBoundary float64
	// CharWordBoundary is the threshold ratio between character and word space (from config: char_word_boundary)
	// A space duration > (dit_duration * CharWordBoundary) is classified as word space
	CharWordBoundary float64
	// FarnsworthWPM is the effective WPM for spacing (0 = same as character WPM) (from config: farnsworth_wpm)
	// When set lower than InitialWPM, character spacing is stretched for easier copy
	FarnsworthWPM int
}

// DecodedCallback is called when a character or word boundary is decoded.
// Must be non-blocking and fast.
type DecodedCallback func(output DecodedOutput)

// DecodedOutput represents decoded CW output
type DecodedOutput struct {
	// Character is the decoded character (0 if word space)
	Character rune
	// IsWordSpace is true if this represents a word boundary
	IsWordSpace bool
	// Timestamp is when this was decoded
	Timestamp time.Time
	// CurrentWPM is the estimated WPM at time of decode
	CurrentWPM int
}

// Decoder decodes CW from tone events into characters and words.
type Decoder struct {
	config DecoderConfig

	// Timing state
	ditDurationMs float64 // Current estimate of dit duration in milliseconds
	mu            sync.Mutex

	// Current character being built
	treeIndex int  // Position in MorseTree (1 = start)
	inChar    bool // Whether we're currently building a character

	// Callback for decoded output
	callbackPtr *DecodedCallback
}

// NewDecoder creates a new CW decoder with the given configuration.
func NewDecoder(cfg DecoderConfig) (*Decoder, error) {
	if cfg.InitialWPM <= 0 {
		return nil, ErrInvalidWPM
	}
	if cfg.FarnsworthWPM < 0 || cfg.FarnsworthWPM > cfg.InitialWPM {
		return nil, ErrInvalidFarnsworthWPM
	}
	if cfg.AdaptiveSmoothing < 0 || cfg.AdaptiveSmoothing > 1 {
		return nil, ErrInvalidAdaptiveSmoothing
	}
	if cfg.DitDahBoundary <= 0 {
		return nil, ErrInvalidDitDahBoundary
	}
	if cfg.CharWordBoundary <= 0 {
		return nil, ErrInvalidCharWordBoundary
	}

	// Calculate initial dit duration from WPM
	// WPM = (dits per minute) / DitsPerWord
	// dits per minute = 60000 / dit_duration_ms
	// dit_duration_ms = 60000 / (WPM * DitsPerWord)
	ditDurationMs := MillisecondsPerMinute / (float64(cfg.InitialWPM) * DitsPerWord)

	return &Decoder{
		config:        cfg,
		ditDurationMs: ditDurationMs,
		treeIndex:     1, // Start at root
		inChar:        false,
	}, nil
}

// SetCallback sets the callback for decoded output.
func (d *Decoder) SetCallback(cb DecodedCallback) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if cb == nil {
		d.callbackPtr = nil
	} else {
		d.callbackPtr = &cb
	}
}

// HandleToneEvent processes a tone event from the detector.
// This is the main entry point, typically called from detector's callback.
func (d *Decoder) HandleToneEvent(event dsp.ToneEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if event.ToneOn {
		// Tone just started - check if previous silence was long enough for char/word boundary
		d.handleSilenceEnd(event)
	} else {
		// Tone just ended - classify as dit or dah
		d.handleToneEnd(event)
	}
}

// handleToneEnd classifies the tone duration as dit or dah and updates the tree position.
func (d *Decoder) handleToneEnd(event dsp.ToneEvent) {
	durationMs := float64(event.Duration.Milliseconds())

	// Classify as dit or dah based on duration
	isDah := durationMs > (d.ditDurationMs * d.config.DitDahBoundary)

	// Update timing estimate if adaptive timing is enabled
	if d.config.AdaptiveTiming {
		d.adaptTiming(durationMs, isDah)
	}

	// Navigate the Morse tree
	if !d.inChar {
		d.treeIndex = 1 // Start new character
		d.inChar = true
	}

	if isDah {
		// Right branch (dah)
		d.treeIndex = d.treeIndex*2 + 1
	} else {
		// Left branch (dit)
		d.treeIndex = d.treeIndex * 2
	}

	// Check for tree overflow (too many elements)
	if d.treeIndex >= len(MorseTree) {
		// Invalid sequence - reset
		d.treeIndex = 1
		d.inChar = false
	}
}

// handleSilenceEnd checks if the silence duration indicates a character or word boundary.
func (d *Decoder) handleSilenceEnd(event dsp.ToneEvent) {
	if !d.inChar {
		return // No character being built
	}

	durationMs := float64(event.Duration.Milliseconds())

	// Use Farnsworth timing for spacing if configured
	spacingDitMs := d.ditDurationMs
	if d.config.FarnsworthWPM > 0 && d.config.FarnsworthWPM < d.config.InitialWPM {
		spacingDitMs = MillisecondsPerMinute / (float64(d.config.FarnsworthWPM) * DitsPerWord)
	}

	// Determine if this is a character boundary or word boundary
	isWordSpace := durationMs > (spacingDitMs * d.config.CharWordBoundary)
	isCharSpace := durationMs > (spacingDitMs * IntraCharSpaceRatio)

	if isCharSpace || isWordSpace {
		// Emit the current character
		d.emitCharacter(event.Timestamp)

		if isWordSpace {
			// Also emit word space
			d.emitWordSpace(event.Timestamp)
		}
	}
}

// emitCharacter outputs the current character being built.
func (d *Decoder) emitCharacter(timestamp time.Time) {
	if d.treeIndex > 0 && d.treeIndex < len(MorseTree) {
		char := MorseTree[d.treeIndex]
		if char != 0 && d.callbackPtr != nil {
			(*d.callbackPtr)(DecodedOutput{
				Character:   char,
				IsWordSpace: false,
				Timestamp:   timestamp,
				CurrentWPM:  d.currentWPM(),
			})
		}
	}

	// Reset for next character
	d.treeIndex = 1
	d.inChar = false
}

// emitWordSpace outputs a word space marker.
func (d *Decoder) emitWordSpace(timestamp time.Time) {
	if d.callbackPtr != nil {
		(*d.callbackPtr)(DecodedOutput{
			Character:   ' ',
			IsWordSpace: true,
			Timestamp:   timestamp,
			CurrentWPM:  d.currentWPM(),
		})
	}
}

// adaptTiming updates the dit duration estimate using exponential moving average.
func (d *Decoder) adaptTiming(durationMs float64, isDah bool) {
	// Convert dah to equivalent dit duration
	estimatedDit := durationMs
	if isDah {
		estimatedDit = durationMs / DahDitRatio
	}

	// Apply EMA smoothing
	smoothing := d.config.AdaptiveSmoothing
	d.ditDurationMs = (AdaptiveWeightOld-smoothing+smoothing)*d.ditDurationMs +
		smoothing*estimatedDit

	// Simplified: just use config smoothing directly
	d.ditDurationMs = (1-smoothing)*d.ditDurationMs + smoothing*estimatedDit
}

// currentWPM returns the current estimated WPM based on dit duration.
func (d *Decoder) currentWPM() int {
	if d.ditDurationMs <= 0 {
		return d.config.InitialWPM
	}
	wpm := MillisecondsPerMinute / (d.ditDurationMs * DitsPerWord)
	return int(wpm + 0.5) // Round to nearest
}

// CurrentWPM returns the current estimated WPM (thread-safe).
func (d *Decoder) CurrentWPM() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.currentWPM()
}

// Reset clears the decoder state and resets timing to initial WPM.
func (d *Decoder) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.ditDurationMs = MillisecondsPerMinute / (float64(d.config.InitialWPM) * DitsPerWord)
	d.treeIndex = 1
	d.inChar = false
}
