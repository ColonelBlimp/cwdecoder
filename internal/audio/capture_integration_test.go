//go:build integration

package audio

import (
	"context"
	"testing"
	"time"
)

// These tests require actual audio hardware and are skipped by default.
// Run with: go test -tags=integration ./internal/audio

func TestCapture_Init_Integration(t *testing.T) {
	capture := New(DefaultConfig())
	defer capture.Close()

	err := capture.Init()
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if capture.ctx == nil {
		t.Error("Init() did not set context")
	}
}

func TestCapture_ListDevices_Integration(t *testing.T) {
	capture := New(DefaultConfig())
	defer capture.Close()

	if err := capture.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	devices, err := capture.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices() error = %v", err)
	}

	t.Logf("Found %d capture devices:", len(devices))
	for i, d := range devices {
		t.Logf("  [%d] %s", i, d.Name())
	}
}

func TestCapture_StartStop_Integration(t *testing.T) {
	capture := New(DefaultConfig())
	defer capture.Close()

	if err := capture.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := capture.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !capture.IsRunning() {
		t.Error("IsRunning() = false after Start()")
	}

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	if err := capture.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if capture.IsRunning() {
		t.Error("IsRunning() = true after Stop()")
	}
}

func TestCapture_ReceivesSamples_Integration(t *testing.T) {
	capture := New(DefaultConfig())
	defer capture.Close()

	if err := capture.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := capture.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case samples := <-capture.Samples:
		t.Logf("Received %d samples", len(samples))
		if len(samples) == 0 {
			t.Error("Received empty sample buffer")
		}
	case <-ctx.Done():
		t.Error("Timeout waiting for samples")
	}
}

func TestCapture_Callback_Integration(t *testing.T) {
	capture := New(DefaultConfig())
	defer capture.Close()

	if err := capture.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	callbackCalled := make(chan struct{})
	capture.SetCallback(func(samples []float32) {
		select {
		case callbackCalled <- struct{}{}:
		default:
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := capture.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case <-callbackCalled:
		t.Log("Callback was invoked")
	case <-ctx.Done():
		t.Error("Timeout waiting for callback")
	}
}

func TestCapture_Close_Integration(t *testing.T) {
	capture := New(DefaultConfig())

	if err := capture.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := capture.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := capture.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify state after close
	if capture.IsRunning() {
		t.Error("IsRunning() = true after Close()")
	}
}

func TestCapture_ContextCancellation_Integration(t *testing.T) {
	capture := New(DefaultConfig())
	defer capture.Close()

	if err := capture.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	if err := capture.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !capture.IsRunning() {
		t.Error("IsRunning() = false after Start()")
	}

	// Cancel context
	cancel()

	// Give goroutine time to handle cancellation
	time.Sleep(100 * time.Millisecond)

	if capture.IsRunning() {
		t.Error("IsRunning() = true after context cancellation")
	}
}
