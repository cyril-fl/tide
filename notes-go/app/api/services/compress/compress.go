// Package compress sends the big JSON answers of the API as gzip: on Tailscale from a phone, weight costs more than
// the server's time.
package compress

import (
	"compress/gzip"
	"log"
	"net/http"
	"strings"
)

// minSize is the body under which gzip gains nothing worth its time.
const minSize = 2 << 10

// New compresses the JSON bodies of next of minSize at least, when the client takes gzip. Files (images, already
// compressed) and the export are not JSON: sent as they are.
func New(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		zw := &writer{ResponseWriter: w}
		defer zw.close()
		next.ServeHTTP(zw, r)
	})
}

// writer holds the status back until the first Write, which decides: respond.JSON writes a whole body at once.
type writer struct {
	http.ResponseWriter
	status  int
	started bool
	zip     *gzip.Writer
}

func (w *writer) WriteHeader(status int) {
	w.status = status
}

func (w *writer) Write(body []byte) (int, error) {
	if !w.started {
		header := w.Header()
		w.start(len(body) >= minSize && strings.HasPrefix(header.Get("Content-Type"), "application/json") && header.Get("Content-Encoding") == "")
	}
	if w.zip != nil {
		return w.zip.Write(body)
	}
	return w.ResponseWriter.Write(body)
}

// Unwrap lets http.ResponseController reach the real writer.
func (w *writer) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// start sends the status, after the headers of gzip when the body is compressed.
func (w *writer) start(zip bool) {
	w.started = true
	if zip {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length")
		// BestSpeed: the best level doubles the compression's time for a few percent less. Never fails: the level is valid.
		w.zip, _ = gzip.NewWriterLevel(w.ResponseWriter, gzip.BestSpeed)
	}
	if w.status != 0 {
		w.ResponseWriter.WriteHeader(w.status)
	}
}

func (w *writer) close() {
	if !w.started {
		w.start(false) // a status without a body
	}
	if w.zip == nil {
		return
	}
	if err := w.zip.Close(); err != nil {
		log.Printf("write response: %v", err)
	}
}
