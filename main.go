package main

import (
	"fmt"
	"os"

	"github.com/hutchii/tk/internal/cli"
	"github.com/hutchii/tk/internal/tui"
)

func main() {
	if len(os.Args) > 1 {
		cli.Main(os.Args[1:])
		return
	}
	if err := tui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tk:", err)
		os.Exit(1)
	}
}
