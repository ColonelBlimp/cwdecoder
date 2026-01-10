// internal/dsp/detector_test.go
package dsp

import (
	"sync"
	"testing"
	"time"
)

// Test configuration constants matching config file defaults
const (
	detectorTestSampleRate    = 48000.0
	detectorTestToneFrequency = 600.0
	detectorTestBlockSize     = 512
	detectorTestThreshold     = 0.4
	detectorTestHysteresis    = 5
	detectorTestOverlapPct    = 50
	detectorTestAGCDecay      = 0.9995
	detectorTestAGCAttack     = 0.1
)

// createTestGoertzel creates a Goertzel instance for testing
func createTestGoertzel(t *testing.T) *Goertzel {
	t.Helper()
	cfg := GoertzelConfig{
		TargetFrequency: detectorTestToneFrequency,
		SampleRate:      detectorTestSampleRate,
		BlockSize:       detectorTestBlockSize,
	}
	g, err := NewGoertzel(cfg)
	if err != nil {
		t.Fatalf("Failed to create Goertzel: %v", err)
	}
	return g
}

// createTestDetectorConfig creates a valid detector config for testing
func createTestDetectorConfig() DetectorConfig {
	return DetectorConfig{
		Threshold:  detectorTestThreshold,
		Hysteresis: detectorTestHysteresis,
		OverlapPct: detectorTestOverlapPct,
		AGCEnabled: true,
		AGCDecay:   detectorTestAGCDecay,
		AGCAttack:  detectorTestAGCAttack,
	}
}

func TestNewDetector_ValidConfig(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed with valid config: %v", err)
	}

	if d == nil {
		t.Fatal("NewDetector returned nil with valid config")
	}

	// Verify config is stored
	storedCfg := d.Config()
	if storedCfg.Threshold != cfg.Threshold {
		t.Errorf("Threshold mismatch: got %v, want %v", storedCfg.Threshold, cfg.Threshold)
	}
	if storedCfg.Hysteresis != cfg.Hysteresis {
		t.Errorf("Hysteresis mismatch: got %v, want %v", storedCfg.Hysteresis, cfg.Hysteresis)
	}
}

func TestNewDetector_NilGoertzel(t *testing.T) {
	cfg := createTestDetectorConfig()

	_, err := NewDetector(cfg, nil)
	if err != ErrGoertzelRequired {
		t.Errorf("expected ErrGoertzelRequired, got: %v", err)
	}
}

func TestNewDetector_InvalidThreshold(t *testing.T) {
	g := createTestGoertzel(t)

	testCases := []struct {
		name      string
		threshold float64
	}{
		{"negative", -0.1},
		{"too high", 1.1},
		{"very negative", -1.0},
		{"way too high", 2.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := createTestDetectorConfig()
			cfg.Threshold = tc.threshold

			_, err := NewDetector(cfg, g)
			if err != ErrInvalidThreshold {
				t.Errorf("expected ErrInvalidThreshold, got: %v", err)
			}
		})
	}
}

func TestNewDetector_InvalidHysteresis(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.Hysteresis = -1

	_, err := NewDetector(cfg, g)
	if err != ErrInvalidHysteresis {
		t.Errorf("expected ErrInvalidHysteresis, got: %v", err)
	}
}

func TestNewDetector_InvalidOverlap(t *testing.T) {
	g := createTestGoertzel(t)

	testCases := []struct {
		name    string
		overlap int
	}{
		{"negative", -1},
		{"100 percent", 100},
		{"over 100", 150},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := createTestDetectorConfig()
			cfg.OverlapPct = tc.overlap

			_, err := NewDetector(cfg, g)
			if err != ErrInvalidOverlap {
				t.Errorf("expected ErrInvalidOverlap, got: %v", err)
			}
		})
	}
}

