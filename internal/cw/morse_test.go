package cw

import (
	"sync"
	"testing"
	"time"

	"github.com/ColonelBlimp/cwdecoder/internal/dsp"
)

// validConfig returns a valid DecoderConfig for testing
func validConfig() DecoderConfig {
	return DecoderConfig{
		InitialWPM:        15,
		AdaptiveTiming:    true,
		AdaptiveSmoothing: 0.1,
		DitDahBoundary:    2.0,
		InterCharBoundary: 2.0,
		CharWordBoundary:  5.0,
		FarnsworthWPM:     0,
	}
}

func TestNewDecoder_ValidConfig(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	if decoder == nil {
		t.Fatal("NewDecoder() returned nil decoder")
	}
}

func TestNewDecoder_InvalidWPM(t *testing.T) {
	cfg := validConfig()
	cfg.InitialWPM = 0

	_, err := NewDecoder(cfg)
	if err != ErrInvalidWPM {
		t.Errorf("NewDecoder() error = %v, want %v", err, ErrInvalidWPM)
	}

	cfg.InitialWPM = -5
	_, err = NewDecoder(cfg)
	if err != ErrInvalidWPM {
		t.Errorf("NewDecoder() error = %v, want %v", err, ErrInvalidWPM)
	}
}

func TestNewDecoder_InvalidFarnsworthWPM(t *testing.T) {
	cfg := validConfig()
	cfg.InitialWPM = 15
	cfg.FarnsworthWPM = 20 // Greater than InitialWPM

	_, err := NewDecoder(cfg)
	if err != ErrInvalidFarnsworthWPM {
		t.Errorf("NewDecoder() error = %v, want %v", err, ErrInvalidFarnsworthWPM)
	}

	cfg.FarnsworthWPM = -1
	_, err = NewDecoder(cfg)
	if err != ErrInvalidFarnsworthWPM {
		t.Errorf("NewDecoder() error = %v, want %v", err, ErrInvalidFarnsworthWPM)
	}
}

func TestNewDecoder_InvalidAdaptiveSmoothing(t *testing.T) {
	cfg := validConfig()

	cfg.AdaptiveSmoothing = -0.1
	_, err := NewDecoder(cfg)
	if err != ErrInvalidAdaptiveSmoothing {
		t.Errorf("NewDecoder() error = %v, want %v", err, ErrInvalidAdaptiveSmoothing)
	}

	cfg.AdaptiveSmoothing = 1.5
	_, err = NewDecoder(cfg)
	if err != ErrInvalidAdaptiveSmoothing {
		t.Errorf("NewDecoder() error = %v, want %v", err, ErrInvalidAdaptiveSmoothing)
	}
}

func TestNewDecoder_InvalidDitDahBoundary(t *testing.T) {
	cfg := validConfig()

	cfg.DitDahBoundary = 0
	_, err := NewDecoder(cfg)
	if err != ErrInvalidDitDahBoundary {
		t.Errorf("NewDecoder() error = %v, want %v", err, ErrInvalidDitDahBoundary)
	}

	cfg.DitDahBoundary = -1
	_, err = NewDecoder(cfg)
	if err != ErrInvalidDitDahBoundary {
		t.Errorf("NewDecoder() error = %v, want %v", err, ErrInvalidDitDahBoundary)
	}
}

func TestNewDecoder_InvalidCharWordBoundary(t *testing.T) {
	cfg := validConfig()

	cfg.CharWordBoundary = 0
	_, err := NewDecoder(cfg)
	if err != ErrInvalidCharWordBoundary {
		t.Errorf("NewDecoder() error = %v, want %v", err, ErrInvalidCharWordBoundary)
	}

	cfg.CharWordBoundary = -1
	_, err = NewDecoder(cfg)
	if err != ErrInvalidCharWordBoundary {
		t.Errorf("NewDecoder() error = %v, want %v", err, ErrInvalidCharWordBoundary)
	}
}

func TestDecoder_InitialDitDuration(t *testing.T) {
	cfg := validConfig()
	cfg.InitialWPM = 15

	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	// At 15 WPM: dit duration = 60000 / (15 * 50) = 80ms
	expectedDitMs := MillisecondsPerMinute / (float64(cfg.InitialWPM) * DitsPerWord)
	if decoder.ditDurationMs != expectedDitMs {
		t.Errorf("ditDurationMs = %v, want %v", decoder.ditDurationMs, expectedDitMs)
	}
}

