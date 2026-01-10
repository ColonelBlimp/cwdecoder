package decode

import (
	"github.com/ColonelBlimp/cwdecoder/internal/config"
	"github.com/gen2brain/malgo"
)

type Decoder struct {
	cfg config.Config

	ctx    *malgo.AllocatedContext
	device *malgo.Device
}

type AudioDevice struct {
}

type AudioDeviceSlice []AudioDevice

func NewDecoder(cfg config.Config) (*Decoder, error) {
	return nil, nil
}

func ListAudioDevices() (AudioDeviceSlice, error) {
	return nil, nil
}
