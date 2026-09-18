package main

import (
	"fmt"
	"io"
	"net/http"

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
	bytes, err := io.ReadAll(file)
	if err != nil {
		respondWithError(w, 400, "Failed to read file", err)
		return
	}
	dbVideo, err := cfg.db.GetVideo(videoID)
	if err != nil || dbVideo.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}
	videoThumbnails[videoID] = thumbnail{
		data:      bytes,
		mediaType: mediaType,
	}
	thumbnailUrl := fmt.Sprintf("http://localhost:8091/api/thumbnails/%v", videoID)
	dbVideo.ThumbnailURL = &thumbnailUrl
	err = cfg.db.UpdateVideo(dbVideo)
	if err != nil {
		respondWithError(w, 500, "Failed to save thumbnail", err)
		return
	}

	respondWithJSON(w, http.StatusOK, dbVideo)
}
