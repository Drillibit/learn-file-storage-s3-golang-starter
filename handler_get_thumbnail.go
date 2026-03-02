package main

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

func getVideoFromPath(r *http.Request) (uuid.UUID, error) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("invalid video ID: %w", err)
	}
	return videoID, nil
}

func (cfg *apiConfig) handlerThumbnailGet(w http.ResponseWriter, r *http.Request) {
	videoID, err := getVideoFromPath(r)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video ID", err)
		return
	}

	tn, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Thumbnail not found", nil)
		return
	}

	w.Header().Set("Content-Type", *tn.VideoURL)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(*tn.VideoURL)))

	_, err = w.Write([]byte(*tn.VideoURL))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error writing response", err)
		return
	}
}
