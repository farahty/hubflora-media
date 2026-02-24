package model

import "time"

// MediaType classifies the uploaded file.
type MediaType string

const (
	MediaTypeImage    MediaType = "IMAGE"
	MediaTypeVideo    MediaType = "VIDEO"
	MediaTypeAudio    MediaType = "AUDIO"
	MediaTypeDocument MediaType = "DOCUMENT"
	MediaTypeOther    MediaType = "OTHER"
)

// GetMediaType determines the MediaType from a MIME type string.
func GetMediaType(mimeType string) MediaType {
	switch {
	case len(mimeType) >= 6 && mimeType[:6] == "image/":
		return MediaTypeImage
	case len(mimeType) >= 6 && mimeType[:6] == "video/":
		return MediaTypeVideo
	case len(mimeType) >= 6 && mimeType[:6] == "audio/":
		return MediaTypeAudio
	case mimeType == "application/pdf" ||
		mimeType == "application/msword" ||
		mimeType == "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return MediaTypeDocument
	default:
		return MediaTypeOther
	}
}

// MediaFile represents the stored file metadata returned by the service.
type MediaFile struct {
	ID               string            `json:"id"`
	Filename         string            `json:"filename"`
	OriginalFilename string            `json:"originalFilename"`
	MimeType         string            `json:"mimeType"`
	FileSize         int64             `json:"fileSize"`
	Width            *int              `json:"width,omitempty"`
	Height           *int              `json:"height,omitempty"`
	BucketName       string            `json:"bucketName"`
	ObjectKey        string            `json:"objectKey"`
	URL              string            `json:"url"`
	ThumbnailURL     *string           `json:"thumbnailUrl,omitempty"`
	Metadata         map[string]any    `json:"metadata,omitempty"`
	Variants         []MediaVariant    `json:"variants,omitempty"`
	CreatedAt        time.Time         `json:"createdAt"`
}

// MediaVariant represents a generated variant (e.g. thumbnail, small).
type MediaVariant struct {
	Name      string `json:"name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	FileSize  int64  `json:"fileSize"`
	ObjectKey string `json:"objectKey"`
	URL       string `json:"url"`
	MimeType  string `json:"mimeType"`
}
