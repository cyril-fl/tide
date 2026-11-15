// Package cmdadmin holds the commands of the server's admin, on the base: create accounts, reset their password and
// change the server's secret.
package cmdadmin

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/huhform"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/postgrey"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/adduser"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/resetpassword"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/rotatesecret"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/seedperf"
	"github.com/cyril-fl/tide/notes-go/config"
	"github.com/spf13/cobra"
)

// AddUser creates an account.
func AddUser(cobra *cobra.Command, args []string) error {
	password, err := newPassword(cobra)
	if err != nil {
		return err
	}

	process := adduser.Process(cobra.Context(), postgrey.GetDb(), adduser.Input{Name: args[0], Password: password})
	process.Run()
	if process.HasErr() {
		return process.GetErr()
	}
	fmt.Printf("Successfully created account %s\n", process.GetResult().Name)
	return nil
}

// SeedPerf creates an account of generated notes, 50,000 unless args[1] says otherwise, to measure the app at scale.
func SeedPerf(cobra *cobra.Command, args []string) error {
	notes := 50000
	if len(args) == 2 {
		var err error
		if notes, err = strconv.Atoi(args[1]); err != nil || notes < 1 {
			return fmt.Errorf("notes: %q is not a positive number", args[1])
		}
	}
	password, err := newPassword(cobra)
	if err != nil {
		return err
	}

	process := seedperf.Process(cobra.Context(), postgrey.GetDb(), seedperf.Input{Name: args[0], Password: password, Notes: notes})
	process.Run()
	if process.HasErr() {
		return process.GetErr()
	}
	if created, count := process.GetResult(); created {
		fmt.Printf("Successfully created account %s with %d notes\n", args[0], count)
	} else {
		fmt.Printf("Account %s exists: nothing done\n", args[0])
	}
	return nil
}

// ResetPassword gives an account a new password and logs all its devices out: for a forgotten one.
func ResetPassword(cobra *cobra.Command, args []string) error {
	password, err := newPassword(cobra)
	if err != nil {
		return err
	}

	process := resetpassword.Process(cobra.Context(), postgrey.GetDb(), args[0], password)
	process.Run()
	if process.HasErr() {
		return process.GetErr()
	}
	fmt.Printf("Successfully reset the password of %s: log in again on every device\n", args[0])
	return nil
}

// RotateSecret seals the keys of the accounts by TIDE_SECRET, in place of TIDE_OLD_SECRET.
func RotateSecret(cobra *cobra.Command, args []string) error {
	process := rotatesecret.Process(cobra.Context(), postgrey.GetDb(), os.Getenv(config.Env("OLD_SECRET")))
	process.Run()
	if process.HasErr() {
		return process.GetErr()
	}
	fmt.Printf("Successfully sealed %d account keys by the new %s: put it in .env.docker too, then make api\n", process.GetResult(), config.Env("SECRET"))
	return nil
}

// newPassword reads it from stdin with --password-stdin (scripts), else asks it twice.
func newPassword(cobra *cobra.Command) (string, error) {
	if fromStdin, _ := cobra.Flags().GetBool("password-stdin"); fromStdin {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	var password, again string
	err := huhform.New(
		huh.NewInput().Title("Password").EchoMode(huh.EchoModePassword).Value(&password),
		huh.NewInput().Title("Password again").EchoMode(huh.EchoModePassword).Value(&again).
			Validate(func(value string) error {
				if value != password {
					return errors.New("the passwords differ")
				}
				return nil
			}),
	).Run()
	return password, err
}
