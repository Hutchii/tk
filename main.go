package main

import (
	"fmt"
	"os"

	"github.com/hutchii/tk/internal/cli"
	"github.com/hutchii/tk/internal/store"
	"github.com/hutchii/tk/internal/tui"
)

func main() {
	open, title := store.Open, "tk"
	if len(os.Args) == 2 && os.Args[1] == "post" {
		open, title = store.OpenSocial, "tk posts"
	} else if len(os.Args) > 1 {
		cli.Main(os.Args[1:])
		return
	}
	if err := tui.Run(open, title); err != nil {
		fmt.Fprintln(os.Stderr, "tk:", err)
		os.Exit(1)
	}
}
