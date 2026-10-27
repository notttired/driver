package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"driver/internal/jobs"
	"driver/internal/store"
)

// maxBody caps request bodies; the biggest legitimate one is the notes.
const maxBody = 2 << 20

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeFailure maps an engine error onto the contract's status codes: an
// unknown job is 404, malformed input 400, a validation refusal 409, and
// anything else (a store failure) 500.
func writeFailure(w http.ResponseWriter, what string, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such job")
	case errors.Is(err, jobs.ErrBadInput):
		// the wrapped sentinel's own text is not part of the sentence
		writeError(w, http.StatusBadRequest, strings.TrimSuffix(err.Error(), ": "+jobs.ErrBadInput.Error()))
	case jobs.IsRefusal(err):
		writeError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("%s: %v", what, err)
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// decodePost enforces POST + JSON body; it has already answered when false.
func decodePost(w http.ResponseWriter, r *http.Request, into any) bool {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(into); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return false
	}
	return true
}
