package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os/exec"
)

type ffprobeOutput struct {
	Streams []struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"streams"`
}

func getVideoAspectRatio(filePath string) (string, error) {
	var stdout bytes.Buffer
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	var meta ffprobeOutput
	err = json.Unmarshal(stdout.Bytes(), &meta)
	if err != nil {
		return "", err
	}

	targets := []struct {
		name  string
		value float64
	}{
		{"16:9", 16.0 / 9.0},
		{"4:3", 4.0 / 3.0},
		{"1:1", 1.0},
		{"9:16", 9.0 / 16.0},
	}

	actual := float64(meta.Streams[0].Width) / float64(meta.Streams[0].Height)
	tol := 0.03

	bestName := "custom"
	bestDiff := math.MaxFloat64

	for _, t := range targets {
		diff := math.Abs(actual - t.value)
		if diff < bestDiff {
			bestDiff = diff
			bestName = t.name
		}
	}

	if bestDiff <= tol {
		return bestName, nil
	}
	return "custom", nil
}
