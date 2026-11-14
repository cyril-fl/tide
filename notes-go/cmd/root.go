package cmd

import (
	"errors"
	"fmt"
	"os"

	"charm.land/huh/v2"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/screen"
	"github.com/cyril-fl/notes/notes-go/config"
	"github.com/spf13/cobra"
)

var c = config.Current
var cmd = c.Commands

var cmdRoot = &cobra.Command{
	Use:     c.Name,
	Short:   cmd.Root.Short,
	Long:    cmd.Root.Long,
	Version: config.Version(),
	Example: cmd.Root.Example,
	// Errors are printed by Execute so a form abort (Ctrl+C / Esc) stays silent.
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cobra *cobra.Command, args []string) error {
		return cobra.Help()
	},
}

// groups sorts the commands of the help by scope; a command left out shows under "Additional Commands".
var groups = []struct {
	group    cobra.Group
	commands []string
}{
	{cobra.Group{ID: "notes", Title: "Notes, folders and tags:"}, []string{"add", "show", "edit", "move", "remove"}},
	{cobra.Group{ID: "browse", Title: "Browse:"}, []string{"finder", "recent", "search", "graph"}},
	{cobra.Group{ID: "trash", Title: "Trash:"}, []string{"trash", "restore"}},
	{cobra.Group{ID: "files", Title: "Markdown files:"}, []string{"import", "export"}},
	{cobra.Group{ID: "account", Title: "Account:"}, []string{"login", "logout", "whoami", "account", "mcp"}},
}

func groupCommands() {
	byName := map[string]string{}
	for _, g := range groups {
		cmdRoot.AddGroup(&g.group)
		for _, name := range g.commands {
			byName[name] = g.group.ID
		}
	}
	for _, command := range cmdRoot.Commands() {
		command.GroupID = byName[command.Name()]
	}
}

func Execute() {
	groupCommands()
	err := cmdRoot.Execute()
	if errors.Is(err, huh.ErrUserAborted) || errors.Is(err, screen.ErrQuit) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
