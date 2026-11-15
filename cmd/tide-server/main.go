// tide-server is the only binary on the base and Silo: the API and the web app, and the commands of its admin.
// The CLI, tide, is a client of its API.
package main

import (
	"errors"
	"fmt"
	"os"

	"charm.land/huh/v2"
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/config"
	cmdadmin "github.com/cyril-fl/tide/core/server/cmd-admin"
	cmdserve "github.com/cyril-fl/tide/core/server/cmd-serve"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/spf13/cobra"
)

var cmd = config.Current.Commands

var cmdRoot = &cobra.Command{
	Use:           cmd.ServerRoot.Use,
	Short:         cmd.ServerRoot.Short,
	Long:          cmd.ServerRoot.Long,
	Version:       config.Version(),
	SilenceUsage:  true,
	SilenceErrors: true,

	// Runs before every subcommand: schema creation is idempotent (IF NOT EXISTS), and user add may come before the
	// first serve.
	PersistentPreRunE: func(cobra *cobra.Command, args []string) error {
		if !needsDb(cobra) {
			return nil
		}
		env := postgrey.NewEnvironment()
		env.ReadEnv()
		postgrey.SetDb(env.Connect())

		schema := createschema.Process(cobra.Context(), postgrey.GetDb())
		schema.Run()
		if schema.HasErr() {
			return schema.GetErr()
		}
		return nil
	},
	PersistentPostRunE: func(cobra *cobra.Command, args []string) error {
		if db := postgrey.GetDb(); db != nil {
			return db.Close()
		}
		return nil
	},
	RunE: func(cobra *cobra.Command, args []string) error {
		return cobra.Help()
	},
}

var cmdServe = &cobra.Command{
	Use:     cmd.Serve.Use,
	Short:   cmd.Serve.Short,
	Long:    cmd.Serve.Long,
	Example: cmd.Serve.Example,
	Args:    cobra.NoArgs,
	RunE:    cmdserve.Serve,
}

var cmdUser = &cobra.Command{
	Use:   cmd.User.Use,
	Short: cmd.User.Short,
}

var cmdUserAdd = &cobra.Command{
	Use:     cmd.UserAdd.Use,
	Short:   cmd.UserAdd.Short,
	Long:    cmd.UserAdd.Long,
	Example: cmd.UserAdd.Example,
	Args:    cobra.ExactArgs(1),
	RunE:    cmdadmin.AddUser,
}

// Hidden: a tool to measure the app, not something an admin needs.
var cmdUserPerf = &cobra.Command{
	Use:     cmd.UserPerf.Use,
	Short:   cmd.UserPerf.Short,
	Long:    cmd.UserPerf.Long,
	Example: cmd.UserPerf.Example,
	Args:    cobra.RangeArgs(1, 2),
	Hidden:  true,
	RunE:    cmdadmin.SeedPerf,
}

var cmdUserResetPassword = &cobra.Command{
	Use:     cmd.UserResetPassword.Use,
	Short:   cmd.UserResetPassword.Short,
	Long:    cmd.UserResetPassword.Long,
	Example: cmd.UserResetPassword.Example,
	Args:    cobra.ExactArgs(1),
	RunE:    cmdadmin.ResetPassword,
}

var cmdRotateSecret = &cobra.Command{
	Use:     cmd.RotateSecret.Use,
	Short:   cmd.RotateSecret.Short,
	Long:    cmd.RotateSecret.Long,
	Example: cmd.RotateSecret.Example,
	Args:    cobra.NoArgs,
	RunE:    cmdadmin.RotateSecret,
}

// needsDb is false for what only prints help or shell completion: those work with the base down.
func needsDb(command *cobra.Command) bool {
	if !command.HasParent() {
		return false
	}
	for ; command != nil; command = command.Parent() {
		switch command.Name() {
		case "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			return false
		}
	}
	return true
}

func init() {
	plumcliutilsargs.Makeflags(cmdServe, cmd.Serve.Flags)
	plumcliutilsargs.Makeflags(cmdUserAdd, cmd.UserAdd.Flags)
	plumcliutilsargs.Makeflags(cmdUserPerf, cmd.UserPerf.Flags)
	plumcliutilsargs.Makeflags(cmdUserResetPassword, cmd.UserResetPassword.Flags)

	cmdUser.AddCommand(cmdUserAdd, cmdUserPerf, cmdUserResetPassword)
	cmdRoot.AddCommand(cmdServe, cmdUser, cmdRotateSecret)
}

func main() {
	err := cmdRoot.Execute()
	if errors.Is(err, huh.ErrUserAborted) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
