package model

// UploadRequest is the expected multipart form fields for /api/v1/media/upload.
type UploadRequest struct {
	OrgSlug           string `json:"orgSlug"`
	GenerateVariants  bool   `json:"generateVariants"`
	Async             bool   `json:"async"`
	Alt               string `json:"alt,omitempty"`
	Caption           string `json:"caption,omitempty"`
	Description       string `json:"description,omitempty"`
}

// UploadResponse is returned after a successful upload.
type UploadResponse struct {
	Success   bool       `json:"success"`
	MediaFile *MediaFile `json:"mediaFile,omitempty"`
	JobID     string     `json:"jobId,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// PresignedUploadRequest is the JSON body for presigned upload.
type PresignedUploadRequest struct {
	OrgSlug  string `json:"orgSlug"`
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
}

// PresignedUploadResponse returns the pre-signed upload URL.
type PresignedUploadResponse struct {
	UploadURL string `json:"uploadUrl"`
	ObjectKey string `json:"objectKey"`
}

// CropRequest defines the crop area.
type CropRequest struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// CropResponse is returned after cropping.
type CropResponse struct {
	Success   bool       `json:"success"`
	MediaFile *MediaFile `json:"mediaFile,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// VariantRegenerateRequest triggers variant regeneration.
type VariantRegenerateRequest struct {
	Variants []string `json:"variants"`
}

// VariantRegenerateResponse returns the job ID.
type VariantRegenerateResponse struct {
	JobID string `json:"jobId"`
}

// DeleteResponse is returned after deleting a file.
type DeleteResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// PresignedDownloadResponse returns a pre-signed download URL.
type PresignedDownloadResponse struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expiresAt"`
}

// ErrorResponse is a generic error response.
type ErrorResponse struct {
	Error string `json:"error"`
}