func TestDecoder_CurrentWPM(t *testing.T) {
	cfg := validConfig()
	cfg.InitialWPM = 20

	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	wpm := decoder.CurrentWPM()
	if wpm != 20 {
		t.Errorf("CurrentWPM() = %v, want 20", wpm)
	}
}

func TestDecoder_Reset(t *testing.T) {
	cfg := validConfig()
	cfg.InitialWPM = 15

	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	// Modify internal state
	decoder.ditDurationMs = 50.0
	decoder.treeIndex = 10
	decoder.inChar = true

	decoder.Reset()

	// Verify reset
	if decoder.treeIndex != 1 {
		t.Errorf("after Reset(), treeIndex = %v, want 1", decoder.treeIndex)
	}
	if decoder.inChar != false {
		t.Error("after Reset(), inChar should be false")
	}
	// Dit duration should be recalculated from initial WPM
	expectedDitMs := MillisecondsPerMinute / (float64(cfg.InitialWPM) * DitsPerWord)
	if decoder.ditDurationMs != expectedDitMs {
		t.Errorf("after Reset(), ditDurationMs = %v, want %v", decoder.ditDurationMs, expectedDitMs)
	}
}

func TestDecoder_SetCallback(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	decoder.SetCallback(func(_ DecodedOutput) {
		// Callback set successfully
	})

	if decoder.callbackPtr == nil {
		t.Error("SetCallback() should set callbackPtr")
	}

	// Set to nil
	decoder.SetCallback(nil)
	if decoder.callbackPtr != nil {
		t.Error("SetCallback(nil) should set callbackPtr to nil")
	}
}

func TestDecoder_HandleToneEvent_Dit(t *testing.T) {
	cfg := validConfig()
	cfg.InitialWPM = 15
	cfg.AdaptiveTiming = false // Disable for predictable testing

	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	// Simulate a dit (short tone) followed by character space
	// At 15 WPM, dit = 80ms
	ditDuration := 80 * time.Millisecond

	// Tone ends (this is a dit)
	decoder.HandleToneEvent(dsp.ToneEvent{
		ToneOn:    false,
		Duration:  ditDuration,
		Timestamp: time.Now(),
		Magnitude: 0.8,
	})

	// Should be building a character now
	if !decoder.inChar {
		t.Error("after dit, inChar should be true")
	}
	// Tree index should be 2 (left from 1)
	if decoder.treeIndex != 2 {
		t.Errorf("after dit, treeIndex = %v, want 2", decoder.treeIndex)
	}
}

func TestDecoder_HandleToneEvent_Dah(t *testing.T) {
	cfg := validConfig()
	cfg.InitialWPM = 15
	cfg.AdaptiveTiming = false

	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	// Simulate a dah (long tone) - 3x dit = 240ms at 15 WPM
	dahDuration := 240 * time.Millisecond

	decoder.HandleToneEvent(dsp.ToneEvent{
		ToneOn:    false,
		Duration:  dahDuration,
		Timestamp: time.Now(),
		Magnitude: 0.8,
	})

	// Tree index should be 3 (right from 1)
	if decoder.treeIndex != 3 {
		t.Errorf("after dah, treeIndex = %v, want 3", decoder.treeIndex)
	}
}

func TestDecoder_HandleToneEvent_LetterE(t *testing.T) {
	cfg := validConfig()
	cfg.InitialWPM = 15
	cfg.AdaptiveTiming = false

	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	var received []DecodedOutput
	var mu sync.Mutex
	decoder.SetCallback(func(output DecodedOutput) {
		mu.Lock()
		received = append(received, output)
		mu.Unlock()
	})

	now := time.Now()
	ditDuration := 80 * time.Millisecond
	charSpace := 300 * time.Millisecond // > 3 * dit

	// Dit (tone off)
	decoder.HandleToneEvent(dsp.ToneEvent{
		ToneOn:    false,
		Duration:  ditDuration,
		Timestamp: now,
		Magnitude: 0.8,
	})

	// Character space (tone on after silence)
	decoder.HandleToneEvent(dsp.ToneEvent{
		ToneOn:    true,
		Duration:  charSpace,
		Timestamp: now.Add(charSpace),
		Magnitude: 0.8,
	})

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("expected to receive decoded character")
	}

	// First should be 'E' (dit = index 2 in tree)
	if received[0].Character != 'E' {
		t.Errorf("decoded character = %c, want E", received[0].Character)
	}
}

