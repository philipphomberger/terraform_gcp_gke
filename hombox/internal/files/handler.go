package files

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/philipphomberger/hombox/internal/auth"
)

// Handler holds HTTP handlers for file endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a new file handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Service returns the underlying service.
func (h *Handler) Service() *Service {
	return h.svc
}

// InitiateUpload handles POST /api/files/upload.
func (h *Handler) InitiateUpload(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)

	var req struct {
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		MimeType string `json:"mime_type"`
		ParentID string `json:"parent_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Size <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and size are required"})
		return
	}

	var parentID *string
	if req.ParentID != "" {
		parentID = &req.ParentID
	}

	file, signedURL, err := h.svc.InitiateUpload(r.Context(), userID, parentID, req.Name, req.Size, req.MimeType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"file_id":    file.ID,
		"signed_url": signedURL,
		"file":       file,
	})
}

// ConfirmUpload handles POST /api/files/upload/complete.
func (h *Handler) ConfirmUpload(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)

	var req struct {
		FileID   string `json:"file_id"`
		Checksum string `json:"checksum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	file, err := h.svc.ConfirmUpload(r.Context(), userID, req.FileID, req.Checksum)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, file)
}

// Download handles GET /api/files/{id}/download.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)
	fileID := r.PathValue("id")

	url, _, err := h.svc.GetDownloadURL(r.Context(), userID, fileID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

// CreateFolder handles POST /api/files/folder.
func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)

	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	var parentID *string
	if req.ParentID != "" {
		parentID = &req.ParentID
	}

	file, err := h.svc.CreateFolder(r.Context(), userID, parentID, req.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, file)
}

// List handles GET /api/files.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)
	parentIDStr := r.URL.Query().Get("parent_id")

	var parentID *string
	if parentIDStr != "" {
		parentID = &parentIDStr
	}

	files, err := h.svc.List(r.Context(), userID, parentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, files)
}

// Rename handles PATCH /api/files/{id}/rename.
func (h *Handler) Rename(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)
	fileID := r.PathValue("id")

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	file, err := h.svc.Rename(r.Context(), userID, fileID, req.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, file)
}

// Move handles PATCH /api/files/{id}/move.
func (h *Handler) Move(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)
	fileID := r.PathValue("id")

	var req struct {
		ParentID string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	var newParentID *string
	if req.ParentID != "" {
		newParentID = &req.ParentID
	}

	file, err := h.svc.Move(r.Context(), userID, fileID, newParentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, file)
}

// Delete handles DELETE /api/files/{id}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)
	fileID := r.PathValue("id")

	if err := h.svc.Delete(r.Context(), userID, fileID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Search handles GET /api/files/search.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)
	query := r.URL.Query().Get("q")
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query parameter 'q' is required"})
		return
	}

	files, err := h.svc.Search(r.Context(), userID, query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, files)
}

// Preview handles GET /api/files/{id}/preview.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(auth.UserIDKey).(string)
	fileID := r.PathValue("id")

	wStr := r.URL.Query().Get("w")
	hStr := r.URL.Query().Get("h")
	width, _ := strconv.Atoi(wStr)
	height, _ := strconv.Atoi(hStr)
	if width <= 0 {
		width = 400
	}
	if height <= 0 {
		height = 300
	}

	url, err := h.svc.GeneratePreview(r.Context(), userID, fileID, width, height)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
