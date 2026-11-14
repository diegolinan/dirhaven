package setupwizard

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/diegolinan/dirhaven/internal/configstore"
)

type savedResponse struct {
	OK       bool     `json:"ok"`
	Exists   bool     `json:"exists"`
	Mode     string   `json:"mode,omitempty"`
	Roots    []string `json:"roots,omitempty"`
	Revision string   `json:"revision,omitempty"`
	Error    string   `json:"error,omitempty"`
}

type saveRequest struct {
	Mode                   string   `json:"mode"`
	Paths                  []string `json:"paths"`
	AcknowledgeBroadAccess bool     `json:"acknowledgeBroadAccess"`
	ExpectedRevision       string   `json:"expectedRevision"`
}

type revokeRequest struct {
	ExpectedRevision string `json:"expectedRevision"`
}

func (w wizard) allowedAPI(out http.ResponseWriter, req *http.Request, method string) bool {
	if req.Method != method {
		http.Error(out, "method not permitted", http.StatusMethodNotAllowed)
		return false
	}
	if subtle.ConstantTimeCompare([]byte(req.Header.Get("X-Dirhaven-Setup-Token")), []byte(w.token)) != 1 {
		http.Error(out, "setup authorization required", http.StatusUnauthorized)
		return false
	}
	if w.store == nil {
		http.Error(out, "local saved configuration is unavailable", http.StatusNotImplemented)
		return false
	}
	return true
}

func (w wizard) handleSaved(out http.ResponseWriter, req *http.Request) {
	if !w.allowedAPI(out, req, http.MethodGet) {
		return
	}
	saved, revision, exists, err := w.store.Load()
	if err != nil {
		// Never overwrite or delete a malformed file without user intervention.
		writeSavedJSON(out, http.StatusConflict, savedResponse{Error: "Saved configuration is invalid or inaccessible. Existing data was not changed."})
		return
	}
	writeSavedJSON(out, http.StatusOK, savedResponse{OK: true, Exists: exists, Mode: saved.Mode, Roots: saved.Roots, Revision: revision})
}

func (w wizard) handleSave(out http.ResponseWriter, req *http.Request) {
	if !w.allowedAPI(out, req, http.MethodPost) {
		return
	}
	var input saveRequest
	if !decodeSavedJSON(out, req, &input) {
		return
	}
	prepared := w.prepareScope(scopeRequest{
		Mode:                   input.Mode,
		Paths:                  input.Paths,
		AcknowledgeBroadAccess: input.AcknowledgeBroadAccess,
	})
	if !prepared.OK {
		writeSavedJSON(out, http.StatusUnprocessableEntity, savedResponse{Error: prepared.Error})
		return
	}
	value := configstore.Scope{Version: configstore.SchemaVersion, Mode: input.Mode, Roots: prepared.Roots}
	revision, err := w.store.Save(value, input.ExpectedRevision)
	if err != nil {
		writeSavedJSON(out, http.StatusConflict, savedResponse{Error: "Cannot save configuration. Reload saved preferences first; the existing file was not changed."})
		return
	}
	writeSavedJSON(out, http.StatusOK, savedResponse{
		OK: true, Exists: true, Mode: input.Mode, Roots: prepared.Roots, Revision: revision,
	})
}

func (w wizard) handleRevoke(out http.ResponseWriter, req *http.Request) {
	if !w.allowedAPI(out, req, http.MethodPost) {
		return
	}
	var input revokeRequest
	if !decodeSavedJSON(out, req, &input) {
		return
	}
	if input.ExpectedRevision == "" {
		writeSavedJSON(out, http.StatusConflict, savedResponse{Error: "Load saved preferences before revoking them."})
		return
	}
	if err := w.store.Remove(input.ExpectedRevision); err != nil {
		writeSavedJSON(out, http.StatusConflict, savedResponse{Error: "Cannot revoke saved preferences. Reload first; the existing file was not changed."})
		return
	}
	writeSavedJSON(out, http.StatusOK, savedResponse{OK: true, Exists: false})
}

func decodeSavedJSON(out http.ResponseWriter, req *http.Request, destination any) bool {
	if req.Header.Get("Content-Type") != "application/json" {
		http.Error(out, "JSON required", http.StatusUnsupportedMediaType)
		return false
	}
	req.Body = http.MaxBytesReader(out, req.Body, configstore.MaxBytes)
	defer req.Body.Close()
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		http.Error(out, "invalid request", http.StatusBadRequest)
		return false
	}
	var tail any
	if err := decoder.Decode(&tail); !errors.Is(err, io.EOF) {
		http.Error(out, "unexpected trailing content", http.StatusBadRequest)
		return false
	}
	return true
}

func writeSavedJSON(out http.ResponseWriter, status int, response savedResponse) {
	out.Header().Set("Content-Type", "application/json; charset=utf-8")
	out.WriteHeader(status)
	_ = json.NewEncoder(out).Encode(response)
}
