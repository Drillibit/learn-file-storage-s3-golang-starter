package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

var videoFolderByAspectRatio = map[string]string{
	"16:9":   "landscape",
	"9:16":   "portrait",
	"custom": "other",
}

const videoMimeType = "video/mp4"

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)
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

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video ID", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't get video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You can't upload this video", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get the video file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse media type", err)
		return
	}

	if mediaType != videoMimeType {
		respondWithError(w, http.StatusUnsupportedMediaType, "Unsupported media type", nil)
		return
	}

	exts, err := mime.ExtensionsByType(mediaType)
	if err != nil || len(exts) == 0 {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get extension", err)
		return
	}
	videoExt := exts[0]
	if mediaType == videoMimeType {
		videoExt = ".mp4"
	}

	tmpFile, err := os.CreateTemp("", "video-tmp.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}

	if _, err := io.Copy(tmpFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't copy file", err)
		return
	}

	defer tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	tmpFile.Seek(0, io.SeekStart)
	aspectRatio, err := getVideoAspectRatio(tmpFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video aspect ratio", err)
		return
	}
	processedVideoPath, err := processVideoForFastStart(tmpFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't process video", err)
		return
	}
	defer os.Remove(processedVideoPath)

	processedVideoFile, err := os.Open(processedVideoPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't open processed video file", err)
		return
	}
	defer processedVideoFile.Close()

	folder, ok := videoFolderByAspectRatio[aspectRatio]
	if !ok {
		folder = "other"
	}

	videoKeyID, err := generateKey()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate file ID", err)
		return
	}

	videoKey := fmt.Sprintf("%s/%s%s", folder, videoKeyID, videoExt)

	if _, err := cfg.s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &videoKey,
		Body:        processedVideoFile,
		ContentType: &mediaType,
	}); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload video", err)
		return
	}
	url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, videoKey)
	video.VideoURL = &url

	videoURL := fmt.Sprintf("%s,%s", cfg.s3Bucket, videoKey)
	video.VideoURL = &videoURL

	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}
	presignedVideo, err := cfg.dbVideoToSignedVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get presigned video", err)
		return
	}

	respondWithJSON(w, http.StatusCreated, presignedVideo)
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	client := s3.NewPresignClient(s3Client)

	presignedURL, err := client.PresignGetObject(context.Background(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}

	return presignedURL.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	if video.VideoURL == nil {
		return video, nil
	}
	videoURL := strings.Split(*video.VideoURL, ",")
	if len(videoURL) != 2 {
		return video, errors.New("invalid video URL")
	}

	bucket := videoURL[0]
	key := videoURL[1]

	presignedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, 1*time.Hour)
	if err != nil {
		return video, err
	}

	video.VideoURL = &presignedURL
	return video, nil
}
