// Package cmdmcp serves the account's notes to an agent over MCP, a client of the API like the rest of the CLI:
// the rules (titles, tags, links, trash, encryption) are those of the server.
package cmdmcp

import (
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// Serve speaks MCP on stdin and stdout: nothing else may be printed there.
func Serve(cobra *cobra.Command, _ []string) error {
	// The account is the session's, as for every client of the API: the handlers act for it.
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	server := mcp.NewServer(&mcp.Implementation{Name: config.Current.Name, Title: config.Title(), Version: cobra.Root().Version}, &mcp.ServerOptions{Instructions: Instructions(false)})
	AddTools(server, client, false)
	return server.Run(cobra.Context(), &mcp.StdioTransport{})
}

// AddTools gives the server the tools acting through client, for the account of its session; the reading ones only
// when readOnly.
func AddTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addSearchTools(server, client, readOnly)
	addNoteTools(server, client, readOnly)
	addFolderTools(server, client, readOnly)
	addTagTools(server, client, readOnly)
	addFileTools(server, client, readOnly)
	addSnippetTools(server, client, readOnly)
	addSavedSearchTools(server, client, readOnly)
}
