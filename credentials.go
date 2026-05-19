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

const googleCloudStorageReadOnlyScope = "https://www.googleapis.com/auth/devstorage.read_only"

type credentialConfig struct {
	creds     *credentials.Credentials
	transport http.RoundTripper
}

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

type awsCredentialProvider struct {
	ctx     context.Context
	expires time.Time
}

func newAWSCredentialChain(ctx context.Context) *credentials.Credentials {
	return credentials.New(&awsCredentialProvider{ctx: ctx})
}

func (p *awsCredentialProvider) Retrieve() (credentials.Value, error) {
	return p.retrieve(p.ctx)
}

func (p *awsCredentialProvider) RetrieveWithCredContext(*credentials.CredContext) (credentials.Value, error) {
	return p.retrieve(p.ctx)
}

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

func (p *awsCredentialProvider) IsExpired() bool {
	return !p.expires.IsZero() && time.Now().After(p.expires.Add(-time.Minute))
}

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

type googleCloudTransport struct {
	base      http.RoundTripper
	projectID string
	source    oauth2.TokenSource
}

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
