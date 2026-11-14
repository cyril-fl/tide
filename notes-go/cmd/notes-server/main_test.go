package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestNeedsDb(t *testing.T) {
	completion := &cobra.Command{Use: "completion"}
	zsh := &cobra.Command{Use: "zsh"}
	request := &cobra.Command{Use: cobra.ShellCompRequestCmd}
	completion.AddCommand(zsh)
	root := &cobra.Command{Use: "notes-server"}
	root.AddCommand(completion, request)

	tests := map[string]struct {
		command *cobra.Command
		want    bool
	}{
		"root help":        {cmdRoot, false},
		"serve":            {cmdServe, true},
		"user add":         {cmdUserAdd, true},
		"rotate-secret":    {cmdRotateSecret, true},
		"completion":       {completion, false},
		"completion zsh":   {zsh, false},
		"shell __complete": {request, false},
	}

	for name, test := range tests {
		if got := needsDb(test.command); got != test.want {
			t.Errorf("%s: needsDb() = %v, want %v", name, got, test.want)
		}
	}
}
