package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"yuon/configuration"
)

// S3Client implements FileStorage backed by an S3-compatible service.
type S3Client struct {
	bucket   string
	baseURL  string
	uploader *manager.Uploader
	client   *s3.Client
}

func NewS3Client(cfg *configuration.StorageConfig) (*S3Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("storage config is nil")
	}

	cred := credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")
	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if cfg.Endpoint != "" {
			return aws.Endpoint{
				URL:               cfg.Endpoint,
				SigningRegion:     cfg.Region,
				HostnameImmutable: true,
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	awsCfg, err := awscfg.LoadDefaultConfig(context.Background(),
		awscfg.WithRegion(cfg.Region),
		awscfg.WithCredentialsProvider(cred),
		awscfg.WithEndpointResolverWithOptions(resolver),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.UsePath
	})

	uploader := manager.NewUploader(s3Client, func(u *manager.Uploader) {
		u.PartSize = 10 * 1024 * 1024
		u.Concurrency = 2
	})

	return &S3Client{
		bucket:   cfg.Bucket,
		baseURL:  strings.TrimRight(cfg.BaseURL, "/"),
		uploader: uploader,
		client:   s3Client,
	}, nil
}

func (c *S3Client) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if c.bucket == "" {
		return "", fmt.Errorf("bucket is not configured")
	}

	input := &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
		ACL:         types.ObjectCannedACLPrivate,
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if _, err := c.uploader.Upload(ctx, input); err != nil {
		return "", fmt.Errorf("s3 upload failed: %w", err)
	}

	if c.baseURL != "" {
		return fmt.Sprintf("%s/%s", c.baseURL, key), nil
	}
	return key, nil
}

func (c *S3Client) Download(ctx context.Context, key string) ([]byte, string, error) {
	if c.bucket == "" {
		return nil, "", fmt.Errorf("bucket is not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", fmt.Errorf("s3 download failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("s3 read failed: %w", err)
	}

	contentType := "application/octet-stream"
	if resp.ContentType != nil && *resp.ContentType != "" {
		contentType = *resp.ContentType
	}

	return body, contentType, nil
}
