package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const maxLimit int64 = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxLimit)

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
	dbVideo, err := cfg.db.GetVideo(videoID)

	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't find the video", err)
		return
	}

	if dbVideo.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not the owner of the video", errors.New("Expected owner"))
		return
	}
	multipartFile, multipartHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to form file", err)
		return
	}
	defer multipartFile.Close()
	mediatype, _, err := mime.ParseMediaType(multipartHeader.Header.Get("content-type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Not a video", err)
		return
	}

	if mediatype != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Not a video", errors.New("expected video/mp4"))
		return
	}

	file, err := os.CreateTemp("", "tubely-upload*.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create temp", err)
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	_, err = io.Copy(file, multipartFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to copy files", err)
		return
	}
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to seek start", err)
		return
	}

	key := make([]byte, 32)
	_, err = rand.Read(key)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create key", err)
		return
	}
	aspectRatio, err := getVideoAspectRatio(file.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get aspect ratio", err)
		return
	}

	prefix := "other/"
	switch aspectRatio {
	case "16:9":
		prefix = "landscape/"
	case "9:16":
		prefix = "portrait/"
	}
	keyString := prefix + base64.RawURLEncoding.EncodeToString(key) + ".mp4"
	itemInput := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &keyString,
		Body:        file,
		ContentType: &mediatype,
	}
	_, err = cfg.s3Client.PutObject(r.Context(), &itemInput)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to put object", err)
		return
	}
	videoURL := fmt.Sprintf("https://%v.s3.%v.amazonaws.com/%v", cfg.s3Bucket, cfg.s3Region, keyString)
	dbVideo.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(dbVideo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to update video url", err)
		return
	}
	respondWithJSON(w, http.StatusOK, dbVideo)
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(&out)
	videoStreams := VideoStreams{}
	err = decoder.Decode(&videoStreams)
	if err != nil {
		return "", err
	}
	if len(videoStreams.Streams) == 0 {
		return "", errors.New("empty streams")
	}
	width := 0
	height := 0
	for _, stream := range videoStreams.Streams {
		if stream.CodecType == "video" {
			width = stream.Width
			height = stream.Height
			break
		}
	}

	if width == 0 || height == 0 {
		return "", errors.New("invalid aspect ratio or not a video")
	}

	aspectRatio := float64(width) / float64(height)

	if math.Abs(aspectRatio-16.0/9.0) < 0.05 {
		return "16:9", nil
	} else if math.Abs(aspectRatio-9.0/16.0) < 0.05 {
		return "9:16", nil
	} else {
		return "other", nil
	}

}

type VideoStreams struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width,omitempty"`
		Height    int    `json:"height,omitempty"`
	} `json:"streams"`
}
