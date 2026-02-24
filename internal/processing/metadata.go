package processing

// ImageMetadata holds extracted image info.
type ImageMetadata struct {
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Format      string `json:"format"`
	Space       string `json:"space"`
	Channels    int    `json:"channels"`
	Orientation int    `json:"orientation"`
}
