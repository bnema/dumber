package port

import "context"

type ConvertedFavicon struct {
	PNG      []byte
	SizedPNG map[int][]byte
}

type FaviconImageConverter interface {
	Convert(ctx context.Context, original []byte, contentType string, sizes []int) (*ConvertedFavicon, error)
}
