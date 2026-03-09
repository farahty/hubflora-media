package processing

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
)

// VideoMetadata holds extracted video info.
type VideoMetadata struct {
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Duration int    `json:"duration"` // seconds
	Codec    string `json:"codec"`
	Format   string `json:"format"`
}

// ffprobeOutput maps the JSON output from ffprobe.
type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type ffprobeFormat struct {
	Duration string `json:"duration"`
	Name     string `json:"format_name"`
}

// ExtractVideoMetadata uses ffprobe to extract video dimensions, duration, and codec.
func ExtractVideoMetadata(data []byte) (*VideoMetadata, error) {
	tmp, err := writeTempFile(data)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp)

	out, err := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		tmp,
	).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, fmt.Errorf("ffprobe parse failed: %w", err)
	}

	meta := &VideoMetadata{
		Format: probe.Format.Name,
	}

	// Parse duration
	if probe.Format.Duration != "" {
		if d, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
			meta.Duration = int(math.Round(d))
		}
	}

	// Find the first video stream for dimensions and codec
	for _, s := range probe.Streams {
		if s.CodecType == "video" {
			meta.Width = s.Width
			meta.Height = s.Height
			meta.Codec = s.CodecName
			break
		}
	}

	return meta, nil
}

// ExtractVideoThumbnail uses ffmpeg to extract a single frame as PNG bytes.
// Seeks to 1 second for a representative frame, falls back to first frame.
func ExtractVideoThumbnail(data []byte) ([]byte, error) {
	tmp, err := writeTempFile(data)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp)

	// Try at 1 second first
	out, err := exec.Command("ffmpeg",
		"-ss", "1",
		"-i", tmp,
		"-vframes", "1",
		"-f", "image2",
		"-c:v", "png",
		"pipe:1",
	).Output()
	if err == nil && len(out) > 0 {
		return out, nil
	}

	// Fallback: first frame
	out, err = exec.Command("ffmpeg",
		"-i", tmp,
		"-vframes", "1",
		"-f", "image2",
		"-c:v", "png",
		"pipe:1",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg thumbnail extraction failed: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ffmpeg produced empty thumbnail")
	}

	return out, nil
}

// writeTempFile writes data to a temporary file and returns its path.
func writeTempFile(data []byte) (string, error) {
	f, err := os.CreateTemp("", "hubflora-video-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}
	f.Close()
	return f.Name(), nil
}