func TestDecoder_HandleToneEvent_LetterT(t *testing.T) {
	cfg := validConfig()
	cfg.InitialWPM = 15
	cfg.AdaptiveTiming = false

	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	var received []DecodedOutput
	var mu sync.Mutex
	decoder.SetCallback(func(output DecodedOutput) {
		mu.Lock()
		received = append(received, output)
		mu.Unlock()
	})

	now := time.Now()
	dahDuration := 240 * time.Millisecond
	charSpace := 300 * time.Millisecond

	// Dah (tone off)
	decoder.HandleToneEvent(dsp.ToneEvent{
		ToneOn:    false,
		Duration:  dahDuration,
		Timestamp: now,
		Magnitude: 0.8,
	})

	// Character space (tone on)
	decoder.HandleToneEvent(dsp.ToneEvent{
		ToneOn:    true,
		Duration:  charSpace,
		Timestamp: now.Add(charSpace),
		Magnitude: 0.8,
	})

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("expected to receive decoded character")
	}

	if received[0].Character != 'T' {
		t.Errorf("decoded character = %c, want T", received[0].Character)
	}
}

func TestDecoder_HandleToneEvent_WordSpace(t *testing.T) {
	cfg := validConfig()
	cfg.InitialWPM = 15
	cfg.AdaptiveTiming = false

	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	var received []DecodedOutput
	var mu sync.Mutex
	decoder.SetCallback(func(output DecodedOutput) {
		mu.Lock()
		received = append(received, output)
		mu.Unlock()
	})

	now := time.Now()
	ditDuration := 80 * time.Millisecond
	wordSpace := 600 * time.Millisecond // > 5 * dit (CharWordBoundary)

	// Dit
	decoder.HandleToneEvent(dsp.ToneEvent{
		ToneOn:    false,
		Duration:  ditDuration,
		Timestamp: now,
		Magnitude: 0.8,
	})

	// Word space (tone on after long silence)
	decoder.HandleToneEvent(dsp.ToneEvent{
		ToneOn:    true,
		Duration:  wordSpace,
		Timestamp: now.Add(wordSpace),
		Magnitude: 0.8,
	})

	mu.Lock()
	defer mu.Unlock()

	if len(received) < 2 {
		t.Fatalf("expected at least 2 outputs (char + word space), got %d", len(received))
	}

	// Should have character then word space
	if received[0].Character != 'E' {
		t.Errorf("first output character = %c, want E", received[0].Character)
	}
	if !received[1].IsWordSpace {
		t.Error("second output should be word space")
	}
	if received[1].Character != ' ' {
		t.Errorf("word space character = %c, want ' '", received[1].Character)
	}
}

func TestDecoder_AdaptiveTiming(t *testing.T) {
	cfg := validConfig()
	cfg.InitialWPM = 15
	cfg.AdaptiveTiming = true
	cfg.AdaptiveSmoothing = 0.5 // High for testing

	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	initialDit := decoder.ditDurationMs

	// Send a faster dit (shorter duration)
	fasterDit := 50 * time.Millisecond // Faster than 80ms
	decoder.HandleToneEvent(dsp.ToneEvent{
		ToneOn:    false,
		Duration:  fasterDit,
		Timestamp: time.Now(),
		Magnitude: 0.8,
	})

	// Dit duration should have decreased
	if decoder.ditDurationMs >= initialDit {
		t.Errorf("after faster dit, ditDurationMs = %v, should be less than %v", decoder.ditDurationMs, initialDit)
	}
}

