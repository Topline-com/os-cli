package main

import (
	"os"

	"github.com/Topline-com/os-cli/internal/commands"
)

func main() {
	if err := commands.Execute(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
