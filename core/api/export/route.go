package export

import (
	"archive/zip"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/services/process/exportnotes"
	"github.com/cyril-fl/tide/core/services/process/readsavedsearch"
	"github.com/cyril-fl/tide/core/services/silo"
	"github.com/uptrace/bun"
)

// Handler gives every live note as a zip of markdown files, one directory per folder, the images in _files:
// GET /api/export, or only some items with what they hold: GET /api/export?id=<uuid>&id=<uuid>, or what a folder
// holds without the folder itself: GET /api/export?parent=<uuid>, or the notes a smart folder finds, side by side at
// the root: GET /api/export?search=<id>. files=false leaves the images empty, only named.
// Each entry of a folder or a note has its last change as its date and its id as its zip comment.
// @Summary  Export the notes as markdown files
// @Tags     transfer
// @Produce  application/zip
// @Param    id query []string false "items to export with what they hold, every note when left out" collectionFormat(multi)
// @Param    parent query string false "folder whose content to export, without the folder itself"
// @Param    search query string false "smart folder whose notes to export"
// @Param    files query bool false "false: the images in _files are empty, only named"
// @Success  200 {file} binary
// @Failure  400 {object} respond.Failure
// @Failure  404 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/export [get]
func Handler(db bun.IDB, store silo.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ids, parent, search := r.URL.Query()["id"], r.URL.Query().Get("parent"), r.URL.Query().Get("search")
		files := r.URL.Query().Get("files") != "false"
		if !respond.ValidIDs(w, append(ids, parent)...) {
			return
		}
		var exported []exportnotes.File
		if search != "" {
			found := readsavedsearch.Process(r.Context(), db, search)
			found.Run()
			if found.HasErr() {
				respond.Error(w, found.GetErr())
				return
			}
			ids = nil
			for _, match := range found.GetResult() {
				ids = append(ids, match.ID)
			}
		}
		// A smart folder finding nothing exports an empty zip, not every note.
		if search == "" || len(ids) > 0 {
			process := exportnotes.Process(r.Context(), db, parent, ids...)
			process.Run()
			if process.HasErr() {
				respond.Error(w, process.GetErr())
				return
			}
			exported = process.GetResult()
		}

		// Every image goes out: a big account outlasts the server's 30 s.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="notes-`+time.Now().Format("2006-01-02")+`.zip"`)
		archive := zip.NewWriter(w)
		for _, file := range exported {
			name := file.Path
			if file.Dir {
				name += "/" // Keeps the empty folders.
			}
			// The comment holds the item's id, for tide mcp to give it back: unzip shows neither.
			entry, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: file.Modified, Comment: file.ID})
			if err == nil && (files || file.Key == "") {
				err = write(r, store, entry, file)
			}
			if err != nil {
				// The status is sent: cut the connection, or net/http ends the body cleanly and the browser keeps a
				// broken zip as a finished download.
				log.Printf("write export: %v", err)
				panic(http.ErrAbortHandler)
			}
		}
		if err := archive.Close(); err != nil {
			log.Printf("write export: %v", err)
		}
	}
}

// write copies a note's text, or an image's bytes from Silo.
func write(r *http.Request, store silo.Store, entry io.Writer, file exportnotes.File) error {
	if file.Key == "" {
		_, err := io.WriteString(entry, file.Content)
		return err
	}
	body, err := store.Get(r.Context(), file.Key)
	if err != nil {
		return err
	}
	defer body.Close()
	_, err = io.Copy(entry, body)
	return err
}
