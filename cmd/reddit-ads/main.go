package main

import (
	"fmt"
	"os"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
