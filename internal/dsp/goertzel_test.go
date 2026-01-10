// internal/dsp/goertzel_test.go
package dsp

import (
	"math"
	"testing"
)

// Test configuration constants - these mirror config file values
const (
	testSampleRate    = 48000.0
	testToneFrequency = 600.0
	testBlockSize     = 512
	testNyquistFreq   = testSampleRate / 2.0
	tolerancePercent  = 0.05 // 5% tolerance for floating point comparisons
)

// generateSineWave creates a sine wave at the specified frequency
func generateSineWave(frequency, sampleRate float64, numSamples int, amplitude float32) []float32 {
	samples := make([]float32, numSamples)
	for i := 0; i < numSamples; i++ {
		t := float64(i) / sampleRate
		samples[i] = amplitude * float32(math.Sin(2*math.Pi*frequency*t))
	}
	return samples
}

// generateSilence creates a buffer of silence (zeros)
func generateSilence(numSamples int) []float32 {
	return make([]float32, numSamples)
}

// generateNoise creates random noise samples
func generateNoise(numSamples int, amplitude float32) []float32 {
	samples := make([]float32, numSamples)
	// Simple deterministic "noise" for reproducible tests
	for i := 0; i < numSamples; i++ {
		// Use a simple hash-like function for reproducibility
		val := float32(math.Sin(float64(i*7919))) * amplitude
		samples[i] = val
	}
	return samples
}

func TestNewGoertzel_ValidConfig(t *testing.T) {
	cfg := GoertzelConfig{
		TargetFrequency: testToneFrequency,
		SampleRate:      testSampleRate,
		BlockSize:       testBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		t.Fatalf("NewGoertzel failed with valid config: %v", err)
	}

	if g == nil {
		t.Fatal("NewGoertzel returned nil with valid config")
	}

	// Verify config is stored
	if g.Config().TargetFrequency != testToneFrequency {
		t.Errorf("TargetFrequency mismatch: got %v, want %v", g.Config().TargetFrequency, testToneFrequency)
	}
	if g.Config().SampleRate != testSampleRate {
		t.Errorf("SampleRate mismatch: got %v, want %v", g.Config().SampleRate, testSampleRate)
	}
	if g.Config().BlockSize != testBlockSize {
		t.Errorf("BlockSize mismatch: got %v, want %v", g.Config().BlockSize, testBlockSize)
	}
}

func TestNewGoertzel_InvalidBlockSize(t *testing.T) {
	testCases := []struct {
		name      string
		blockSize int
	}{
		{"zero", 0},
		{"negative", -1},
		{"very negative", -100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := GoertzelConfig{
				TargetFrequency: testToneFrequency,
				SampleRate:      testSampleRate,
				BlockSize:       tc.blockSize,
			}

			_, err := NewGoertzel(cfg)
			if err != ErrInvalidBlockSize {
				t.Errorf("expected ErrInvalidBlockSize, got: %v", err)
			}
		})
	}
}

func TestNewGoertzel_InvalidSampleRate(t *testing.T) {
	testCases := []struct {
		name       string
		sampleRate float64
	}{
		{"zero", 0},
		{"negative", -48000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := GoertzelConfig{
				TargetFrequency: testToneFrequency,
				SampleRate:      tc.sampleRate,
				BlockSize:       testBlockSize,
			}

			_, err := NewGoertzel(cfg)
			if err != ErrInvalidSampleRate {
				t.Errorf("expected ErrInvalidSampleRate, got: %v", err)
			}
		})
	}
}

func TestNewGoertzel_InvalidFrequency(t *testing.T) {
	testCases := []struct {
		name      string
		frequency float64
	}{
		{"zero", 0},
		{"negative", -600},
		{"at nyquist", testNyquistFreq},
		{"above nyquist", testNyquistFreq + 1000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := GoertzelConfig{
				TargetFrequency: tc.frequency,
				SampleRate:      testSampleRate,
				BlockSize:       testBlockSize,
			}

			_, err := NewGoertzel(cfg)
			if err != ErrInvalidFrequency {
				t.Errorf("expected ErrInvalidFrequency, got: %v", err)
			}
		})
	}
}