func TestMorseTree_KnownCharacters(t *testing.T) {
	// Test that key characters are in the right positions
	tests := []struct {
		index int
		char  rune
	}{
		{2, 'E'},  // .
		{3, 'T'},  // -
		{4, 'I'},  // ..
		{5, 'A'},  // .-
		{6, 'N'},  // -.
		{7, 'M'},  // --
		{8, 'S'},  // ...
		{15, 'O'}, // ---
		{16, 'H'}, // ....
		{32, '5'}, // .....
		{47, '1'}, // .----
		{63, '0'}, // -----
	}

	for _, tt := range tests {
		if MorseTree[tt.index] != tt.char {
			t.Errorf("MorseTree[%d] = %c, want %c", tt.index, MorseTree[tt.index], tt.char)
		}
	}
}

func TestDecoder_TreeOverflow(t *testing.T) {
	cfg := validConfig()
	cfg.AdaptiveTiming = false

	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	dahDuration := 240 * time.Millisecond

	// Send too many dahs to overflow the tree (need > 6 elements to overflow 64)
	for i := 0; i < 10; i++ {
		decoder.HandleToneEvent(dsp.ToneEvent{
			ToneOn:    false,
			Duration:  dahDuration,
			Timestamp: time.Now(),
			Magnitude: 0.8,
		})
	}

	// Tree should have reset to prevent invalid access
	if decoder.treeIndex >= len(MorseTree) {
		t.Errorf("treeIndex = %v, should be < %v after overflow", decoder.treeIndex, len(MorseTree))
	}
}

func TestDecoder_FarnsworthTiming(t *testing.T) {
	cfg := validConfig()
	cfg.InitialWPM = 20
	cfg.FarnsworthWPM = 10 // Slower spacing
	cfg.AdaptiveTiming = false

	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	var received []DecodedOutput
	var mu sync.Mutex
	decoder.SetCallback(func(output DecodedOutput) {
		mu.Lock()
		received = append(received, output)
		mu.Unlock()
	})

	now := time.Now()
	// Dit at 20 WPM = 60ms
	ditDuration := 60 * time.Millisecond
	// With Farnsworth at 10 WPM, spacing dit = 120ms
	// InterCharBoundary = 2.0, so char space threshold = 120 * 2.0 = 240ms
	charSpace := 300 * time.Millisecond // Must be > 240ms

	// Dit
	decoder.HandleToneEvent(dsp.ToneEvent{
		ToneOn:    false,
		Duration:  ditDuration,
		Timestamp: now,
		Magnitude: 0.8,
	})

	// Character space (should trigger with Farnsworth spacing)
	decoder.HandleToneEvent(dsp.ToneEvent{
		ToneOn:    true,
		Duration:  charSpace,
		Timestamp: now.Add(charSpace),
		Magnitude: 0.8,
	})

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("expected to receive decoded character with Farnsworth timing")
	}
}

func TestConstants(t *testing.T) {
	// Verify ITU standard constants
	if DahDitRatio != 3.0 {
		t.Errorf("DahDitRatio = %v, want 3.0", DahDitRatio)
	}
	if IntraCharSpaceRatio != 1.0 {
		t.Errorf("IntraCharSpaceRatio = %v, want 1.0", IntraCharSpaceRatio)
	}
	if InterCharSpaceRatio != 3.0 {
		t.Errorf("InterCharSpaceRatio = %v, want 3.0", InterCharSpaceRatio)
	}
	if WordSpaceRatio != 7.0 {
		t.Errorf("WordSpaceRatio = %v, want 7.0", WordSpaceRatio)
	}
	if DitsPerWord != 50.0 {
		t.Errorf("DitsPerWord = %v, want 50.0", DitsPerWord)
	}
	if MillisecondsPerMinute != 60000.0 {
		t.Errorf("MillisecondsPerMinute = %v, want 60000.0", MillisecondsPerMinute)
	}
}

func TestDecoder_ConcurrentAccess(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	var wg sync.WaitGroup
	eventCount := 100

	// Multiple goroutines sending events
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < eventCount; j++ {
				decoder.HandleToneEvent(dsp.ToneEvent{
					ToneOn:    j%2 == 0,
					Duration:  80 * time.Millisecond,
					Timestamp: time.Now(),
					Magnitude: 0.8,
				})
			}
		}()
	}

	// Multiple goroutines reading WPM
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < eventCount; j++ {
				_ = decoder.CurrentWPM()
			}
		}()
	}

	wg.Wait()
	// If we get here without race detector errors, test passes
}
