package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// googleCloudStorageReadOnlyScope limits Google ADC tokens to object read access.
const googleCloudStorageReadOnlyScope = "https://www.googleapis.com/auth/devstorage.read_only"

// credentialConfig contains MinIO credentials and optional request transport.
type credentialConfig struct {
	creds     *credentials.Credentials
	transport http.RoundTripper
}

// awsCredentialProvider adapts AWS SDK credentials to the MinIO credential interface.
type awsCredentialProvider struct {
	ctx     context.Context
	expires time.Time
}

// googleCloudTransport adds Google OAuth headers to outgoing storage requests.
type googleCloudTransport struct {
	base      http.RoundTripper
	projectID string
	source    oauth2.TokenSource
}

// newCredentialConfig creates credentials for raw keys, AWS config, or Google ADC.
func newCredentialConfig(ctx context.Context, mode, accessKey, secretKey, sessionToken string) (credentialConfig, error) {
	mode = resolveCredentialMode(mode, accessKey, secretKey)
	switch mode {
	case "raw":
		if accessKey == "" && secretKey == "" && sessionToken == "" {
			return credentialConfig{
				creds: credentials.NewStatic("", "", "", credentials.SignatureAnonymous),
			}, nil
		}
		return credentialConfig{
			creds: credentials.NewStaticV4(accessKey, secretKey, sessionToken),
		}, nil
	case "aws":
		return credentialConfig{creds: newAWSCredentialChain(ctx)}, nil
	case "gcp":
		transport, err := newGoogleCloudTransport(ctx, http.DefaultTransport)
		if err != nil {
			return credentialConfig{}, err
		}
		return credentialConfig{
			creds:     credentials.NewStatic("", "", "", credentials.SignatureAnonymous),
			transport: transport,
		}, nil
	default:
		return credentialConfig{}, fmt.Errorf("-creds must be one of raw, aws, or gcp")
	}
}

// newAWSCredentialChain returns a MinIO credential chain backed by AWS default config.
func newAWSCredentialChain(ctx context.Context) *credentials.Credentials {
	return credentials.New(&awsCredentialProvider{ctx: ctx})
}

// newGoogleCloudTransport creates a transport that authenticates with Google ADC.
func newGoogleCloudTransport(ctx context.Context, base http.RoundTripper) (http.RoundTripper, error) {
	if base == nil {
		base = http.DefaultTransport
	}
	adc, err := google.FindDefaultCredentials(ctx, googleCloudStorageReadOnlyScope)
	if err != nil {
		return nil, fmt.Errorf("load Google Cloud default credentials: %w", err)
	}
	return &googleCloudTransport{
		base:      base,
		projectID: adc.ProjectID,
		source:    adc.TokenSource,
	}, nil
}

// Retrieve loads the current AWS credential value for MinIO.
func (p *awsCredentialProvider) Retrieve() (credentials.Value, error) {
	return p.retrieve(p.ctx)
}

// RetrieveWithCredContext loads AWS credentials for MinIO credential refreshes.
func (p *awsCredentialProvider) RetrieveWithCredContext(*credentials.CredContext) (credentials.Value, error) {
	return p.retrieve(p.ctx)
}

// IsExpired reports whether cached AWS credentials should be refreshed.
func (p *awsCredentialProvider) IsExpired() bool {
	return !p.expires.IsZero() && time.Now().After(p.expires.Add(-time.Minute))
}

// RoundTrip signs a request with Google ADC before delegating to the base transport.
func (t *googleCloudTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.source.Token()
	if err != nil {
		return nil, err
	}
	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()
	token.SetAuthHeader(cloned)
	if t.projectID != "" {
		cloned.Header.Set("x-goog-project-id", t.projectID)
	}
	return t.base.RoundTrip(cloned)
}

// retrieve loads AWS credentials and records their expiration for MinIO.
func (p *awsCredentialProvider) retrieve(ctx context.Context) (credentials.Value, error) {
	cfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		return credentials.Value{}, err
	}
	value, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return credentials.Value{}, err
	}
	p.expires = value.Expires
	return credentials.Value{
		AccessKeyID:     value.AccessKeyID,
		SecretAccessKey: value.SecretAccessKey,
		SessionToken:    value.SessionToken,
		SignerType:      credentials.SignatureV4,
	}, nil
}

// resolveCredentialMode applies defaults and raw-key inference to the credential mode.
func resolveCredentialMode(mode, accessKey, secretKey string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return "raw"
	}
	if accessKey != "" && secretKey != "" && mode == "aws" {
		return "raw"
	}
	return mode
}