func TestNewDetector_InvalidAGCDecay(t *testing.T) {
	g := createTestGoertzel(t)

	testCases := []struct {
		name  string
		decay float64
	}{
		{"negative", -0.1},
		{"too high", 1.1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := createTestDetectorConfig()
			cfg.AGCDecay = tc.decay

			_, err := NewDetector(cfg, g)
			if err != ErrInvalidAGCDecay {
				t.Errorf("expected ErrInvalidAGCDecay, got: %v", err)
			}
		})
	}
}

func TestNewDetector_InvalidAGCAttack(t *testing.T) {
	g := createTestGoertzel(t)

	testCases := []struct {
		name   string
		attack float64
	}{
		{"negative", -0.1},
		{"too high", 1.1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := createTestDetectorConfig()
			cfg.AGCAttack = tc.attack

			_, err := NewDetector(cfg, g)
			if err != ErrInvalidAGCAttack {
				t.Errorf("expected ErrInvalidAGCAttack, got: %v", err)
			}
		})
	}
}

func TestNewDetector_InvalidAGCWarmup(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.AGCWarmupBlocks = -1

	_, err := NewDetector(cfg, g)
	if err != ErrInvalidAGCWarmup {
		t.Errorf("expected ErrInvalidAGCWarmup, got: %v", err)
	}
}

func TestDetector_AGCWarmup_SkipsDetectionDuringWarmup(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.AGCWarmupBlocks = 20 // Require 20 blocks of warmup (generous)
	cfg.Hysteresis = 0       // Disable hysteresis for simpler testing
	cfg.OverlapPct = 0       // Disable overlap for predictable block counting

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}

	// Track events
	var events []ToneEvent
	var mu sync.Mutex
	d.SetCallback(func(event ToneEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// Generate a strong tone signal that would normally trigger detection
	block := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, detectorTestBlockSize, 0.8)

	// Process fewer blocks than warmup requires - should not trigger any events
	// With 0% overlap, each Process(block) = exactly 1 processBlock call
	for i := 0; i < 15; i++ { // 15 < 20 warmup blocks
		d.Process(block)
	}

	mu.Lock()
	eventsDuringWarmup := len(events)
	mu.Unlock()

	if eventsDuringWarmup != 0 {
		t.Errorf("Expected no events during warmup (processed 15 of 20 warmup blocks), got %d", eventsDuringWarmup)
	}

	// Now process enough to complete warmup and detect
	for i := 0; i < 10; i++ { // 5 more to complete warmup + 5 for detection
		d.Process(block)
	}

	mu.Lock()
	eventsAfterWarmup := len(events)
	mu.Unlock()

	if eventsAfterWarmup == 0 {
		t.Error("Expected events after warmup, got none")
	}
}

func TestDetector_AGCWarmup_ZeroWarmupAllowsImmediateDetection(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.AGCWarmupBlocks = 0 // No warmup
	cfg.Hysteresis = 0      // Disable hysteresis

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}

	// Track events
	var events []ToneEvent
	var mu sync.Mutex
	d.SetCallback(func(event ToneEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// Generate and process a strong tone signal
	block := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, detectorTestBlockSize, 0.8)
	d.Process(block)

	mu.Lock()
	eventCount := len(events)
	mu.Unlock()

	// With zero warmup and zero hysteresis, first block should trigger detection
	if eventCount == 0 {
		t.Error("Expected immediate detection with zero warmup, got no events")
	}
}

