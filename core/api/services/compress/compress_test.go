package compress

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	big := `["` + strings.Repeat("a", minSize) + `"]`
	tests := map[string]struct {
		accept, contentType, body string
		status                    int
		zipped                    bool
	}{
		"big JSON":            {"gzip, deflate, br", "application/json", big, http.StatusOK, true},
		"big JSON of a error": {"gzip", "application/json", big, http.StatusBadRequest, true},
		"client without gzip": {"", "application/json", big, http.StatusOK, false},
		"small JSON":          {"gzip", "application/json", `["a"]`, http.StatusCreated, false},
		"a file":              {"gzip", "image/png", big, http.StatusOK, false},
		"no body":             {"gzip", "", "", http.StatusNoContent, false},
	}

	for name, test := range tests {
		handler := New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if test.contentType != "" {
				w.Header().Set("Content-Type", test.contentType)
			}
			w.WriteHeader(test.status)
			io.WriteString(w, test.body)
		}))
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Accept-Encoding", test.accept)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		body := io.Reader(w.Body)
		if zipped := w.Header().Get("Content-Encoding") == "gzip"; zipped != test.zipped {
			t.Errorf("%s: gzip = %v, want %v", name, zipped, test.zipped)
		} else if zipped {
			zr, err := gzip.NewReader(w.Body)
			if err != nil {
				t.Fatalf("%s: not gzip: %v", name, err)
			}
			body = zr
		}
		data, _ := io.ReadAll(body)
		if string(data) != test.body || w.Code != test.status || w.Header().Get("Vary") != "Accept-Encoding" {
			t.Errorf("%s: status %d, vary %q, body of %d bytes; want %d and the body sent", name, w.Code, w.Header().Get("Vary"), len(data), test.status)
		}
	}
}
