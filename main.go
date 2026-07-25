package main

import (
	"fmt"
	"os"

	"github.com/moveeeax/tf-plan-summary/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "tf-plan-summary:", err)
		os.Exit(1)
	}
}
