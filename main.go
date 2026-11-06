package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/moveeeax/tf-plan-summary/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "tf-plan-summary:", err)
		var gate *cmd.GateError
		if errors.As(err, &gate) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
