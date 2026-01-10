package audio

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DeviceIndex != -1 {
		t.Errorf("DefaultConfig().DeviceIndex = %d, want -1", cfg.DeviceIndex)
	}
	if cfg.SampleRate != 48000 {
		t.Errorf("DefaultConfig().SampleRate = %d, want 48000", cfg.SampleRate)
	}
	if cfg.Channels != 1 {
		t.Errorf("DefaultConfig().Channels = %d, want 1", cfg.Channels)
	}
	if cfg.BufferSize != 512 {
		t.Errorf("DefaultConfig().BufferSize = %d, want 512", cfg.BufferSize)
	}
}

func TestNew(t *testing.T) {
	cfg := Config{
		DeviceIndex: 2,
		SampleRate:  44100,
		Channels:    2,
		BufferSize:  1024,
	}

	capture := New(cfg)

	if capture == nil {
		t.Fatal("New() returned nil")
	}
	if capture.config.DeviceIndex != 2 {
		t.Errorf("capture.config.DeviceIndex = %d, want 2", capture.config.DeviceIndex)
	}
	if capture.config.SampleRate != 44100 {
		t.Errorf("capture.config.SampleRate = %d, want 44100", capture.config.SampleRate)
	}
	if capture.Samples == nil {
		t.Error("capture.Samples channel is nil")
	}
}

func TestNew_ChannelBufferSize(t *testing.T) {
	capture := New(DefaultConfig())

	// Channel should be buffered with 64 capacity
	if cap(capture.Samples) != 64 {
		t.Errorf("capture.Samples capacity = %d, want 64", cap(capture.Samples))
	}
}

func TestCapture_IsRunning_InitialState(t *testing.T) {
	capture := New(DefaultConfig())

	if capture.IsRunning() {
		t.Error("IsRunning() = true for new capture, want false")
	}
}

func TestCapture_SetCallback(t *testing.T) {
	capture := New(DefaultConfig())

	capture.SetCallback(func(samples []float32) {
		// callback set
	})

	// Verify callback is set using atomic load
	if capture.callbackPtr.Load() == nil {
		t.Error("SetCallback() did not set callback")
	}
}

func TestCapture_SetCallback_Nil(t *testing.T) {
	capture := New(DefaultConfig())

	// Set a callback first
	capture.SetCallback(func(samples []float32) {})

	// Then set to nil
	capture.SetCallback(nil)

	if capture.callbackPtr.Load() != nil {
		t.Error("SetCallback(nil) should clear callback")
	}
}

func TestCapture_ListDevices_NotInitialized(t *testing.T) {
	capture := New(DefaultConfig())

	_, err := capture.ListDevices()
	if err != ErrNotInitialized {
		t.Errorf("ListDevices() error = %v, want ErrNotInitialized", err)
	}
}

func TestCapture_Start_NotInitialized(t *testing.T) {
	capture := New(DefaultConfig())
	ctx := context.Background()

	err := capture.Start(ctx)
	if err != ErrNotInitialized {
		t.Errorf("Start() error = %v, want ErrNotInitialized", err)
	}
}

func TestCapture_Start_AlreadyRunning(t *testing.T) {
	capture := New(DefaultConfig())

	// Manually set running state to simulate already running
	capture.running.Store(true)

	ctx := context.Background()
	err := capture.Start(ctx)
	if err != ErrAlreadyRunning {
		t.Errorf("Start() when running error = %v, want ErrAlreadyRunning", err)
	}
}

func TestCapture_Stop_NotRunning(t *testing.T) {
	capture := New(DefaultConfig())

	err := capture.Stop()
	if err != ErrNotRunning {
		t.Errorf("Stop() error = %v, want ErrNotRunning", err)
	}
}

func TestBytesToFloat32_Empty(t *testing.T) {
	result := bytesToFloat32([]byte{})
	if len(result) != 0 {
		t.Errorf("bytesToFloat32(empty) length = %d, want 0", len(result))
	}
}

func TestBytesToFloat32_SingleSample(t *testing.T) {
	// IEEE 754 representation of 1.0 in little-endian
	// 1.0 = 0x3F800000
	bytes := []byte{0x00, 0x00, 0x80, 0x3F}

	result := bytesToFloat32(bytes)

	if len(result) != 1 {
		t.Fatalf("bytesToFloat32() length = %d, want 1", len(result))
	}
	if result[0] != 1.0 {
		t.Errorf("bytesToFloat32() = %f, want 1.0", result[0])
	}
}

func TestBytesToFloat32_MultipleSamples(t *testing.T) {
	// 0.0 = 0x00000000, 1.0 = 0x3F800000, -1.0 = 0xBF800000
	bytes := []byte{
		0x00, 0x00, 0x00, 0x00, // 0.0
		0x00, 0x00, 0x80, 0x3F, // 1.0
		0x00, 0x00, 0x80, 0xBF, // -1.0
	}

	result := bytesToFloat32(bytes)

	if len(result) != 3 {
		t.Fatalf("bytesToFloat32() length = %d, want 3", len(result))
	}

	expected := []float32{0.0, 1.0, -1.0}
	for i, exp := range expected {
		if result[i] != exp {
			t.Errorf("bytesToFloat32()[%d] = %f, want %f", i, result[i], exp)
		}
	}
}

