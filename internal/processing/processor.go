package processing

import (
	"fmt"

	"github.com/h2non/bimg"
)

// Processor handles image resize, crop, and format conversion using libvips via bimg.
type Processor struct{}

// NewProcessor creates a new image processor.
func NewProcessor() *Processor {
	return &Processor{}
}

// ProcessResult holds the output of an image operation.
type ProcessResult struct {
	Data     []byte
	Width    int
	Height   int
	MimeType string
	Format   Format
}

// ProcessImage resizes and converts an image according to a variant definition.
func (p *Processor) ProcessImage(input []byte, variant ImageVariant) (*ProcessResult, error) {
	img := bimg.NewImage(input)

	bimgType := toBimgType(variant.Format)

	opts := bimg.Options{
		Width:   variant.Width,
		Height:  variant.Height,
		Quality: variant.Quality,
		Type:    bimgType,
	}

	if variant.Fit == FitCover {
		opts.Crop = true
		opts.Gravity = bimg.GravitySmart
	}

	output, err := img.Process(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to process image for variant %q: %w", variant.Name, err)
	}

	size, err := bimg.NewImage(output).Size()
	if err != nil {
		return nil, fmt.Errorf("failed to get output size: %w", err)
	}

	return &ProcessResult{
		Data:     output,
		Width:    size.Width,
		Height:   size.Height,
		MimeType: variant.Format.MimeType(),
		Format:   variant.Format,
	}, nil
}

// CropImage crops the input image to the specified rectangle.
func (p *Processor) CropImage(input []byte, x, y, width, height int) (*ProcessResult, error) {
	img := bimg.NewImage(input)

	// First extract the crop area
	output, err := img.Process(bimg.Options{
		AreaWidth:  width,
		AreaHeight: height,
		Left:       x,
		Top:        y,
		Type:       bimg.WEBP,
		Quality:    90,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to crop image: %w", err)
	}

	size, err := bimg.NewImage(output).Size()
	if err != nil {
		return nil, fmt.Errorf("failed to get crop output size: %w", err)
	}

	return &ProcessResult{
		Data:     output,
		Width:    size.Width,
		Height:   size.Height,
		MimeType: "image/webp",
		Format:   FormatWebP,
	}, nil
}

// GetMetadata extracts image metadata (dimensions, format, etc.).
func (p *Processor) GetMetadata(input []byte) (*ImageMetadata, error) {
	img := bimg.NewImage(input)

	size, err := img.Size()
	if err != nil {
		return nil, fmt.Errorf("failed to get image size: %w", err)
	}

	meta, err := img.Metadata()
	if err != nil {
		return nil, fmt.Errorf("failed to get image metadata: %w", err)
	}

	return &ImageMetadata{
		Width:       size.Width,
		Height:      size.Height,
		Format:      meta.Type,
		Space:       meta.Space,
		Channels:    meta.Channels,
		Orientation: meta.Orientation,
	}, nil
}

// IsImageMimeType checks if the given MIME type is an image that can be processed.
func IsImageMimeType(mimeType string) bool {
	switch mimeType {
	case "image/jpeg", "image/png", "image/webp", "image/avif", "image/gif", "image/tiff":
		return true
	default:
		return false
	}
}

func toBimgType(f Format) bimg.ImageType {
	switch f {
	case FormatWebP:
		return bimg.WEBP
	case FormatJPEG:
		return bimg.JPEG
	case FormatPNG:
		return bimg.PNG
	case FormatAVIF:
		return bimg.AVIF
	default:
		return bimg.WEBP
	}
}
