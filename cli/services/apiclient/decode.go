package apiclient

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

// Decode reads a JSON answer into the generated type of the route; any status but 2xx is the API's error.
func Decode[T any](response *http.Response, err error) (T, error) {
	var out T
	if err != nil {
		return out, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return out, err
	}
	if response.StatusCode/100 != 2 {
		return out, Fail(response, body)
	}
	return out, json.Unmarshal(body, &out)
}

// Paged decodes a page of a list, and the X-Total-Count of the whole list.
func Paged[T any](response *http.Response, err error) (T, int, error) {
	out, err := Decode[T](response, err)
	if err != nil {
		return out, 0, err
	}
	total, err := strconv.Atoi(response.Header.Get("X-Total-Count"))
	return out, total, err
}
