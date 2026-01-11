// cmd/root.go
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ColonelBlimp/cwdecoder/internal/audio"
	"github.com/ColonelBlimp/cwdecoder/internal/config"
	"github.com/ColonelBlimp/cwdecoder/internal/cw"
	"github.com/ColonelBlimp/cwdecoder/internal/dsp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "decoder",
	Short: "CW (Morse code) decoder from audio input",
	Long:  `A real-time CW decoder that processes audio input and outputs decoded text.`,
	RunE:  runDecoder,
}

// runDecoder is the main entry point that wires all components together.
func runDecoder(_ *cobra.Command, _ []string) error {
	// Get validated settings
	settings, err := config.Get()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if settings.Debug {
		fmt.Printf("Config: sample_rate=%.0f, tone_frequency=%.0f, block_size=%d\n",
			settings.SampleRate, settings.ToneFrequency, settings.BlockSize)
		fmt.Printf("Detection: threshold=%.2f, hysteresis=%d, agc_enabled=%v\n",
			settings.Threshold, settings.Hysteresis, settings.AGCEnabled)
	}

	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
		cancel()
	}()

	// Initialize audio capture
	audioConfig := audio.Config{
		DeviceIndex: settings.DeviceIndex,
		SampleRate:  uint32(settings.SampleRate),
		Channels:    uint32(settings.Channels),
		BufferSize:  uint32(settings.BufferSize),
	}
	capture := audio.New(audioConfig)

	if err := capture.Init(); err != nil {
		return fmt.Errorf("init audio: %w", err)
	}
	defer func() {
		if err := capture.Close(); err != nil {
			if _, printErr := fmt.Fprintf(os.Stderr, "error closing audio capture: %v\n", err); printErr != nil {
				// Fall back to standard print if Fprintf fails
				fmt.Println("error closing audio capture:", err)
			}
		}
	}()

	// List available devices if in debug mode
	if settings.Debug {
		devices, err := capture.ListDevices()
		if err != nil {
			if _, printErr := fmt.Fprintf(os.Stderr, "warning: could not list audio devices: %v\n", err); printErr != nil {
				fmt.Println("warning: could not list audio devices:", err)
			}
		} else {
			fmt.Printf("Available audio devices:\n")
			for i, dev := range devices {
				fmt.Printf("  [%d] %s\n", i, dev.Name())
			}
		}
	}

	// Initialize Goertzel filter for tone detection
	goertzelConfig := dsp.GoertzelConfig{
		TargetFrequency: settings.ToneFrequency,
		SampleRate:      settings.SampleRate,
		BlockSize:       settings.BlockSize,
	}
	goertzel, err := dsp.NewGoertzel(goertzelConfig)
	if err != nil {
		return fmt.Errorf("init goertzel: %w", err)
	}

	// Initialize tone detector
	detectorConfig := dsp.DetectorConfig{
		Threshold:       settings.Threshold,
		Hysteresis:      settings.Hysteresis,
		OverlapPct:      settings.OverlapPct,
		AGCEnabled:      settings.AGCEnabled,
		AGCDecay:        settings.AGCDecay,
		AGCAttack:       settings.AGCAttack,
		AGCWarmupBlocks: settings.AGCWarmupBlocks,
	}
	detector, err := dsp.NewDetector(detectorConfig, goertzel)
	if err != nil {
		return fmt.Errorf("init detector: %w", err)
	}

	// Initialize CW decoder
	cwDecoderConfig := cw.DecoderConfig{
		InitialWPM:        settings.WPM,
		AdaptiveTiming:    settings.AdaptiveTiming,
		AdaptiveSmoothing: settings.AdaptiveSmoothing,
		DitDahBoundary:    settings.DitDahBoundary,
		InterCharBoundary: settings.InterCharBoundary,
		CharWordBoundary:  settings.CharWordBoundary,
		FarnsworthWPM:     settings.FarnsworthWPM,
	}
	cwDecoder, err := cw.NewDecoder(cwDecoderConfig)
	if err != nil {
		return fmt.Errorf("init cw decoder: %w", err)
	}

	// Set up decoded output callback
	cwDecoder.SetCallback(func(output cw.DecodedOutput) {
		if output.IsWordSpace {
			fmt.Print(" ")
		} else if output.Character != 0 {
			fmt.Print(string(output.Character))
		}
		// Flush output for real-time display
		if err := os.Stdout.Sync(); err != nil {
			// Sync can fail on some terminals, ignore non-critical error
			_ = err
		}
	})

	// Wire detector to CW decoder
	detector.SetCallback(func(event dsp.ToneEvent) {
		if settings.Debug {
			if event.ToneOn {
				fmt.Printf("[TONE ON]  magnitude=%.3f\n", event.Magnitude)
			} else {
				fmt.Printf("[TONE OFF] duration=%v magnitude=%.3f\n",
					event.Duration, event.Magnitude)
			}
		}
		cwDecoder.HandleToneEvent(event)
	})

	// Wire audio capture to detector (direct callback for lowest latency)
	capture.SetCallback(func(samples []float32) {
		detector.Process(samples)
	})

	// Start audio capture
	fmt.Println("Starting CW decoder... Press Ctrl+C to stop.")
	if err := capture.Start(ctx); err != nil {
		return fmt.Errorf("start audio capture: %w", err)
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Stop capture gracefully
	if err := capture.Stop(); err != nil && err != audio.ErrNotRunning {
		if _, printErr := fmt.Fprintf(os.Stderr, "error stopping audio capture: %v\n", err); printErr != nil {
			fmt.Println("error stopping audio capture:", err)
		}
	}

	fmt.Println("CW decoder stopped.")
	return nil
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "execution error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags (override config file)
	rootCmd.PersistentFlags().IntP("device", "d", -1, "audio device index (-1 for default)")
	rootCmd.PersistentFlags().Float64P("frequency", "f", 600, "CW tone frequency in Hz")
	rootCmd.PersistentFlags().IntP("wpm", "w", 15, "initial WPM estimate")
	rootCmd.PersistentFlags().BoolP("debug", "D", false, "enable debug output")

	// Bind flags to viper
	cobra.CheckErr(viper.BindPFlag("device_index", rootCmd.PersistentFlags().Lookup("device")))
	cobra.CheckErr(viper.BindPFlag("tone_frequency", rootCmd.PersistentFlags().Lookup("frequency")))
	cobra.CheckErr(viper.BindPFlag("wpm", rootCmd.PersistentFlags().Lookup("wpm")))
	cobra.CheckErr(viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug")))
}

func initConfig() {
	if err := config.Init(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
}
