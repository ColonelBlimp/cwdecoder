package main

import (
	"github.com/ColonelBlimp/cwdecoder/cmd"
	"github.com/ColonelBlimp/cwdecoder/internal/recovery"
)

func main() {
	defer recovery.HandlePanic()
	cmd.Execute()
}