func TestGoertzel_CoefficientComputation(t *testing.T) {
	cfg := GoertzelConfig{
		TargetFrequency: testToneFrequency,
		SampleRate:      testSampleRate,
		BlockSize:       testBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		t.Fatalf("NewGoertzel failed: %v", err)
	}

	// Manually compute expected coefficient
	k := (testToneFrequency / testSampleRate) * float64(testBlockSize)
	omega := (2.0 * math.Pi * k) / float64(testBlockSize)
	expectedCoeff := 2.0 * math.Cos(omega)

	if math.Abs(g.Coefficient()-expectedCoeff) > 1e-10 {
		t.Errorf("Coefficient mismatch: got %v, want %v", g.Coefficient(), expectedCoeff)
	}
}

func TestGoertzel_Magnitude_PureSineWave(t *testing.T) {
	cfg := GoertzelConfig{
		TargetFrequency: testToneFrequency,
		SampleRate:      testSampleRate,
		BlockSize:       testBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		t.Fatalf("NewGoertzel failed: %v", err)
	}

	// Generate a pure sine wave at the target frequency
	samples := generateSineWave(testToneFrequency, testSampleRate, testBlockSize, 1.0)

	magnitude, err := g.Magnitude(samples)
	if err != nil {
		t.Fatalf("Magnitude failed: %v", err)
	}

	// For a pure sine wave at the target frequency, magnitude should be close to 1.0
	if magnitude < 0.9 || magnitude > 1.1 {
		t.Errorf("Expected magnitude ~1.0 for pure sine wave, got: %v", magnitude)
	}
}

func TestGoertzel_Magnitude_Silence(t *testing.T) {
	cfg := GoertzelConfig{
		TargetFrequency: testToneFrequency,
		SampleRate:      testSampleRate,
		BlockSize:       testBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		t.Fatalf("NewGoertzel failed: %v", err)
	}

	samples := generateSilence(testBlockSize)

	magnitude, err := g.Magnitude(samples)
	if err != nil {
		t.Fatalf("Magnitude failed: %v", err)
	}

	// Silence should produce zero or near-zero magnitude
	if magnitude > 0.001 {
		t.Errorf("Expected near-zero magnitude for silence, got: %v", magnitude)
	}
}

func TestGoertzel_Magnitude_OffFrequency(t *testing.T) {
	cfg := GoertzelConfig{
		TargetFrequency: testToneFrequency,
		SampleRate:      testSampleRate,
		BlockSize:       testBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		t.Fatalf("NewGoertzel failed: %v", err)
	}

	// Test with frequencies that are significantly off from target
	testCases := []struct {
		name      string
		frequency float64
	}{
		{"200 Hz below", testToneFrequency - 200},
		{"200 Hz above", testToneFrequency + 200},
		{"1000 Hz", 1000.0},
		{"2000 Hz", 2000.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			samples := generateSineWave(tc.frequency, testSampleRate, testBlockSize, 1.0)

			magnitude, err := g.Magnitude(samples)
			if err != nil {
				t.Fatalf("Magnitude failed: %v", err)
			}

			// Off-frequency signals should produce significantly lower magnitude
			if magnitude > 0.3 {
				t.Errorf("Expected low magnitude for off-frequency signal at %v Hz, got: %v", tc.frequency, magnitude)
			}
		})
	}
}

