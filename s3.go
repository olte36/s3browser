package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const previewBytes = 256 * 1024

type s3Service interface {
	ListBuckets(ctx context.Context) ([]bucketItem, error)
	ListObjects(ctx context.Context, bucket string) ([]objectItem, error)
	InspectObject(ctx context.Context, bucket, key string, limit int64) (objectDetail, error)
}

type bucketItem struct {
	Name         string
	CreationDate time.Time
	Region       string
}

type objectItem struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
	ContentType  string
}

type objectDetail struct {
	Object     objectItem
	Metadata   map[string]string
	Preview    string
	Binary     bool
	Truncated  bool
	PreviewLen int64
}

type minioService struct {
	client *minio.Client
}

func newMinioService(cfg endpointConfig, creds *credentials.Credentials) (*minioService, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:        creds,
		Secure:       cfg.Secure,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, err
	}
	client.SetAppInfo("s3-objects-browser", "0.1.0")
	return &minioService{client: client}, nil
}

func (s *minioService) ListBuckets(ctx context.Context) ([]bucketItem, error) {
	buckets, err := s.client.ListBuckets(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]bucketItem, 0, len(buckets))
	for _, bucket := range buckets {
		items = append(items, bucketItem{
			Name:         bucket.Name,
			CreationDate: bucket.CreationDate,
			Region:       bucket.BucketRegion,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (s *minioService) ListObjects(ctx context.Context, bucket string) ([]objectItem, error) {
	objectCh := s.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true})
	var objects []objectItem
	for obj := range objectCh {
		if obj.Err != nil {
			return nil, obj.Err
		}
		objects = append(objects, objectItem{
			Key:          obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
			ETag:         obj.ETag,
			ContentType:  obj.ContentType,
		})
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return objects, nil
}

func (s *minioService) InspectObject(ctx context.Context, bucket, key string, limit int64) (objectDetail, error) {
	stat, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return objectDetail{}, err
	}

	object, err := s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return objectDetail{}, err
	}
	defer object.Close()

	readLimit := limit
	if readLimit <= 0 {
		readLimit = previewBytes
	}
	buf, err := io.ReadAll(io.LimitReader(object, readLimit+1))
	if err != nil {
		return objectDetail{}, err
	}

	truncated := int64(len(buf)) > readLimit
	if truncated {
		buf = buf[:readLimit]
	}

	preview, binary := renderPreview(buf)

	return objectDetail{
		Object: objectItem{
			Key:          stat.Key,
			Size:         stat.Size,
			LastModified: stat.LastModified,
			ETag:         stat.ETag,
			ContentType:  stat.ContentType,
		},
		Metadata:   flattenMetadata(stat.Metadata),
		Preview:    preview,
		Binary:     binary,
		Truncated:  truncated || stat.Size > readLimit,
		PreviewLen: int64(len(buf)),
	}, nil
}

func renderPreview(buf []byte) (string, bool) {
	if len(buf) == 0 {
		return "", false
	}
	if isBinaryPreview(buf) {
		return hex.Dump(buf), true
	}
	return sanitizeTerminalText(string(buf)), false
}

func isBinaryPreview(buf []byte) bool {
	if !utf8.Valid(buf) {
		return true
	}

	controlBytes := 0
	for _, b := range buf {
		switch {
		case b == 0:
			return true
		case b == '\n' || b == '\r' || b == '\t':
			continue
		case b < 32 || b == 127:
			controlBytes++
		}
	}
	return controlBytes > 0 && controlBytes*100/len(buf) > 20
}

func sanitizeTerminalText(text string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return r
		}
		if r < 32 || r == 127 {
			return '.'
		}
		return r
	}, text)
}

func flattenMetadata(metadata map[string][]string) map[string]string {
	out := make(map[string]string, len(metadata))
	for key, values := range metadata {
		if len(values) == 0 {
			out[key] = ""
			continue
		}
		out[key] = values[0]
	}
	return out
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}
