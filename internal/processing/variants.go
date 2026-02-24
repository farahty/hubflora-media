package processing

// ImageVariant defines a resize/format preset.
type ImageVariant struct {
	Name    string
	Width   int
	Height  int
	Quality int
	Format  Format
	Fit     Fit
}

type Format int

const (
	FormatWebP Format = iota
	FormatJPEG
	FormatPNG
	FormatAVIF
)

func (f Format) String() string {
	switch f {
	case FormatWebP:
		return "webp"
	case FormatJPEG:
		return "jpeg"
	case FormatPNG:
		return "png"
	case FormatAVIF:
		return "avif"
	default:
		return "webp"
	}
}

// MimeType returns the MIME type for a format.
func (f Format) MimeType() string {
	switch f {
	case FormatWebP:
		return "image/webp"
	case FormatJPEG:
		return "image/jpeg"
	case FormatPNG:
		return "image/png"
	case FormatAVIF:
		return "image/avif"
	default:
		return "image/webp"
	}
}

type Fit int

const (
	FitCover   Fit = iota // Crop to fill
	FitInside             // Scale down to fit within bounds
)

// DefaultVariants returns the standard set of image variants to generate.
var DefaultVariants = []ImageVariant{
	{Name: "thumbnail", Width: 400, Height: 400, Quality: 80, Format: FormatWebP, Fit: FitCover},
	{Name: "small", Width: 600, Height: 600, Quality: 85, Format: FormatWebP, Fit: FitInside},
	{Name: "medium", Width: 1024, Height: 1024, Quality: 85, Format: FormatWebP, Fit: FitInside},
	{Name: "large", Width: 1440, Height: 1440, Quality: 90, Format: FormatWebP, Fit: FitInside},
	{Name: "original_webp", Width: 2048, Height: 2048, Quality: 95, Format: FormatWebP, Fit: FitInside},
}

// FindVariant returns the named variant definition, or nil if not found.
func FindVariant(name string) *ImageVariant {
	for _, v := range DefaultVariants {
		if v.Name == name {
			return &v
		}
	}
	return nil
}
