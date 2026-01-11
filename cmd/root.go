// cmd/root.go
package cmd

import (
	"fmt"
	"os"

	"github.com/ColonelBlimp/cwdecoder/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "decoder",
	Short: "CW (Morse code) decoder from audio input",
	Long:  `A real-time CW decoder that processes audio input and outputs decoded text.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Running decoder...")
		return nil
	},
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
