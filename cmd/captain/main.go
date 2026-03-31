package main

import (
	"fmt"
	"os"

	"github.com/crherman7/captain/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