func TestBytesToFloat32_PartialBytes(t *testing.T) {
	// Only 3 bytes - should produce 0 samples (need 4 bytes per float32)
	bytes := []byte{0x00, 0x00, 0x80}

	result := bytesToFloat32(bytes)

	if len(result) != 0 {
		t.Errorf("bytesToFloat32(3 bytes) length = %d, want 0", len(result))
	}
}

func TestBytesToFloat32_ExtraBytes(t *testing.T) {
	// 5 bytes - should produce 1 sample (truncates extra bytes)
	bytes := []byte{0x00, 0x00, 0x80, 0x3F, 0xFF}

	result := bytesToFloat32(bytes)

	if len(result) != 1 {
		t.Errorf("bytesToFloat32(5 bytes) length = %d, want 1", len(result))
	}
	if result[0] != 1.0 {
		t.Errorf("bytesToFloat32(5 bytes)[0] = %f, want 1.0", result[0])
	}
}

func TestBytesToFloat32_SpecialValues(t *testing.T) {
	tests := []struct {
		name     string
		bytes    []byte
		expected float32
	}{
		{
			name:     "positive zero",
			bytes:    []byte{0x00, 0x00, 0x00, 0x00},
			expected: 0.0,
		},
		{
			name:     "0.5",
			bytes:    []byte{0x00, 0x00, 0x00, 0x3F}, // 0x3F000000
			expected: 0.5,
		},
		{
			name:     "-0.5",
			bytes:    []byte{0x00, 0x00, 0x00, 0xBF}, // 0xBF000000
			expected: -0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bytesToFloat32(tt.bytes)
			if len(result) != 1 {
				t.Fatalf("length = %d, want 1", len(result))
			}
			if result[0] != tt.expected {
				t.Errorf("got %f, want %f", result[0], tt.expected)
			}
		})
	}
}

func TestFloat32frombits(t *testing.T) {
	tests := []struct {
		bits     uint32
		expected float32
	}{
		{0x00000000, 0.0},
		{0x3F800000, 1.0},
		{0xBF800000, -1.0},
		{0x40000000, 2.0},
		{0x3F000000, 0.5},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := float32frombits(tt.bits)
			if result != tt.expected {
				t.Errorf("float32frombits(0x%08X) = %f, want %f", tt.bits, result, tt.expected)
			}
		})
	}
}

func TestFloat32frombits_NaN(t *testing.T) {
	// NaN representation
	result := float32frombits(0x7FC00000)
	if !math.IsNaN(float64(result)) {
		t.Errorf("float32frombits(NaN bits) = %f, want NaN", result)
	}
}

func TestFloat32frombits_Infinity(t *testing.T) {
	// Positive infinity
	posInf := float32frombits(0x7F800000)
	if !math.IsInf(float64(posInf), 1) {
		t.Errorf("float32frombits(+Inf) = %f, want +Inf", posInf)
	}

	// Negative infinity
	negInf := float32frombits(0xFF800000)
	if !math.IsInf(float64(negInf), -1) {
		t.Errorf("float32frombits(-Inf) = %f, want -Inf", negInf)
	}
}

func TestErrors(t *testing.T) {
	if ErrNotInitialized.Error() != "audio capture not initialized" {
		t.Errorf("ErrNotInitialized message wrong")
	}
	if ErrAlreadyRunning.Error() != "audio capture already running" {
		t.Errorf("ErrAlreadyRunning message wrong")
	}
	if ErrNotRunning.Error() != "audio capture not running" {
		t.Errorf("ErrNotRunning message wrong")
	}
}

func TestCapture_ConcurrentAccess(t *testing.T) {
	capture := New(DefaultConfig())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = capture.IsRunning()
		}()
	}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			capture.SetCallback(func(samples []float32) {})
		}()
	}

	wg.Wait()
}

func TestCapture_ConcurrentSetCallbackAndRead(t *testing.T) {
	capture := New(DefaultConfig())

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					capture.SetCallback(func(samples []float32) {})
				}
			}
		}()
	}

	// Readers (simulating callback access pattern using atomic)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					_ = capture.callbackPtr.Load()
				}
			}
		}()
	}

	wg.Wait()
}

func TestConfig_ZeroValue(t *testing.T) {
	var cfg Config

	if cfg.DeviceIndex != 0 {
		t.Errorf("zero Config.DeviceIndex = %d, want 0", cfg.DeviceIndex)
	}
	if cfg.SampleRate != 0 {
		t.Errorf("zero Config.SampleRate = %d, want 0", cfg.SampleRate)
	}
}

