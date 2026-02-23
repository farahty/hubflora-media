package storage

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// sanitizeFilename converts a filename to a URL-safe folder name.
// Lowercases, replaces special chars with hyphens, collapses multiple hyphens.
func sanitizeFilename(filename string) string {
	// Remove extension
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)

	// Lowercase
	name = strings.ToLower(name)

	// Replace non-alphanumeric chars with hyphens
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	name = b.String()

	// Collapse multiple hyphens
	re := regexp.MustCompile(`-{2,}`)
	name = re.ReplaceAllString(name, "-")

	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")

	if name == "" {
		name = "file"
	}
	return name
}

// GenerateTimestampedFolderName creates a folder name from the original filename plus a timestamp.
func GenerateTimestampedFolderName(originalFilename string) string {
	base := sanitizeFilename(originalFilename)
	ts := time.Now().UnixMilli()
	return fmt.Sprintf("%s-%d", base, ts)
}

// GenerateObjectKey builds the full S3 object key:
//
//	{orgSlug}/{folderName}/{variant}.{ext}
func GenerateObjectKey(orgSlug, originalFilename, variantName, fileExtension string) string {
	folderName := GenerateTimestampedFolderName(originalFilename)
	ext := strings.TrimPrefix(fileExtension, ".")
	return fmt.Sprintf("%s/%s/%s.%s", orgSlug, folderName, variantName, ext)
}

// GenerateVariantKey builds the S3 key for a variant given the folder path.
//
//	{folderPath}/{variant}.{ext}
func GenerateVariantKey(folderPath, variantName, fileExtension string) string {
	ext := strings.TrimPrefix(fileExtension, ".")
	return fmt.Sprintf("%s/%s.%s", folderPath, variantName, ext)
}

// ExtractFolderPath returns the directory portion of an object key.
func ExtractFolderPath(objectKey string) string {
	idx := strings.LastIndex(objectKey, "/")
	if idx < 0 {
		return ""
	}
	return objectKey[:idx]
}

// FileExtensionFromMimeType maps common MIME types to file extensions.
func FileExtensionFromMimeType(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	case "image/avif":
		return "avif"
	case "image/gif":
		return "gif"
	case "image/svg+xml":
		return "svg"
	case "video/mp4":
		return "mp4"
	case "video/webm":
		return "webm"
	case "audio/mpeg":
		return "mp3"
	case "audio/wav":
		return "wav"
	case "application/pdf":
		return "pdf"
	default:
		return "bin"
	}
}
