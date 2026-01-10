package recovery

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
)

// TestHandlePanic_NoPanic verifies that HandlePanic does nothing when there's no panic
func TestHandlePanic_NoPanic(t *testing.T) {
	// This should not panic or exit
	func() {
		defer HandlePanic()
		// No panic here
	}()
	// If we get here, the test passed
}

// TestHandlePanicFunc_NoPanic verifies that HandlePanicFunc does nothing when there's no panic
func TestHandlePanicFunc_NoPanic(t *testing.T) {
	cleanupCalled := false

	func() {
		defer HandlePanicFunc(func() {
			cleanupCalled = true
		})
		// No panic here
	}()

	if cleanupCalled {
		t.Error("cleanup was called without a panic")
	}
}

// TestHandlePanicFunc_NilCleanup verifies that nil cleanup doesn't cause issues
func TestHandlePanicFunc_NilCleanup(t *testing.T) {
	// This should not panic even with nil cleanup
	func() {
		defer HandlePanicFunc(nil)
		// No panic here
	}()
}

// TestHandlePanic_ExitsOnPanic uses a subprocess to test panic behavior
func TestHandlePanic_ExitsOnPanic(t *testing.T) {
	if os.Getenv("TEST_PANIC_EXIT") == "1" {
		defer HandlePanic()
		panic("test panic")
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestHandlePanic_ExitsOnPanic")
	cmd.Env = append(os.Environ(), "TEST_PANIC_EXIT=1")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Should have exited with code 1
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 1 {
			t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
		}
	} else if err == nil {
		t.Error("expected process to exit with error, but it succeeded")
	}

	// Should have written to stderr
	output := stderr.String()
	if !bytes.Contains([]byte(output), []byte("FATAL")) {
		t.Errorf("stderr should contain 'FATAL', got: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("test panic")) {
		t.Errorf("stderr should contain 'test panic', got: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("Stack trace")) {
		t.Errorf("stderr should contain 'Stack trace', got: %s", output)
	}
}

// TestHandlePanicFunc_ExitsOnPanic uses a subprocess to test panic behavior with cleanup
func TestHandlePanicFunc_ExitsOnPanic(t *testing.T) {
	if os.Getenv("TEST_PANIC_FUNC_EXIT") == "1" {
		defer HandlePanicFunc(func() {
			// Write marker to stdout to verify cleanup was called
			_, _ = os.Stdout.WriteString("CLEANUP_CALLED\n")
		})
		panic("test panic func")
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestHandlePanicFunc_ExitsOnPanic")
	cmd.Env = append(os.Environ(), "TEST_PANIC_FUNC_EXIT=1")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Should have exited with code 1
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 1 {
			t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
		}
	} else if err == nil {
		t.Error("expected process to exit with error, but it succeeded")
	}

	// Cleanup should have been called
	if !bytes.Contains(stdout.Bytes(), []byte("CLEANUP_CALLED")) {
		t.Errorf("stdout should contain 'CLEANUP_CALLED', got: %s", stdout.String())
	}

	// Should have written error to stderr
	if !bytes.Contains(stderr.Bytes(), []byte("test panic func")) {
		t.Errorf("stderr should contain 'test panic func', got: %s", stderr.String())
	}
}
