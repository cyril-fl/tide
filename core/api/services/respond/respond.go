// Package respond is what every route shares: reading the request, writing JSON, turning process errors into statuses.
package respond

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/hashtag"
	"github.com/cyril-fl/tide/core/services/process/adduser"
	"github.com/cyril-fl/tide/core/services/process/createapitoken"
	"github.com/cyril-fl/tide/core/services/process/createfile"
	"github.com/cyril-fl/tide/core/services/process/createfolder"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/createsession"
	"github.com/cyril-fl/tide/core/services/process/createtag"
	"github.com/cyril-fl/tide/core/services/process/deleteapitoken"
	"github.com/cyril-fl/tide/core/services/process/deleteitems"
	"github.com/cyril-fl/tide/core/services/process/deletetag"
	"github.com/cyril-fl/tide/core/services/process/deleteuser"
	"github.com/cyril-fl/tide/core/services/process/editfolder"
	"github.com/cyril-fl/tide/core/services/process/editnote"
	"github.com/cyril-fl/tide/core/services/process/editnotetext"
	"github.com/cyril-fl/tide/core/services/process/editpassword"
	"github.com/cyril-fl/tide/core/services/process/edittag"
	"github.com/cyril-fl/tide/core/services/process/edituser"
	"github.com/cyril-fl/tide/core/services/process/emptytrash"
	"github.com/cyril-fl/tide/core/services/process/exportnotes"
	"github.com/cyril-fl/tide/core/services/process/listitems"
	"github.com/cyril-fl/tide/core/services/process/mergefork"
	"github.com/cyril-fl/tide/core/services/process/moveitems"
	"github.com/cyril-fl/tide/core/services/process/patchnote"
	"github.com/cyril-fl/tide/core/services/process/pinnote"
	"github.com/cyril-fl/tide/core/services/process/readfile"
	"github.com/cyril-fl/tide/core/services/process/readfork"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/cyril-fl/tide/core/services/process/readnotes"
	"github.com/cyril-fl/tide/core/services/process/readsavedsearch"
	"github.com/cyril-fl/tide/core/services/process/readsession"
	"github.com/cyril-fl/tide/core/services/process/readtag"
	"github.com/cyril-fl/tide/core/services/process/replacenotes"
	"github.com/cyril-fl/tide/core/services/process/resetpassword"
	"github.com/cyril-fl/tide/core/services/process/resolveitem"
	"github.com/cyril-fl/tide/core/services/process/restoreitems"
	"github.com/cyril-fl/tide/core/services/process/searchnotes"
	"github.com/cyril-fl/tide/core/services/process/unmergefork"
)

// maxBody limits a request body: a note is text, 10 MiB is far beyond any real one.
const maxBody = 10 << 20

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var statuses = []struct {
	status int
	errs   []error
}{
	{http.StatusBadRequest, []error{
		createfolder.ErrNoTitle, createfolder.ErrParentNotFound,
		createfile.ErrNoData,
		createnote.ErrNoContent, createnote.ErrParentNotFound,
		createtag.ErrNoValue,
		deleteitems.ErrNoValue,
		deletetag.ErrNoValue,
		editfolder.ErrNoValue, editfolder.ErrNoTitle,
		editnote.ErrNoValue, editnote.ErrNoContent,
		editnotetext.ErrNoValue, editnotetext.ErrNoMatch,
		edituser.ErrInvalidLocale, edituser.ErrInvalidTheme, edituser.ErrInvalidColorMode, edituser.ErrInvalidView, edituser.ErrInvalidSort, edituser.ErrInvalidPlan, edituser.ErrInvalidAfterMerge, edituser.ErrInvalidRetention, edituser.ErrInvalidNeighbors, edituser.ErrInvalidAvatar, edituser.ErrInvalidSnippet, edituser.ErrInvalidSavedSearch, edituser.ErrInvalidSidebar,
		adduser.ErrNoName, adduser.ErrShortPassword, editpassword.ErrWrongPassword,
		createapitoken.ErrNoName, createapitoken.ErrLongName, createapitoken.ErrInvalidLifetime,
		edittag.ErrNoValue,
		hashtag.ErrInvalid,
		listitems.ErrInvalidID, listitems.ErrInvalidKind, listitems.ErrInvalidPage, listitems.ErrInvalidSort,
		moveitems.ErrNoValue, moveitems.ErrInvalidID, moveitems.ErrParentNotFound,
		readitem.ErrNoValue,
		readnotes.ErrInvalid,
		readtag.ErrNoValue,
		restoreitems.ErrNoValue, restoreitems.ErrInvalidID,
		replacenotes.ErrNoValue,
		searchnotes.ErrNoValue,
	}},
	{http.StatusUnauthorized, []error{
		createsession.ErrBadCredentials, readsession.ErrNoSession,
	}},
	{http.StatusForbidden, []error{
		adduser.ErrBadInvite, edituser.ErrNoSavedSearches,
	}},
	{http.StatusNotFound, []error{
		deleteapitoken.ErrNotFound, deleteuser.ErrNotFound, editfolder.ErrNotFound, editnote.ErrNotFound, emptytrash.ErrNotFound, edittag.ErrNotFound, exportnotes.ErrNotFound, exportnotes.ErrFolderNotFound, pinnote.ErrNotFound, readfile.ErrNotFound, readfork.ErrNotFork, readitem.ErrNotFound, readsavedsearch.ErrNotFound, readtag.ErrNotFound, resetpassword.ErrNotFound, resolveitem.ErrNotFound,
	}},
	{http.StatusUnsupportedMediaType, []error{
		createfile.ErrFileType,
	}},
	{http.StatusConflict, []error{
		domain.ErrFolderExists, edittag.ErrAlreadyExists, adduser.ErrNameTaken, deleteuser.ErrSelf, deleteuser.ErrLastAdmin, moveitems.ErrCycle, restoreitems.ErrParentInTrash, patchnote.ErrLocked, patchnote.ErrStale, edituser.ErrStale, mergefork.ErrOriginGone, unmergefork.ErrNoMerge, resolveitem.ErrAmbiguous, editnotetext.ErrMany,
	}},
}

