package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

func getUserFromRequest(r *http.Request, cfg *apiConfig) (*database.User, error) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		return nil, fmt.Errorf("couldn't find token: %w", err)
	}

	user, err := cfg.db.GetUserByRefreshToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("couldn't get user: %w", err)
	}
	return user, nil
}

func (cfg *apiConfig) handlerRefresh(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Token string `json:"token"`
	}
	user, err := getUserFromRequest(r, cfg)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't get user", err)
		return
	}

	accessToken, err := auth.MakeJWT(
		user.ID,
		cfg.jwtSecret,
		time.Hour,
	)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate token", err)
		return
	}

	respondWithJSON(w, http.StatusOK, response{
		Token: accessToken,
	})
}

func (cfg *apiConfig) handlerRevoke(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find token", err)
		return
	}

	err = cfg.db.RevokeRefreshToken(refreshToken)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't revoke session", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
