package storage

import "context"

// FileStorage defines uploading interface.
type FileStorage interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) (string, error)
	Download(ctx context.Context, key string) ([]byte, string, error)
}
