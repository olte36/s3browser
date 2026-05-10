package main

import (
	"context"
	"time"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type awsCredentialProvider struct {
	expires time.Time
}

func newAWSCredentialChain() *credentials.Credentials {
	return credentials.New(&awsCredentialProvider{})
}

func (p *awsCredentialProvider) Retrieve() (credentials.Value, error) {
	return p.retrieve(context.Background())
}

func (p *awsCredentialProvider) RetrieveWithCredContext(*credentials.CredContext) (credentials.Value, error) {
	return p.retrieve(context.Background())
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
