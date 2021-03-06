package web

import (
	"net/http"
	"os"
	"strconv"

	"github.com/DataManager-Go/DataManagerServer/models"
	"github.com/gorilla/mux"
	"github.com/h2non/filetype"
)

// RawFileHandler handler for previews
func RawFileHandler(handlerData HandlerData, w http.ResponseWriter, r *http.Request) error {
	vars := mux.Vars(r)
	fileID := vars["fileID"]

	// Get requested file
	file, found, err := models.GetPublicFile(handlerData.Db, fileID)
	if !found {
		NotFoundHandler(handlerData, w, r)
		return nil
	}

	w.Header().Set("content-disposition", "attachment; filename=\""+file.GetPublicNameWithExtension()+"\"")
	w.Header().Set("Content-Length", strconv.FormatInt(file.FileSize, 10))

	// Send error
	if LogError(err) {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return nil
	}

	// Send not found if not public
	if !file.IsPublic {
		NotFoundHandler(handlerData, w, r)
		return nil
	}

	// Set content type header if available and valid
	if len(file.FileType) > 0 && filetype.IsMIMESupported(file.FileType) {
		setContentType(w, file.FileType)
	}

	// Open file
	f, err := os.Open(handlerData.Config.GetStorageFile(file.LocalName))
	defer f.Close()
	if LogError(err) {
		if os.IsNotExist(err) {
			NotFoundHandler(handlerData, w, r)
			return nil
		}

		http.Error(w, "Server error", http.StatusInternalServerError)
		return nil
	}

	serveFileStream(handlerData.Config, f, w)

	return nil
}
