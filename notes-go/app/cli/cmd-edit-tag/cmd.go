package cmdedittag

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/huh/v2"
	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/huhform"
	"github.com/spf13/cobra"
)

var errRequired = errors.New("required")

// Edit renames the tag through PATCH /api/tags/{value}, in the text of its notes too.
func Edit(cobra *cobra.Command, args []string) error {
	_, err := rename(cobra.Context(), args[0], args[1])
	return err
}

// Interactive renames the tag through a form pre-filled with its value, and returns the new value.
func Interactive(ctx context.Context, value string) (string, error) {
	to := value
	if err := huhform.New(huh.NewInput().Title("Tag").Value(&to).Validate(notBlank)).Run(); err != nil {
		return value, err
	}
	return rename(ctx, value, to)
}

func rename(ctx context.Context, from string, to string) (string, error) {
	client, err := apiclient.Read().Client()
	if err != nil {
		return from, err
	}
	response, err := client.PatchApiTagsValueWithResponse(ctx, from, apiclient.EdittagBody{Value: to})
	if err != nil {
		return from, err
	}
	if response.JSON200 == nil {
		return from, apiclient.Fail(response.HTTPResponse, response.Body)
	}

	fmt.Printf("Successfully renamed %q to %q\n", from, response.JSON200.Value)
	return response.JSON200.Value, nil
}

func notBlank(value string) error {
	if strings.TrimSpace(value) == "" {
		return errRequired
	}

	return nil
}