func TestGoertzel_Magnitude_FrequencySelectivity(t *testing.T) {
	cfg := GoertzelConfig{
		TargetFrequency: testToneFrequency,
		SampleRate:      testSampleRate,
		BlockSize:       testBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		t.Fatalf("NewGoertzel failed: %v", err)
	}

	// Magnitude at target frequency
	targetSamples := generateSineWave(testToneFrequency, testSampleRate, testBlockSize, 1.0)
	targetMag, _ := g.Magnitude(targetSamples)

	// Magnitude at adjacent frequency (50 Hz away)
	adjacentSamples := generateSineWave(testToneFrequency+50, testSampleRate, testBlockSize, 1.0)
	adjacentMag, _ := g.Magnitude(adjacentSamples)

	// Target frequency should have significantly higher magnitude
	if targetMag <= adjacentMag {
		t.Errorf("Target frequency magnitude (%v) should be greater than adjacent (%v)", targetMag, adjacentMag)
	}

	// Selectivity ratio should be meaningful (50Hz separation gives ~1.5x at 512 block size)
	ratio := targetMag / adjacentMag
	if ratio < 1.5 {
		t.Errorf("Expected selectivity ratio > 1.5, got: %v", ratio)
	}
}

func TestGoertzel_Magnitude_InsufficientSamples(t *testing.T) {
	cfg := GoertzelConfig{
		TargetFrequency: testToneFrequency,
		SampleRate:      testSampleRate,
		BlockSize:       testBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		t.Fatalf("NewGoertzel failed: %v", err)
	}

	// Test with fewer samples than block size
	samples := generateSineWave(testToneFrequency, testSampleRate, testBlockSize-1, 1.0)

	_, err = g.Magnitude(samples)
	if err != ErrInsufficientSamples {
		t.Errorf("expected ErrInsufficientSamples, got: %v", err)
	}
}

func TestGoertzel_Magnitude_ExtraSamples(t *testing.T) {
	cfg := GoertzelConfig{
		TargetFrequency: testToneFrequency,
		SampleRate:      testSampleRate,
		BlockSize:       testBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		t.Fatalf("NewGoertzel failed: %v", err)
	}

	// Test with more samples than block size - should only use first BlockSize samples
	samples := generateSineWave(testToneFrequency, testSampleRate, testBlockSize*2, 1.0)

	magnitude, err := g.Magnitude(samples)
	if err != nil {
		t.Fatalf("Magnitude failed: %v", err)
	}

	if magnitude < 0.9 {
		t.Errorf("Expected magnitude ~1.0, got: %v", magnitude)
	}
}

func TestGoertzel_MagnitudeNoAlloc(t *testing.T) {
	cfg := GoertzelConfig{
		TargetFrequency: testToneFrequency,
		SampleRate:      testSampleRate,
		BlockSize:       testBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		t.Fatalf("NewGoertzel failed: %v", err)
	}

	samples := generateSineWave(testToneFrequency, testSampleRate, testBlockSize, 1.0)

	// MagnitudeNoAlloc should return same result as Magnitude
	magNoAlloc := g.MagnitudeNoAlloc(samples)
	magWithCheck, _ := g.Magnitude(samples)

	if math.Abs(magNoAlloc-magWithCheck) > 1e-10 {
		t.Errorf("MagnitudeNoAlloc result differs from Magnitude: %v vs %v", magNoAlloc, magWithCheck)
	}
}

func TestGoertzel_Magnitude_VaryingAmplitudes(t *testing.T) {
	cfg := GoertzelConfig{
		TargetFrequency: testToneFrequency,
		SampleRate:      testSampleRate,
		BlockSize:       testBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		t.Fatalf("NewGoertzel failed: %v", err)
	}

	testCases := []struct {
		name      string
		amplitude float32
	}{
		{"full amplitude", 1.0},
		{"half amplitude", 0.5},
		{"quarter amplitude", 0.25},
		{"low amplitude", 0.1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			samples := generateSineWave(testToneFrequency, testSampleRate, testBlockSize, tc.amplitude)

			magnitude, err := g.Magnitude(samples)
			if err != nil {
				t.Fatalf("Magnitude failed: %v", err)
			}

			// Magnitude should be proportional to input amplitude
			expectedMag := float64(tc.amplitude)
			tolerance := expectedMag * tolerancePercent
			if math.Abs(magnitude-expectedMag) > tolerance+0.05 {
				t.Errorf("Expected magnitude ~%v, got: %v", expectedMag, magnitude)
			}
		})
	}
}

