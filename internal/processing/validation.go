package processing

// AllowedMimeTypes is the whitelist of MIME types accepted for upload.
// Matches the TS enhanced-upload validation.
var AllowedMimeTypes = map[string]bool{
	// Images
	"image/jpeg":    true,
	"image/png":     true,
	"image/gif":     true,
	"image/webp":    true,
	"image/avif":    true,
	"image/svg+xml": true,
	"image/tiff":    true,
	"image/bmp":     true,

	// Videos
	"video/mp4":       true,
	"video/webm":      true,
	"video/quicktime": true,
	"video/x-msvideo": true,

	// Audio
	"audio/mpeg": true,
	"audio/wav":  true,
	"audio/ogg":  true,
	"audio/webm": true,

	// Documents
	"application/pdf": true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.ms-excel": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
}

// IsAllowedMimeType checks if a MIME type is in the upload whitelist.
func IsAllowedMimeType(mimeType string) bool {
	return AllowedMimeTypes[mimeType]
}