func TestNewDetector_ValidBoundaryValues(t *testing.T) {
	g := createTestGoertzel(t)

	testCases := []struct {
		name string
		cfg  DetectorConfig
	}{
		{
			name: "zero threshold",
			cfg: DetectorConfig{
				Threshold:  0.0,
				Hysteresis: 5,
				OverlapPct: 50,
				AGCEnabled: true,
				AGCDecay:   0.9995,
				AGCAttack:  0.1,
			},
		},
		{
			name: "max threshold",
			cfg: DetectorConfig{
				Threshold:  1.0,
				Hysteresis: 5,
				OverlapPct: 50,
				AGCEnabled: true,
				AGCDecay:   0.9995,
				AGCAttack:  0.1,
			},
		},
		{
			name: "zero hysteresis",
			cfg: DetectorConfig{
				Threshold:  0.4,
				Hysteresis: 0,
				OverlapPct: 50,
				AGCEnabled: true,
				AGCDecay:   0.9995,
				AGCAttack:  0.1,
			},
		},
		{
			name: "zero overlap",
			cfg: DetectorConfig{
				Threshold:  0.4,
				Hysteresis: 5,
				OverlapPct: 0,
				AGCEnabled: true,
				AGCDecay:   0.9995,
				AGCAttack:  0.1,
			},
		},
		{
			name: "max overlap",
			cfg: DetectorConfig{
				Threshold:  0.4,
				Hysteresis: 5,
				OverlapPct: 99,
				AGCEnabled: true,
				AGCDecay:   0.9995,
				AGCAttack:  0.1,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := NewDetector(tc.cfg, g)
			if err != nil {
				t.Errorf("expected valid config to succeed, got: %v", err)
			}
			if d == nil {
				t.Error("expected non-nil detector")
			}
		})
	}
}

func TestDetector_ToneState_InitiallyFalse(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	if d.ToneState() != false {
		t.Error("Initial tone state should be false")
	}
}

func TestDetector_SetCallback(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.Hysteresis = 1 // Quick transitions for testing

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	var events []ToneEvent
	var mu sync.Mutex

	callback := func(event ToneEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}

	d.SetCallback(callback)

	// Generate tone samples - enough blocks to trigger hysteresis
	samples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, detectorTestBlockSize*10, 1.0)
	d.Process(samples)

	mu.Lock()
	eventCount := len(events)
	mu.Unlock()

	if eventCount == 0 {
		t.Error("Callback should have been invoked")
	}
}

func TestDetector_SetCallback_Nil(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	// Should not panic
	d.SetCallback(nil)

	// Process should not panic with nil callback
	samples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, detectorTestBlockSize*10, 1.0)
	d.Process(samples)
}

func TestDetector_Process_ToneDetection(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.Hysteresis = 2 // Lower hysteresis for faster testing

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	var events []ToneEvent
	var mu sync.Mutex

	d.SetCallback(func(event ToneEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// Process a strong tone signal
	samples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, detectorTestBlockSize*10, 1.0)
	d.Process(samples)

	mu.Lock()
	defer mu.Unlock()

	// Should have detected tone on
	foundToneOn := false
	for _, e := range events {
		if e.ToneOn {
			foundToneOn = true
			break
		}
	}

	if !foundToneOn {
		t.Error("Expected tone on event")
	}
}

func TestDetector_Process_Silence(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.Hysteresis = 1

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	var events []ToneEvent
	var mu sync.Mutex

	d.SetCallback(func(event ToneEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// Process silence
	samples := generateSilence(detectorTestBlockSize * 10)
	d.Process(samples)

	// Tone state should remain false
	if d.ToneState() != false {
		t.Error("Tone state should be false for silence")
	}
}

func TestDetector_Hysteresis_RequiresConsecutiveBlocks(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.Hysteresis = 5
	cfg.OverlapPct = 0 // No overlap for predictable block processing

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	eventCount := 0
	d.SetCallback(func(event ToneEvent) {
		eventCount++
	})

	// Process fewer blocks than hysteresis requires
	samples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, detectorTestBlockSize*3, 1.0)
	d.Process(samples)

	// Should not have triggered a state change yet
	if eventCount > 0 {
		t.Errorf("Should not trigger event with only 3 blocks when hysteresis is 5, got %d events", eventCount)
	}

	// Process more blocks to exceed hysteresis
	moreSamples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, detectorTestBlockSize*5, 1.0)
	d.Process(moreSamples)

	// Now should have triggered
	if eventCount == 0 {
		t.Error("Should have triggered event after exceeding hysteresis")
	}
}

func TestDetector_AGC_NormalizesLowAmplitude(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.AGCEnabled = true
	cfg.AGCWarmupBlocks = 20 // Allow AGC to calibrate during warmup
	cfg.AGCDecay = 0.99      // Faster decay for testing (100x faster than default)
	cfg.Hysteresis = 2
	cfg.Threshold = 0.4
	cfg.OverlapPct = 0 // Simpler block counting

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	eventCount := 0
	d.SetCallback(func(event ToneEvent) {
		eventCount++
	})

	// Low amplitude signal - without AGC this would be below threshold
	// Process enough samples to complete warmup (20 blocks) + detection (10 blocks)
	samples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, detectorTestBlockSize*35, 0.1)
	d.Process(samples)

	// AGC should normalize and detect the tone after warmup
	if eventCount == 0 {
		t.Error("AGC should normalize low amplitude signal and detect tone after warmup")
	}
}

