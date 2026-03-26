package ai

import "context"

// ImageGenerator generates images from text prompts.
type ImageGenerator interface {
	GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error)
}

// ImageRequest describes an image generation request.
type ImageRequest struct {
	Prompt  string // text prompt for image generation
	Size    string // "1024x1024", "1024x1536", "1536x1024"
	Quality string // "low", "medium", "high"
}

// ImageResult holds the generated image data.
type ImageResult struct {
	ImageData []byte // raw image bytes (PNG or JPEG)
	Format    string // "png" or "jpeg"
}
