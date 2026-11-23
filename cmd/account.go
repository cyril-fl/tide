package cmd

import (
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	cmdaccount "github.com/cyril-fl/tide/cli/cmd-account"
	"github.com/spf13/cobra"
)

var cmdLogin = &cobra.Command{
	Use:     cmd.Login.Use,
	Short:   cmd.Login.Short,
	Long:    cmd.Login.Long,
	Example: cmd.Login.Example,
	Args:    cobra.ExactArgs(1),
	RunE:    cmdaccount.Login,
}

var cmdLogout = &cobra.Command{
	Use:   cmd.Logout.Use,
	Short: cmd.Logout.Short,
	Args:  cobra.NoArgs,
	RunE:  cmdaccount.Logout,
}

var cmdWhoami = &cobra.Command{
	Use:     cmd.Whoami.Use,
	Short:   cmd.Whoami.Short,
	Example: cmd.Whoami.Example,
	Args:    cobra.NoArgs,
	RunE:    cmdaccount.Whoami,
}

var cmdAccount = &cobra.Command{
	Use:   cmd.Account.Use,
	Short: cmd.Account.Short,
}

var cmdAccountPurge = &cobra.Command{
	Use:     cmd.AccountPurge.Use,
	Short:   cmd.AccountPurge.Short,
	Long:    cmd.AccountPurge.Long,
	Example: cmd.AccountPurge.Example,
	Args:    cobra.NoArgs,
	RunE:    cmdaccount.Purge,
}

func init() {
	plumcliutilsargs.Makeflags(cmdLogin, cmd.Login.Flags)
	plumcliutilsargs.Makeflags(cmdWhoami, cmd.Whoami.Flags)

	cmdAccount.AddCommand(cmdAccountPurge)
	cmdRoot.AddCommand(cmdLogin, cmdLogout, cmdWhoami, cmdAccount)
}
