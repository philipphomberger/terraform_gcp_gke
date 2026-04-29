package sharing

import (
	"encoding/json"
	"net/http"

	"github.com/philipphomberger/hombox/internal/auth"
	"github.com/philipphomberger/hombox/internal/files"
)

// Handler holds HTTP handlers for share endpoints.
type Handler struct {
	svc       *Service
	fileSvc   *files.Service
	fileStore *files.Storage
}

// NewHandler creates a new sharing handler.
func NewHandler(svc *Service, fileSvc *files.Service, fileStore *files.Storage) *Handler {
	return &Handler{svc: svc, fileSvc: fileSvc, fileStore: fileStore}
}

// Create handles POST /api/shares.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)

	var req CreateShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Permissions.Read == false && req.Permissions.Write == false {
		req.Permissions.Read = true // default
	}

	share, err := h.svc.Create(r.Context(), userID, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, share)
}

// List handles GET /api/shares.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)

	shares, err := h.svc.ListByOwner(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, shares)
}

// Get handles GET /api/shares/{token}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	share, err := h.svc.GetByToken(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	// List files in the shared folder
	parentID := r.URL.Query().Get("parent_id")
	var pid *string
	if parentID != "" {
		pid = &parentID
	}

	files, err := h.svc.GetSharedFile(r.Context(), token, pid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"share": share,
		"files": files,
	})
}

// Download handles GET /api/shares/{token}/files/{id}/download.
// Allows anonymous download of files in a shared folder.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	fileID := r.PathValue("id")

	share, err := h.svc.GetByToken(r.Context(), token)
	if err != nil || !share.Permissions.Read {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
		return
	}

	// Use fileSvc to get download URL (verifies ownership internally)
	url, _, err := h.fileSvc.GetDownloadURL(r.Context(), share.OwnerID, fileID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

// AnonymousUpload handles POST /api/shares/{token}/upload.
// Allows anonymous upload to shared folders.
func (h *Handler) AnonymousUpload(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	share, err := h.svc.GetByToken(r.Context(), token)
	if err != nil || !share.Permissions.Write {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "upload not allowed"})
		return
	}

	var req struct {
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		MimeType string `json:"mime_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Create file under the share owner's account, in the shared folder
	parentID := share.FileID
	file, signedURL, err := h.fileSvc.InitiateUpload(r.Context(), share.OwnerID, &parentID, req.Name, req.Size, req.MimeType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"file_id":    file.ID,
		"signed_url": signedURL,
	})
}

// AnonymousConfirmUpload handles POST /api/shares/{token}/upload/complete.
func (h *Handler) AnonymousConfirmUpload(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	share, err := h.svc.GetByToken(r.Context(), token)
	if err != nil || !share.Permissions.Write {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "upload not allowed"})
		return
	}

	var req struct {
		FileID   string `json:"file_id"`
		Checksum string `json:"checksum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	file, err := h.fileSvc.ConfirmUpload(r.Context(), share.OwnerID, req.FileID, req.Checksum)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, file)
}

// Delete handles DELETE /api/shares/{id}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)
	shareID := r.PathValue("id")

	if err := h.svc.Delete(r.Context(), userID, shareID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