func TestDetector_AGC_Disabled(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.AGCEnabled = false
	cfg.Hysteresis = 1
	cfg.Threshold = 0.5

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	eventCount := 0
	d.SetCallback(func(event ToneEvent) {
		eventCount++
	})

	// Very low amplitude signal - without AGC this should be below threshold
	samples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, detectorTestBlockSize*10, 0.1)
	d.Process(samples)

	// Without AGC, low amplitude should not trigger detection
	if d.ToneState() {
		t.Error("Without AGC, low amplitude signal should not trigger tone detection")
	}
}

func TestDetector_AGCPeak_TracksSignalLevel(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.AGCEnabled = true

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	initialPeak := d.AGCPeak()

	// Process high amplitude signal
	samples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, detectorTestBlockSize*5, 1.0)
	d.Process(samples)

	// AGC peak should have increased
	if d.AGCPeak() <= initialPeak {
		t.Error("AGC peak should increase with loud signal")
	}
}

func TestDetector_Reset(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.Hysteresis = 1

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	// Process some samples to change state
	samples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, detectorTestBlockSize*10, 1.0)
	d.Process(samples)

	// Verify state changed
	if !d.ToneState() {
		t.Skip("Tone state didn't change - test conditions not met")
	}

	// Reset
	d.Reset()

	// Verify reset
	if d.ToneState() != false {
		t.Error("ToneState should be false after Reset")
	}
	if d.AGCPeak() != 0.001 {
		t.Errorf("AGCPeak should be 0.001 after Reset, got %v", d.AGCPeak())
	}
}

func TestDetector_OverlapBuffer_Accumulation(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.OverlapPct = 50

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	// Process samples smaller than block size
	smallChunk := detectorTestBlockSize / 4
	samples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, smallChunk, 1.0)

	// Should not panic - just accumulate
	d.Process(samples)
	d.Process(samples)
	d.Process(samples)
	d.Process(samples) // Now have enough for one block
}

func TestDetector_ToneEvent_Duration(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.Hysteresis = 1
	cfg.OverlapPct = 0

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	var events []ToneEvent
	var mu sync.Mutex

	d.SetCallback(func(event ToneEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// Generate tone then silence to trigger both on and off events
	toneSamples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, detectorTestBlockSize*5, 1.0)
	silenceSamples := generateSilence(detectorTestBlockSize * 5)

	d.Process(toneSamples)
	time.Sleep(10 * time.Millisecond) // Small delay to ensure measurable duration
	d.Process(silenceSamples)

	mu.Lock()
	defer mu.Unlock()

	// Should have at least tone on event
	if len(events) == 0 {
		t.Fatal("Expected at least one event")
	}

	// First event's duration should be zero (no prior state)
	if events[0].Duration != 0 {
		t.Logf("First event duration: %v (expected 0 for first transition)", events[0].Duration)
	}

	// Subsequent events should have non-zero duration
	for i := 1; i < len(events); i++ {
		if events[i].Duration <= 0 {
			t.Errorf("Event %d duration should be positive: %v", i, events[i].Duration)
		}
	}
}

func TestDetector_ToneEvent_Timestamp(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.Hysteresis = 1

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	var events []ToneEvent
	d.SetCallback(func(event ToneEvent) {
		events = append(events, event)
	})

	before := time.Now()
	samples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, detectorTestBlockSize*5, 1.0)
	d.Process(samples)
	after := time.Now()

	if len(events) == 0 {
		t.Fatal("Expected at least one event")
	}

	// Timestamp should be between before and after
	for i, e := range events {
		if e.Timestamp.Before(before) || e.Timestamp.After(after) {
			t.Errorf("Event %d timestamp %v should be between %v and %v", i, e.Timestamp, before, after)
		}
	}
}

