package cw

import (
	"sync"
	"testing"
	"time"
)

func TestNewAdaptiveDecoder(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	adaptiveConfig := AdaptiveConfig{
		Enabled:             true,
		MinConfidence:       0.7,
		AdjustmentRate:      0.1,
		MinMatchesForAdjust: 3,
	}

	adaptive := NewAdaptiveDecoder(decoder, adaptiveConfig)
	if adaptive == nil {
		t.Fatal("NewAdaptiveDecoder() returned nil")
	}
	if adaptive.decoder != decoder {
		t.Error("adaptive decoder should wrap the provided decoder")
	}
}

func TestNewAdaptiveDecoder_Defaults(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	// Test with zero config values - should use defaults
	adaptiveConfig := AdaptiveConfig{}
	adaptive := NewAdaptiveDecoder(decoder, adaptiveConfig)

	if adaptive.config.MinConfidence != MinPatternConfidence {
		t.Errorf("MinConfidence = %v, want %v", adaptive.config.MinConfidence, MinPatternConfidence)
	}
	if adaptive.config.AdjustmentRate != AdaptiveAdjustmentRate {
		t.Errorf("AdjustmentRate = %v, want %v", adaptive.config.AdjustmentRate, AdaptiveAdjustmentRate)
	}
	if adaptive.config.MinMatchesForAdjust != MinMatchesForAdjustment {
		t.Errorf("MinMatchesForAdjust = %v, want %v", adaptive.config.MinMatchesForAdjust, MinMatchesForAdjustment)
	}
}

func TestAdaptiveDecoder_RecordElement(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	adaptive := NewAdaptiveDecoder(decoder, AdaptiveConfig{Enabled: true})

	// Record some elements
	adaptive.RecordElement(false, 80*time.Millisecond, 80*time.Millisecond, false, false)
	adaptive.RecordElement(true, 240*time.Millisecond, 80*time.Millisecond, false, false)
	adaptive.RecordElement(false, 80*time.Millisecond, 240*time.Millisecond, true, false)

	if len(adaptive.elementBuffer) != 3 {
		t.Errorf("elementBuffer length = %d, want 3", len(adaptive.elementBuffer))
	}
}

func TestAdaptiveDecoder_BufferTrimming(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	adaptive := NewAdaptiveDecoder(decoder, AdaptiveConfig{Enabled: true})

	// Fill buffer beyond max
	for i := 0; i < MaxElementBuffer+10; i++ {
		adaptive.RecordElement(false, 80*time.Millisecond, 80*time.Millisecond, false, false)
	}

	if len(adaptive.elementBuffer) > MaxElementBuffer {
		t.Errorf("elementBuffer length = %d, should not exceed %d", len(adaptive.elementBuffer), MaxElementBuffer)
	}
}

func TestAdaptiveDecoder_SetCorrectedCallback(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	adaptive := NewAdaptiveDecoder(decoder, AdaptiveConfig{Enabled: true})

	called := false
	adaptive.SetCorrectedCallback(func(_ CorrectedOutput) {
		called = true
	})

	if adaptive.correctedCallback == nil {
		t.Error("correctedCallback should be set")
	}

	adaptive.SetCorrectedCallback(nil)
	if adaptive.correctedCallback != nil {
		t.Error("correctedCallback should be nil after setting to nil")
	}

	// Verify called variable was correctly not used (it's set in callback)
	_ = called
}

func TestAdaptiveDecoder_GetDecoder(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	adaptive := NewAdaptiveDecoder(decoder, AdaptiveConfig{Enabled: true})

	if adaptive.GetDecoder() != decoder {
		t.Error("GetDecoder() should return the wrapped decoder")
	}
}

func TestAdaptiveDecoder_Reset(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	adaptive := NewAdaptiveDecoder(decoder, AdaptiveConfig{Enabled: true})

	// Add some data
	adaptive.RecordElement(false, 80*time.Millisecond, 80*time.Millisecond, true, false)
	adaptive.patternMatches["CQ"] = 5

	adaptive.Reset()

	if len(adaptive.elementBuffer) != 0 {
		t.Error("elementBuffer should be empty after Reset()")
	}
	if len(adaptive.patternMatches) != 0 {
		t.Error("patternMatches should be empty after Reset()")
	}
}

func TestAdaptiveDecoder_GetPatternMatchCounts(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	adaptive := NewAdaptiveDecoder(decoder, AdaptiveConfig{Enabled: true})

	adaptive.patternMatches["CQ"] = 5
	adaptive.patternMatches["DE"] = 3

	counts := adaptive.GetPatternMatchCounts()

	if counts["CQ"] != 5 {
		t.Errorf("CQ count = %d, want 5", counts["CQ"])
	}
	if counts["DE"] != 3 {
		t.Errorf("DE count = %d, want 3", counts["DE"])
	}

	// Modify returned map shouldn't affect internal state
	counts["CQ"] = 100
	if adaptive.patternMatches["CQ"] != 5 {
		t.Error("modifying returned map should not affect internal state")
	}
}

