package storage

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// sanitizeRe collapses multiple hyphens into one.
var sanitizeRe = regexp.MustCompile(`-+`)

// GenerateFileFolderName sanitizes an original filename into a folder name.
// Matches the TypeScript: lowercases, keeps [a-z0-9-_], collapses hyphens.
//
//	"My Photo (2).jpg" → "my-photo-2"
func GenerateFileFolderName(originalFilename string) string {
	// Remove file extension
	ext := filepath.Ext(originalFilename)
	name := strings.TrimSuffix(originalFilename, ext)

	// Lowercase
	name = strings.ToLower(name)

	// Replace anything that isn't a-z, 0-9, hyphen, or underscore with a hyphen
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	name = b.String()

	// Collapse multiple hyphens into one
	name = sanitizeRe.ReplaceAllString(name, "-")

	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")

	if name == "" {
		name = "file"
	}
	return name
}

// GenerateTimestampedFolderName creates a folder name with a timestamp suffix.
// Only used when uniqueness per-upload is needed (e.g. chunked uploads).
func GenerateTimestampedFolderName(originalFilename string) string {
	base := GenerateFileFolderName(originalFilename)
	ts := time.Now().UnixMilli()
	return fmt.Sprintf("%s-%d", base, ts)
}

// GenerateFileFolderPath builds the folder path: {orgSlug}/{folderName}
func GenerateFileFolderPath(orgSlug, originalFilename string) string {
	folderName := GenerateFileFolderName(originalFilename)
	return fmt.Sprintf("%s/%s", orgSlug, folderName)
}

// GenerateObjectKey builds the full S3 object key:
//
//	{orgSlug}/{folderName}/{variant}.{ext}
//
// Example: travel-agency/profile-image/original.jpg
func GenerateObjectKey(orgSlug, originalFilename, variantName, fileExtension string) string {
	folderPath := GenerateFileFolderPath(orgSlug, originalFilename)
	ext := strings.TrimPrefix(fileExtension, ".")
	return fmt.Sprintf("%s/%s.%s", folderPath, variantName, ext)
}

// GenerateVariantKey builds the S3 key for a variant given an existing folder path.
//
//	{folderPath}/{variant}.{ext}
func GenerateVariantKey(folderPath, variantName, fileExtension string) string {
	ext := strings.TrimPrefix(fileExtension, ".")
	return fmt.Sprintf("%s/%s.%s", folderPath, variantName, ext)
}

// ExtractFolderPath returns the directory portion of an object key.
//
//	"org/folder/file.ext" → "org/folder"
func ExtractFolderPath(objectKey string) string {
	idx := strings.LastIndex(objectKey, "/")
	if idx < 0 {
		return ""
	}
	return objectKey[:idx]
}

// IsNewFolderStructure checks if an object key follows the
// new 3-segment structure: orgSlug/folderName/variant.ext
func IsNewFolderStructure(objectKey string) bool {
	segments := strings.Split(objectKey, "/")
	if len(segments) != 3 {
		return false
	}
	filename := segments[2]
	return strings.Contains(filename, ".") &&
		(strings.HasPrefix(filename, "original.") || !strings.Contains(filename, "-"))
}

// FolderStructureInfo holds parsed information from an object key.
type FolderStructureInfo struct {
	OrganizationSlug string `json:"organizationSlug"`
	FolderName       string `json:"folderName"`
	Filename         string `json:"filename"`
	IsLegacy         bool   `json:"isLegacy"`
}

// ParseFolderStructure extracts folder structure info from an object key.
func ParseFolderStructure(objectKey string) FolderStructureInfo {
	segments := strings.Split(objectKey, "/")

	if len(segments) >= 3 && IsNewFolderStructure(objectKey) {
		return FolderStructureInfo{
			OrganizationSlug: segments[0],
			FolderName:       segments[1],
			Filename:         strings.Join(segments[2:], "/"),
			IsLegacy:         false,
		}
	}

	// Legacy structure
	filename := ""
	if len(segments) > 0 {
		filename = segments[len(segments)-1]
	}
	orgSlug := "unknown"
	folderName := ""
	if len(segments) > 1 {
		orgSlug = segments[0]
		folderName = strings.Join(segments[:len(segments)-1], "/")
	}

	return FolderStructureInfo{
		OrganizationSlug: orgSlug,
		FolderName:       folderName,
		Filename:         filename,
		IsLegacy:         true,
	}
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