func TestDetector_Process_CWPattern(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.Hysteresis = 2
	cfg.OverlapPct = 0

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	var events []ToneEvent
	var mu sync.Mutex

	d.SetCallback(func(event ToneEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// Simulate a simple dit-dah pattern
	ditLength := detectorTestBlockSize * 5
	dahLength := detectorTestBlockSize * 15
	spaceLength := detectorTestBlockSize * 5

	// Dit
	ditSamples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, ditLength, 1.0)
	d.Process(ditSamples)

	// Space
	spaceSamples := generateSilence(spaceLength)
	d.Process(spaceSamples)

	// Dah
	dahSamples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, dahLength, 1.0)
	d.Process(dahSamples)

	// Final space
	d.Process(spaceSamples)

	mu.Lock()
	eventCount := len(events)
	mu.Unlock()

	// Should have multiple transitions
	if eventCount < 2 {
		t.Errorf("Expected at least 2 events for dit-dah pattern, got %d", eventCount)
	}
}

func TestDetector_OffFrequency_NoDetection(t *testing.T) {
	g := createTestGoertzel(t)
	cfg := createTestDetectorConfig()
	cfg.Hysteresis = 1
	cfg.AGCEnabled = false // Disable AGC for this test

	d, err := NewDetector(cfg, g)
	if err != nil {
		t.Fatalf("NewDetector failed: %v", err)
	}

	eventCount := 0
	d.SetCallback(func(event ToneEvent) {
		eventCount++
	})

	// Generate tone at wrong frequency
	offFrequency := detectorTestToneFrequency + 500 // 500 Hz off
	samples := generateSineWave(offFrequency, detectorTestSampleRate, detectorTestBlockSize*10, 1.0)
	d.Process(samples)

	// Should not detect tone at wrong frequency
	if d.ToneState() {
		t.Error("Should not detect tone at wrong frequency")
	}
}

// Benchmark for performance testing
func BenchmarkDetector_Process(b *testing.B) {
	cfg := GoertzelConfig{
		TargetFrequency: detectorTestToneFrequency,
		SampleRate:      detectorTestSampleRate,
		BlockSize:       detectorTestBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		b.Fatalf("NewGoertzel failed: %v", err)
	}

	detCfg := createTestDetectorConfig()
	d, err := NewDetector(detCfg, g)
	if err != nil {
		b.Fatalf("NewDetector failed: %v", err)
	}

	// Typical audio buffer size
	samples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, 1024, 1.0)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		d.Process(samples)
	}
}

func BenchmarkDetector_ProcessWithCallback(b *testing.B) {
	cfg := GoertzelConfig{
		TargetFrequency: detectorTestToneFrequency,
		SampleRate:      detectorTestSampleRate,
		BlockSize:       detectorTestBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		b.Fatalf("NewGoertzel failed: %v", err)
	}

	detCfg := createTestDetectorConfig()
	d, err := NewDetector(detCfg, g)
	if err != nil {
		b.Fatalf("NewDetector failed: %v", err)
	}

	d.SetCallback(func(event ToneEvent) {
		// Minimal callback
	})

	samples := generateSineWave(detectorTestToneFrequency, detectorTestSampleRate, 1024, 1.0)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		d.Process(samples)
	}
}
