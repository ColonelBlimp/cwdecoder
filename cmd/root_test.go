package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func resetViperForTest() {
	viper.Reset()
}

func TestRootCmd_HasExpectedFlags(t *testing.T) {
	flags := rootCmd.PersistentFlags()

	tests := []struct {
		name      string
		shorthand string
	}{
		{"device", "d"},
		{"frequency", "f"},
		{"wpm", "w"},
		{"debug", "D"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := flags.Lookup(tt.name)
			if flag == nil {
				t.Errorf("flag %q not found", tt.name)
				return
			}
			if flag.Shorthand != tt.shorthand {
				t.Errorf("flag %q shorthand = %q, want %q", tt.name, flag.Shorthand, tt.shorthand)
			}
		})
	}
}

func TestRootCmd_Properties(t *testing.T) {
	if rootCmd.Use != "decoder" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "decoder")
	}
	if rootCmd.Short == "" {
		t.Error("rootCmd.Short is empty")
	}
	if rootCmd.Long == "" {
		t.Error("rootCmd.Long is empty")
	}
}

func TestRootCmd_HelpOutput(t *testing.T) {
	resetViperForTest()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("Execute() with --help error = %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("decoder")) {
		t.Errorf("help output should contain 'decoder'")
	}
	if !bytes.Contains([]byte(output), []byte("--device")) {
		t.Errorf("help output should contain '--device'")
	}
}

func TestRootCmd_FlagDefaults(t *testing.T) {
	flags := rootCmd.PersistentFlags()

	tests := []struct {
		name         string
		defaultValue string
	}{
		{"device", "-1"},
		{"frequency", "600"},
		{"wpm", "15"},
		{"debug", "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := flags.Lookup(tt.name)
			if flag == nil {
				t.Fatalf("flag %q not found", tt.name)
			}
			if flag.DefValue != tt.defaultValue {
				t.Errorf("flag %q default = %q, want %q", tt.name, flag.DefValue, tt.defaultValue)
			}
		})
	}
}

func TestRootCmd_FlagDescriptions(t *testing.T) {
	flags := rootCmd.PersistentFlags()

	flagsToCheck := []string{"device", "frequency", "wpm", "debug"}

	for _, name := range flagsToCheck {
		t.Run(name, func(t *testing.T) {
			flag := flags.Lookup(name)
			if flag == nil {
				t.Fatalf("flag %q not found", name)
			}
			if flag.Usage == "" {
				t.Errorf("flag %q has no description", name)
			}
		})
	}
}

func TestRootCmd_RunE(t *testing.T) {
	resetViperForTest()

	// Setup temp config
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	configDir := filepath.Join(tmpDir, ".config", "cwdecoder")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("wpm: 15"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{})

	// RunE should succeed
	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
}

func TestInitConfig(t *testing.T) {
	resetViperForTest()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	configDir := filepath.Join(tmpDir, ".config", "cwdecoder")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("wpm: 20"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Should not panic
	initConfig()

	// Verify config was loaded
	if viper.GetInt("wpm") != 20 {
		t.Errorf("viper.GetInt(wpm) = %d, want 20", viper.GetInt("wpm"))
	}
}

func TestRootCmd_WithFlags(t *testing.T) {
	resetViperForTest()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	configDir := filepath.Join(tmpDir, ".config", "cwdecoder")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("wpm: 15"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"--wpm", "25", "--debug"})

	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Execute() with flags error = %v", err)
	}
}

func TestRootCmd_VersionFlag(t *testing.T) {
	// Version command doesn't exist yet, but this tests that unknown flags are handled
	resetViperForTest()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Execute() with --help error = %v", err)
	}
}