func TestConfig_CustomValues(t *testing.T) {
	cfg := Config{
		DeviceIndex: 5,
		SampleRate:  96000,
		Channels:    2,
		BufferSize:  2048,
	}

	if cfg.DeviceIndex != 5 {
		t.Errorf("Config.DeviceIndex = %d, want 5", cfg.DeviceIndex)
	}
	if cfg.SampleRate != 96000 {
		t.Errorf("Config.SampleRate = %d, want 96000", cfg.SampleRate)
	}
	if cfg.Channels != 2 {
		t.Errorf("Config.Channels = %d, want 2", cfg.Channels)
	}
	if cfg.BufferSize != 2048 {
		t.Errorf("Config.BufferSize = %d, want 2048", cfg.BufferSize)
	}
}

func TestBytesToFloat32_LargeBuffer(t *testing.T) {
	// Simulate a typical audio buffer (512 samples)
	numSamples := 512
	bytes := make([]byte, numSamples*4)

	// Fill with alternating 1.0 and -1.0 (square wave)
	for i := 0; i < numSamples; i++ {
		offset := i * 4
		if i%2 == 0 {
			// 1.0 = 0x3F800000
			bytes[offset] = 0x00
			bytes[offset+1] = 0x00
			bytes[offset+2] = 0x80
			bytes[offset+3] = 0x3F
		} else {
			// -1.0 = 0xBF800000
			bytes[offset] = 0x00
			bytes[offset+1] = 0x00
			bytes[offset+2] = 0x80
			bytes[offset+3] = 0xBF
		}
	}

	result := bytesToFloat32(bytes)

	if len(result) != numSamples {
		t.Fatalf("length = %d, want %d", len(result), numSamples)
	}

	for i, sample := range result {
		expected := float32(1.0)
		if i%2 != 0 {
			expected = -1.0
		}
		if sample != expected {
			t.Errorf("sample[%d] = %f, want %f", i, sample, expected)
		}
	}
}

func TestBytesAsFloat32_ZeroCopy(t *testing.T) {
	// 1.0 = 0x3F800000 in little-endian
	bytes := []byte{0x00, 0x00, 0x80, 0x3F, 0x00, 0x00, 0x80, 0xBF}

	result := bytesAsFloat32(bytes)

	if len(result) != 2 {
		t.Fatalf("length = %d, want 2", len(result))
	}
	if result[0] != 1.0 {
		t.Errorf("result[0] = %f, want 1.0", result[0])
	}
	if result[1] != -1.0 {
		t.Errorf("result[1] = %f, want -1.0", result[1])
	}
}

func TestBytesAsFloat32_Empty(t *testing.T) {
	result := bytesAsFloat32([]byte{})
	if result != nil {
		t.Errorf("bytesAsFloat32(empty) = %v, want nil", result)
	}
}

func TestBytesAsFloat32_TooSmall(t *testing.T) {
	result := bytesAsFloat32([]byte{0x00, 0x00, 0x80})
	if result != nil {
		t.Errorf("bytesAsFloat32(3 bytes) = %v, want nil", result)
	}
}

func TestCopyFloat32Slice(t *testing.T) {
	original := []float32{1.0, 2.0, 3.0}
	copied := copyFloat32Slice(original)

	if len(copied) != len(original) {
		t.Fatalf("length = %d, want %d", len(copied), len(original))
	}

	// Verify values match
	for i := range original {
		if copied[i] != original[i] {
			t.Errorf("copied[%d] = %f, want %f", i, copied[i], original[i])
		}
	}

	// Verify it's a true copy (modifying original doesn't affect copy)
	original[0] = 999.0
	if copied[0] == 999.0 {
		t.Error("copyFloat32Slice did not create independent copy")
	}
}

func TestCopyFloat32Slice_Nil(t *testing.T) {
	result := copyFloat32Slice(nil)
	if result != nil {
		t.Errorf("copyFloat32Slice(nil) = %v, want nil", result)
	}
}

func TestCopyFloat32Slice_Empty(t *testing.T) {
	result := copyFloat32Slice([]float32{})
	if len(result) != 0 {
		t.Errorf("copyFloat32Slice(empty) length = %d, want 0", len(result))
	}
}

func BenchmarkBytesToFloat32(b *testing.B) {
	// 512 samples typical audio buffer
	data := make([]byte, 512*4)
	for i := range data {
		data[i] = byte(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bytesToFloat32(data)
	}
}

func BenchmarkBytesAsFloat32(b *testing.B) {
	// 512 samples typical audio buffer
	data := make([]byte, 512*4)
	for i := range data {
		data[i] = byte(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bytesAsFloat32(data)
	}
}

func BenchmarkCopyFloat32Slice(b *testing.B) {
	data := make([]float32, 512)
	for i := range data {
		data[i] = float32(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = copyFloat32Slice(data)
	}
}
