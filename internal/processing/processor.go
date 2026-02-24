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
		Quality: variant.Quality,
		Type:    bimgType,
	}

	if variant.Fit == FitCover {
		opts.Width = variant.Width
		opts.Height = variant.Height
		opts.Crop = true
		opts.Gravity = bimg.GravitySmart
	} else {
		// FitInside: scale down to fit within bounds while preserving aspect ratio
		srcSize, err := img.Size()
		if err != nil {
			return nil, fmt.Errorf("failed to get source size: %w", err)
		}
		w, h := fitInside(srcSize.Width, srcSize.Height, variant.Width, variant.Height)
		opts.Width = w
		opts.Height = h
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

// CropOptions configures the crop operation.
type CropOptions struct {
	X       int
	Y       int
	Width   int
	Height  int
	Rotate  float64 // degrees: 0, 90, 180, 270
	Scale   float64 // 1.0 = no scale
	Quality int     // 1-100, 0 means default (90)
	Format  string  // "webp", "jpeg", "png" — empty means "webp"
}

// CropImage crops the input image with optional rotation, scaling, and format.
func (p *Processor) CropImage(input []byte, opts CropOptions) (*ProcessResult, error) {
	img := bimg.NewImage(input)

	// Determine output format
	outFormat := FormatWebP
	switch opts.Format {
	case "jpeg", "jpg":
		outFormat = FormatJPEG
	case "png":
		outFormat = FormatPNG
	}

	quality := opts.Quality
	if quality <= 0 {
		quality = 90
	}

	processOpts := bimg.Options{
		AreaWidth:  opts.Width,
		AreaHeight: opts.Height,
		Left:       opts.X,
		Top:        opts.Y,
		Type:       toBimgType(outFormat),
		Quality:    quality,
	}

	// Apply rotation
	if opts.Rotate != 0 {
		processOpts.Rotate = bimg.Angle(int(opts.Rotate))
	}

	output, err := img.Process(processOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to crop image: %w", err)
	}

	// Apply scaling if needed
	if opts.Scale > 0 && opts.Scale != 1.0 {
		scaledWidth := int(float64(opts.Width) * opts.Scale)
		scaledHeight := int(float64(opts.Height) * opts.Scale)
		output, err = bimg.NewImage(output).Process(bimg.Options{
			Width:   scaledWidth,
			Height:  scaledHeight,
			Type:    toBimgType(outFormat),
			Quality: quality,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to scale cropped image: %w", err)
		}
	}

	size, err := bimg.NewImage(output).Size()
	if err != nil {
		return nil, fmt.Errorf("failed to get crop output size: %w", err)
	}

	return &ProcessResult{
		Data:     output,
		Width:    size.Width,
		Height:   size.Height,
		MimeType: outFormat.MimeType(),
		Format:   outFormat,
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

// fitInside calculates dimensions that fit within maxW x maxH while preserving aspect ratio.
func fitInside(srcW, srcH, maxW, maxH int) (int, int) {
	if srcW <= maxW && srcH <= maxH {
		return srcW, srcH
	}
	ratioW := float64(maxW) / float64(srcW)
	ratioH := float64(maxH) / float64(srcH)
	ratio := ratioW
	if ratioH < ratioW {
		ratio = ratioH
	}
	return int(float64(srcW) * ratio), int(float64(srcH) * ratio)
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