func TestCommonPatterns_CQ(t *testing.T) {
	// Verify CQ pattern is correct: -.-. --.-
	var cqPattern *MorsePattern
	for i := range CommonPatterns {
		if CommonPatterns[i].Text == "CQ" {
			cqPattern = &CommonPatterns[i]
			break
		}
	}

	if cqPattern == nil {
		t.Fatal("CQ pattern not found in CommonPatterns")
	}

	// C = -.-. (dah dit dah dit)
	// Q = --.- (dah dah dit dah)
	expected := []bool{true, false, true, false, true, true, false, true}
	if len(cqPattern.Elements) != len(expected) {
		t.Fatalf("CQ elements length = %d, want %d", len(cqPattern.Elements), len(expected))
	}

	for i, e := range expected {
		if cqPattern.Elements[i] != e {
			t.Errorf("CQ element[%d] = %v, want %v", i, cqPattern.Elements[i], e)
		}
	}

	// Break should be after index 3 (after C: -.-.)
	if len(cqPattern.Breaks) != 1 || cqPattern.Breaks[0] != 3 {
		t.Errorf("CQ breaks = %v, want [3]", cqPattern.Breaks)
	}
}

func TestCommonPatterns_DE(t *testing.T) {
	var dePattern *MorsePattern
	for i := range CommonPatterns {
		if CommonPatterns[i].Text == "DE" {
			dePattern = &CommonPatterns[i]
			break
		}
	}

	if dePattern == nil {
		t.Fatal("DE pattern not found in CommonPatterns")
	}

	// D = -.. (dah dit dit)
	// E = . (dit)
	expected := []bool{true, false, false, false}
	if len(dePattern.Elements) != len(expected) {
		t.Fatalf("DE elements length = %d, want %d", len(dePattern.Elements), len(expected))
	}

	for i, e := range expected {
		if dePattern.Elements[i] != e {
			t.Errorf("DE element[%d] = %v, want %v", i, dePattern.Elements[i], e)
		}
	}

	// Break after index 2 (after D: -..)
	if len(dePattern.Breaks) != 1 || dePattern.Breaks[0] != 2 {
		t.Errorf("DE breaks = %v, want [2]", dePattern.Breaks)
	}
}

func TestCommonPatterns_73(t *testing.T) {
	var pattern *MorsePattern
	for i := range CommonPatterns {
		if CommonPatterns[i].Text == "73" {
			pattern = &CommonPatterns[i]
			break
		}
	}

	if pattern == nil {
		t.Fatal("73 pattern not found in CommonPatterns")
	}

	// 7 = --... (dah dah dit dit dit)
	// 3 = ...-- (dit dit dit dah dah)
	expected := []bool{true, true, false, false, false, false, false, false, true, true}
	if len(pattern.Elements) != len(expected) {
		t.Fatalf("73 elements length = %d, want %d", len(pattern.Elements), len(expected))
	}

	for i, e := range expected {
		if pattern.Elements[i] != e {
			t.Errorf("73 element[%d] = %v, want %v", i, pattern.Elements[i], e)
		}
	}

	// Break after index 4 (after 7: --...)
	if len(pattern.Breaks) != 1 || pattern.Breaks[0] != 4 {
		t.Errorf("73 breaks = %v, want [4]", pattern.Breaks)
	}
}

func TestCommonPatterns_5NN(t *testing.T) {
	var pattern *MorsePattern
	for i := range CommonPatterns {
		if CommonPatterns[i].Text == "5NN" {
			pattern = &CommonPatterns[i]
			break
		}
	}

	if pattern == nil {
		t.Fatal("5NN pattern not found in CommonPatterns")
	}

	// 5 = ..... (dit dit dit dit dit)
	// N = -. (dah dit)
	// N = -. (dah dit)
	expected := []bool{false, false, false, false, false, true, false, true, false}
	if len(pattern.Elements) != len(expected) {
		t.Fatalf("5NN elements length = %d, want %d", len(pattern.Elements), len(expected))
	}

	for i, e := range expected {
		if pattern.Elements[i] != e {
			t.Errorf("5NN element[%d] = %v, want %v", i, pattern.Elements[i], e)
		}
	}

	// Breaks should be after index 4 (after 5) and index 6 (after first N)
	if len(pattern.Breaks) != 2 || pattern.Breaks[0] != 4 || pattern.Breaks[1] != 6 {
		t.Errorf("5NN breaks = %v, want [4 6]", pattern.Breaks)
	}
}