// JSON writes data with this status.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("write response: %v", err)
	}
}

// List writes a list: an empty one is [], not null.
func List[T any](w http.ResponseWriter, status int, items []T) {
	JSON(w, status, append([]T{}, items...))
}

// Paging reads ?limit=&offset= for a process that lists by pages, 0 when left out (no limit: all of them).
// On false the response is already written.
func Paging(w http.ResponseWriter, r *http.Request) (limit, offset int, ok bool) {
	limit, errLimit := number(r.URL.Query().Get("limit"))
	offset, errOffset := number(r.URL.Query().Get("offset"))
	if errLimit != nil || errOffset != nil || limit < 0 || offset < 0 {
		fail(w, http.StatusBadRequest, "invalid limit or offset")
		return 0, 0, false
	}
	return limit, offset, true
}

// Total counts in X-Total-Count the whole list a page comes from. Before the body.
func Total(w http.ResponseWriter, total int) {
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
}

// Error writes the status of a process error. An unknown error is a bug or an outage:
// its detail may expose SQL or connection strings, so it only goes to the logs, with the request and its account.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	for _, group := range statuses {
		for _, known := range group.errs {
			if errors.Is(err, known) {
				fail(w, group.status, err.Error())
				return
			}
		}
	}

	log.Printf("internal error: %s %s account=%s: %v", r.Method, r.URL.Path, domain.UserID(r.Context()), err)
	fail(w, http.StatusInternalServerError, "internal error")
}

// Decode reads the JSON body into dest; on false the response is already written.
func Decode(w http.ResponseWriter, r *http.Request, dest any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	decoder.DisallowUnknownFields()

	err := decoder.Decode(dest)
	var tooLarge *http.MaxBytesError
	switch {
	case err == nil:
		return true
	case errors.As(err, &tooLarge):
		fail(w, http.StatusRequestEntityTooLarge, "request body too large")
	default:
		fail(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
	}
	return false
}

// ID reads {id} from the path. Not a uuid means no such item, and the check keeps Postgres from raising
// an error that not every process translates. On false the response is already written.
func ID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !uuid.MatchString(id) {
		fail(w, http.StatusNotFound, "item not found")
		return "", false
	}
	return id, true
}

// ValidIDs checks the uuids given in a body or a query, where empty means none (the root);
// on false the response is already written.
func ValidIDs(w http.ResponseWriter, ids ...string) bool {
	for _, id := range ids {
		if id != "" && !uuid.MatchString(id) {
			fail(w, http.StatusBadRequest, "not a uuid: "+id)
			return false
		}
	}
	return true
}

// Failure is the body of every error.
type Failure struct {
	Error string `json:"error"`
}

// number reads a query number, 0 when left out.
func number(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	return strconv.Atoi(value)
}

func fail(w http.ResponseWriter, status int, message string) {
	JSON(w, status, Failure{Error: message})
}
