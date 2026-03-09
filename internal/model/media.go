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
	Duration         *int              `json:"duration,omitempty"`
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

// MediaFileRecord represents a row in the media_files table.
type MediaFileRecord struct {
	ID               string               `json:"id"`
	Filename         string               `json:"filename"`
	OriginalFilename string               `json:"originalFilename"`
	MimeType         string               `json:"mimeType"`
	FileSize         int64                `json:"fileSize"`
	Width            *int                 `json:"width,omitempty"`
	Height           *int                 `json:"height,omitempty"`
	Duration         *int                 `json:"duration,omitempty"`
	BucketName       string               `json:"bucketName"`
	ObjectKey        string               `json:"objectKey"`
	URL              string               `json:"url"`
	ThumbnailURL     *string              `json:"thumbnailUrl,omitempty"`
	Alt              *string              `json:"alt,omitempty"`
	Caption          *string              `json:"caption,omitempty"`
	Description      *string              `json:"description,omitempty"`
	Metadata         map[string]any       `json:"metadata,omitempty"`
	IsPrivate        bool                 `json:"isPrivate"`
	OrganizationID   *string              `json:"organizationId,omitempty"`
	UploadedBy       string               `json:"uploadedBy"`
	Variants         []MediaVariantRecord `json:"variants,omitempty"`
	CreatedAt        time.Time            `json:"createdAt"`
	UpdatedAt        time.Time            `json:"updatedAt"`
}

// MediaVariantRecord represents a row in the media_variants table.
type MediaVariantRecord struct {
	ID          string    `json:"id"`
	MediaFileID string    `json:"mediaFileId"`
	Variant     string    `json:"variant"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	FileSize    int64     `json:"fileSize"`
	ObjectKey   string    `json:"objectKey"`
	URL         string    `json:"url"`
	MimeType    string    `json:"mimeType"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ToMediaFile converts a DB record to the API response MediaFile.
func (r *MediaFileRecord) ToMediaFile() *MediaFile {
	mf := &MediaFile{
		ID:               r.ID,
		Filename:         r.Filename,
		OriginalFilename: r.OriginalFilename,
		MimeType:         r.MimeType,
		FileSize:         r.FileSize,
		Width:            r.Width,
		Height:           r.Height,
		Duration:         r.Duration,
		BucketName:       r.BucketName,
		ObjectKey:        r.ObjectKey,
		URL:              r.URL,
		ThumbnailURL:     r.ThumbnailURL,
		Metadata:         r.Metadata,
		CreatedAt:        r.CreatedAt,
	}
	for _, vr := range r.Variants {
		mf.Variants = append(mf.Variants, MediaVariant{
			Name:      vr.Variant,
			Width:     vr.Width,
			Height:    vr.Height,
			FileSize:  vr.FileSize,
			ObjectKey: vr.ObjectKey,
			URL:       vr.URL,
			MimeType:  vr.MimeType,
		})
	}
	return mf
}
