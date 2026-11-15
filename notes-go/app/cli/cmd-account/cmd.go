// Package cmdaccount logs the CLI in and out and empties the account, through the API.
package cmdaccount

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"charm.land/huh/v2"
	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/huhform"
	"github.com/cyril-fl/notes/notes-go/config"
	"github.com/spf13/cobra"
)

// Login asks the account's name and password and keeps the server and the session in the user's config folder, or
// with --print-token prints a session of its own, for NOTES_SESSION, and keeps nothing.
func Login(cobra *cobra.Command, args []string) error {
	var name, password string
	print, _ := cobra.Flags().GetBool("print-token")
	form := huhform.New(
		huh.NewInput().Title("Name").Value(&name),
		huh.NewInput().Title("Password").EchoMode(huh.EchoModePassword).Value(&password),
	)
	// The form on stderr leaves stdout to the token alone, for NOTES_SESSION=$(notes login <url> --print-token).
	if print && huhform.Input == nil {
		form = form.WithOutput(os.Stderr)
	}
	if err := form.Run(); err != nil {
		return err
	}

	client, err := apiclient.NewClientWithResponses(args[0])
	if err != nil {
		return err
	}
	asked := true
	response, err := client.PostApiLoginWithResponse(cobra.Context(), apiclient.CreatesessionInput{Name: name, Password: password, Token: &asked})
	if err != nil {
		return err
	}
	if response.JSON200 == nil || response.JSON200.Token == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	if print {
		fmt.Println(*response.JSON200.Token)
		return nil
	}

	session := apiclient.Session{URL: args[0], Token: *response.JSON200.Token}
	if err := session.Save(); err != nil {
		return err
	}
	fmt.Printf("Logged in as %s\n", response.JSON200.User.Name)
	return nil
}

// Logout ends the CLI's session, on the server and on disk; a NOTES_SESSION one on the server only, the file is
// another. A session the server no longer knows is removed from disk all the same.
func Logout(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.PostApiLogoutWithResponse(cobra.Context())
	if err != nil {
		return err
	}
	if code := response.StatusCode(); code != http.StatusNoContent && code != http.StatusUnauthorized {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	if strings.TrimSpace(os.Getenv(config.Env("SESSION"))) == "" {
		file, err := apiclient.SessionFile()
		if err != nil {
			return err
		}
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	fmt.Println("Logged out")
	return nil
}

// Whoami prints the account of the CLI's session and its server.
func Whoami(cobra *cobra.Command, args []string) error {
	session := apiclient.Read()
	client, err := session.Client()
	if err != nil {
		return err
	}
	response, err := client.GetApiMeWithResponse(cobra.Context())
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	fmt.Printf("%s on %s\n", response.JSON200.Name, session.URL)
	return nil
}

// Purge deletes for good every note, folder, tag and image of the account logged in, once its name (GET /api/me) is
// typed, through DELETE /api/me/data.
func Purge(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	me, err := client.GetApiMeWithResponse(cobra.Context())
	if err != nil {
		return err
	}
	if me.JSON200 == nil {
		return apiclient.Fail(me.HTTPResponse, me.Body)
	}
	name := me.JSON200.Name

	var typed string
	if err := huhform.New(
		huh.NewInput().Title(fmt.Sprintf("Type %s to delete every note, folder, tag and image for good", name)).Value(&typed).
			Validate(sameName(name)),
	).Run(); err != nil {
		return err
	}

	response, err := client.DeleteApiMeDataWithResponse(cobra.Context())
	if err != nil {
		return err
	}
	if response.StatusCode() != http.StatusNoContent {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	fmt.Printf("Successfully emptied account %s\n", name)
	return nil
}

// sameName accepts the account's name only, exactly as the app's danger zone does.
func sameName(name string) func(string) error {
	return func(value string) error {
		if value != name {
			return errors.New("not the account's name")
		}
		return nil
	}
}
