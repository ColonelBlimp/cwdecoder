package decode

import (
	"testing"

	"github.com/ColonelBlimp/cwdecoder/internal/config"
)

func TestNewDecoder(t *testing.T) {
	// Currently returns nil, nil - just verify it doesn't panic
	decoder, err := NewDecoder(config.Settings{})
	if err != nil {
		t.Errorf("NewDecoder() error = %v", err)
	}
	// Current implementation returns nil
	if decoder != nil {
		t.Errorf("NewDecoder() = %v, want nil (not yet implemented)", decoder)
	}
}

func TestListAudioDevices(t *testing.T) {
	// Currently returns nil, nil - just verify it doesn't panic
	devices, err := ListAudioDevices()
	if err != nil {
		t.Errorf("ListAudioDevices() error = %v", err)
	}
	// Current implementation returns nil
	if devices != nil {
		t.Errorf("ListAudioDevices() = %v, want nil (not yet implemented)", devices)
	}
}
