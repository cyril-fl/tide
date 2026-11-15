package apiclient

import (
	"context"
	"strings"
)

// Resolve turns a path (/Projects/Ideas) into the uuid of the item of this kind there; anything else is taken as a
// uuid, as the commands take <uuid|path>.
func Resolve(ctx context.Context, client *ClientWithResponses, kind DomainKind, ref string) (string, error) {
	if !strings.HasPrefix(ref, "/") {
		return ref, nil
	}
	asked := string(kind)
	response, err := client.GetApiResolveWithResponse(ctx, &GetApiResolveParams{Path: ref, Kind: &asked})
	if err != nil {
		return "", err
	}
	if response.JSON200 == nil {
		return "", Fail(response.HTTPResponse, response.Body)
	}
	return response.JSON200.Id, nil
}