func TestGoertzel_BlockSize(t *testing.T) {
	cfg := GoertzelConfig{
		TargetFrequency: testToneFrequency,
		SampleRate:      testSampleRate,
		BlockSize:       testBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		t.Fatalf("NewGoertzel failed: %v", err)
	}

	if g.BlockSize() != testBlockSize {
		t.Errorf("BlockSize() returned %v, expected %v", g.BlockSize(), testBlockSize)
	}
}

func TestGoertzel_DifferentBlockSizes(t *testing.T) {
	testCases := []struct {
		name      string
		blockSize int
	}{
		{"small block", 128},
		{"medium block", 512},
		{"large block", 1024},
		{"very large block", 2048},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := GoertzelConfig{
				TargetFrequency: testToneFrequency,
				SampleRate:      testSampleRate,
				BlockSize:       tc.blockSize,
			}

			g, err := NewGoertzel(cfg)
			if err != nil {
				t.Fatalf("NewGoertzel failed: %v", err)
			}

			samples := generateSineWave(testToneFrequency, testSampleRate, tc.blockSize, 1.0)

			magnitude, err := g.Magnitude(samples)
			if err != nil {
				t.Fatalf("Magnitude failed: %v", err)
			}

			// Should still detect the frequency regardless of block size
			if magnitude < 0.8 {
				t.Errorf("Expected high magnitude for block size %d, got: %v", tc.blockSize, magnitude)
			}
		})
	}
}

func TestGoertzel_DifferentSampleRates(t *testing.T) {
	testCases := []struct {
		name       string
		sampleRate float64
	}{
		{"8 kHz", 8000},
		{"16 kHz", 16000},
		{"44.1 kHz", 44100},
		{"48 kHz", 48000},
		{"96 kHz", 96000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := GoertzelConfig{
				TargetFrequency: testToneFrequency,
				SampleRate:      tc.sampleRate,
				BlockSize:       testBlockSize,
			}

			g, err := NewGoertzel(cfg)
			if err != nil {
				t.Fatalf("NewGoertzel failed: %v", err)
			}

			samples := generateSineWave(testToneFrequency, tc.sampleRate, testBlockSize, 1.0)

			magnitude, err := g.Magnitude(samples)
			if err != nil {
				t.Fatalf("Magnitude failed: %v", err)
			}

			// Should detect the frequency at any valid sample rate
			if magnitude < 0.8 {
				t.Errorf("Expected high magnitude for sample rate %v, got: %v", tc.sampleRate, magnitude)
			}
		})
	}
}

// Benchmark for performance testing
func BenchmarkGoertzel_MagnitudeNoAlloc(b *testing.B) {
	cfg := GoertzelConfig{
		TargetFrequency: testToneFrequency,
		SampleRate:      testSampleRate,
		BlockSize:       testBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		b.Fatalf("NewGoertzel failed: %v", err)
	}

	samples := generateSineWave(testToneFrequency, testSampleRate, testBlockSize, 1.0)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = g.MagnitudeNoAlloc(samples)
	}
}

func BenchmarkGoertzel_Magnitude(b *testing.B) {
	cfg := GoertzelConfig{
		TargetFrequency: testToneFrequency,
		SampleRate:      testSampleRate,
		BlockSize:       testBlockSize,
	}

	g, err := NewGoertzel(cfg)
	if err != nil {
		b.Fatalf("NewGoertzel failed: %v", err)
	}

	samples := generateSineWave(testToneFrequency, testSampleRate, testBlockSize, 1.0)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = g.Magnitude(samples)
	}
}
