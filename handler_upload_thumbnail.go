package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory int64 = 10 << 20
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, 400, "Parse multipart failed", err)
		return
	}

	file, fileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, 400, "Failed to get form file", err)
		return
	}
	mediaType := fileHeader.Header.Get("Content-Type")
	extensions, err := mime.ExtensionsByType(mediaType)
	if err != nil || len(extensions) == 0 {
		respondWithError(w, 500, "failed to extract extension", err)
		return
	}

	dbVideo, err := cfg.db.GetVideo(videoID)
	if err != nil || dbVideo.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	thumbailPath := filepath.Join(cfg.assetsRoot, videoID.String()+extensions[0])
	thumbnailFile, err := os.Create(thumbailPath)
	if err != nil {
		respondWithError(w, 500, "Failed to create file", err)
		return
	}
	_, err = io.Copy(thumbnailFile, file)
	if err != nil {
		respondWithError(w, 500, "Failed to copy contents", err)
		return
	}
	thumbnailUrl := fmt.Sprintf("http://localhost:%v/assets/%v.%v", cfg.port, videoID, extensions[0])
	dbVideo.ThumbnailURL = &thumbnailUrl
	err = cfg.db.UpdateVideo(dbVideo)
	if err != nil {
		respondWithError(w, 500, "Failed to save thumbnail", err)
		return
	}

	respondWithJSON(w, http.StatusOK, dbVideo)
}
