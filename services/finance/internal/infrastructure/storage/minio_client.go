// Package storage provides MinIO/S3 storage operations for the finance service.
//
// Used by the worker to persist generated artifacts (Excel exports, etc.) to
// the shared `goapps-staging` bucket and by gRPC handlers to issue presigned
// download URLs back to the BFF.
package storage

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/zerolog/log"
)

// Config holds MinIO connection configuration.
type Config struct {
	Endpoint           string `mapstructure:"endpoint"`
	AccessKey          string `mapstructure:"access_key"`
	SecretKey          string `mapstructure:"secret_key"`
	Bucket             string `mapstructure:"bucket"`
	UseSSL             bool   `mapstructure:"use_ssl"`
	InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"`
	Region             string `mapstructure:"region"`
	PublicURL          string `mapstructure:"public_url"`
}

// Service is the storage interface used by the finance worker.
type Service interface {
	// PutObject uploads an object at the given key. contentType drives the stored Content-Type.
	PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	// PresignedGetURL returns a signed download URL valid for `validity`.
	PresignedGetURL(ctx context.Context, key string, validity time.Duration, downloadName string) (string, error)
	// RemoveObject deletes an object. No-op if it doesn't exist.
	RemoveObject(ctx context.Context, key string) error
	// Bucket returns the configured bucket name.
	Bucket() string
}

// MinIOClient is the production implementation of Service.
type MinIOClient struct {
	client    *minio.Client
	bucket    string
	publicURL string
}

// NewMinIOClient builds a configured MinIO client; bucket is NOT created here
// (the operator/init container is expected to provision it).
func NewMinIOClient(cfg Config) (*MinIOClient, error) {
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	}
	if cfg.UseSSL && cfg.InsecureSkipVerify {
		opts.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // operator-opted self-signed certs
		}
	}
	c, err := minio.New(cfg.Endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	log.Info().Str("endpoint", cfg.Endpoint).Str("bucket", cfg.Bucket).Bool("ssl", cfg.UseSSL).Msg("MinIO client initialized")
	return &MinIOClient{client: c, bucket: cfg.Bucket, publicURL: cfg.PublicURL}, nil
}

// PutObject implements Service.
func (m *MinIOClient) PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	if _, err := m.client.PutObject(ctx, m.bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}

// PresignedGetURL implements Service.
func (m *MinIOClient) PresignedGetURL(ctx context.Context, key string, validity time.Duration, downloadName string) (string, error) {
	reqParams := url.Values{}
	if downloadName != "" {
		reqParams.Set("response-content-disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeDownloadName(downloadName)))
	}
	u, err := m.client.PresignedGetObject(ctx, m.bucket, key, validity, reqParams)
	if err != nil {
		return "", fmt.Errorf("presign get %s: %w", key, err)
	}
	if m.publicURL != "" {
		return rewriteHost(u.String(), m.publicURL), nil
	}
	return u.String(), nil
}

// RemoveObject implements Service.
func (m *MinIOClient) RemoveObject(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	if err := m.client.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("remove object %s: %w", key, err)
	}
	return nil
}

// Bucket implements Service.
func (m *MinIOClient) Bucket() string { return m.bucket }

// sanitizeDownloadName strips characters that would break the
// Content-Disposition header without trying to be perfect.
func sanitizeDownloadName(s string) string {
	r := strings.NewReplacer(`"`, "", `\`, "", "\n", " ", "\r", " ")
	return r.Replace(s)
}

// rewriteHost replaces the scheme+host of a presigned URL with the configured
// public URL while preserving the path + signed query string. Used when the
// internal MinIO endpoint differs from what browsers can reach.
func rewriteHost(presigned, publicURL string) string {
	pu, err := url.Parse(presigned)
	if err != nil {
		return presigned
	}
	base, err := url.Parse(strings.TrimRight(publicURL, "/"))
	if err != nil || base.Host == "" {
		return presigned
	}
	pu.Scheme = base.Scheme
	pu.Host = base.Host
	return pu.String()
}