func TestCommonPatterns_GM(t *testing.T) {
	var pattern *MorsePattern
	for i := range CommonPatterns {
		if CommonPatterns[i].Text == "GM" {
			pattern = &CommonPatterns[i]
			break
		}
	}

	if pattern == nil {
		t.Fatal("GM pattern not found in CommonPatterns")
	}

	// G = --. (dah dah dit)
	// M = -- (dah dah)
	expected := []bool{true, true, false, true, true}
	if len(pattern.Elements) != len(expected) {
		t.Fatalf("GM elements length = %d, want %d", len(pattern.Elements), len(expected))
	}

	for i, e := range expected {
		if pattern.Elements[i] != e {
			t.Errorf("GM element[%d] = %v, want %v", i, pattern.Elements[i], e)
		}
	}

	// Break after index 2 (after G: --.)
	if len(pattern.Breaks) != 1 || pattern.Breaks[0] != 2 {
		t.Errorf("GM breaks = %v, want [2]", pattern.Breaks)
	}
}

func TestAdaptiveDecoder_MatchPattern(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	adaptive := NewAdaptiveDecoder(decoder, AdaptiveConfig{
		Enabled:       true,
		MinConfidence: 0.7,
	})

	// Create elements matching "CQ" pattern
	// C = -.-. (dah dit dah dit)
	// Q = --.- (dah dah dit dah)
	elements := []Element{
		{IsDah: true, Duration: 240 * time.Millisecond, GapAfter: 80 * time.Millisecond, IsCharEnd: false},
		{IsDah: false, Duration: 80 * time.Millisecond, GapAfter: 80 * time.Millisecond, IsCharEnd: false},
		{IsDah: true, Duration: 240 * time.Millisecond, GapAfter: 80 * time.Millisecond, IsCharEnd: false},
		{IsDah: false, Duration: 80 * time.Millisecond, GapAfter: 240 * time.Millisecond, IsCharEnd: true}, // End of C
		{IsDah: true, Duration: 240 * time.Millisecond, GapAfter: 80 * time.Millisecond, IsCharEnd: false},
		{IsDah: true, Duration: 240 * time.Millisecond, GapAfter: 80 * time.Millisecond, IsCharEnd: false},
		{IsDah: false, Duration: 80 * time.Millisecond, GapAfter: 80 * time.Millisecond, IsCharEnd: false},
		{IsDah: true, Duration: 240 * time.Millisecond, GapAfter: 400 * time.Millisecond, IsCharEnd: true}, // End of Q
	}

	match := adaptive.findBestMatch(elements)

	if match == nil {
		t.Fatal("expected to find a match for CQ pattern")
	}
	if match.Pattern.Text != "CQ" {
		t.Errorf("matched pattern = %s, want CQ", match.Pattern.Text)
	}
	if match.Confidence < 0.7 {
		t.Errorf("confidence = %.2f, want >= 0.7", match.Confidence)
	}
}

func TestAdaptiveDecoder_DecodeElements(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	adaptive := NewAdaptiveDecoder(decoder, AdaptiveConfig{Enabled: true})

	// Elements for "E" (single dit with char end)
	elements := []Element{
		{IsDah: false, Duration: 80 * time.Millisecond, IsCharEnd: true},
	}

	result := adaptive.decodeElements(elements)
	if result != "E" {
		t.Errorf("decodeElements() = %q, want %q", result, "E")
	}

	// Elements for "T" (single dah with char end)
	elements = []Element{
		{IsDah: true, Duration: 240 * time.Millisecond, IsCharEnd: true},
	}

	result = adaptive.decodeElements(elements)
	if result != "T" {
		t.Errorf("decodeElements() = %q, want %q", result, "T")
	}
}

func TestAdaptiveDecoder_ConcurrentAccess(t *testing.T) {
	cfg := validConfig()
	decoder, err := NewDecoder(cfg)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}

	adaptive := NewAdaptiveDecoder(decoder, AdaptiveConfig{Enabled: true})

	var wg sync.WaitGroup

	// Multiple goroutines recording elements
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				adaptive.RecordElement(j%2 == 0, 80*time.Millisecond, 80*time.Millisecond, j%5 == 0, j%10 == 0)
			}
		}()
	}

	// Multiple goroutines reading match counts
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = adaptive.GetPatternMatchCounts()
			}
		}()
	}

	wg.Wait()
	// If we get here without race detector errors, test passes
}

func TestAbs(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{5.0, 5.0},
		{-5.0, 5.0},
		{0.0, 0.0},
		{-0.001, 0.001},
	}

	for _, tt := range tests {
		result := abs(tt.input)
		if result != tt.expected {
			t.Errorf("abs(%v) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestAdaptiveConstants(t *testing.T) {
	if MaxElementBuffer <= 0 {
		t.Error("MaxElementBuffer should be positive")
	}
	if MinPatternConfidence < 0 || MinPatternConfidence > 1 {
		t.Error("MinPatternConfidence should be between 0 and 1")
	}
	if AdaptiveAdjustmentRate < 0 || AdaptiveAdjustmentRate > 1 {
		t.Error("AdaptiveAdjustmentRate should be between 0 and 1")
	}
	if MinMatchesForAdjustment <= 0 {
		t.Error("MinMatchesForAdjustment should be positive")
	}
}
