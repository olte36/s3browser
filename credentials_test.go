package main

import (
	"context"
	"testing"
)

func TestNewCredentialConfigRaw(t *testing.T) {
	cfg, err := newCredentialConfig(context.Background(), "raw", "access", "secret", "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.creds == nil {
		t.Fatal("expected credentials")
	}
	value, err := cfg.creds.Get()
	if err != nil {
		t.Fatalf("get credentials: %v", err)
	}
	if value.AccessKeyID != "access" || value.SecretAccessKey != "secret" || value.SessionToken != "token" {
		t.Fatalf("unexpected credential value: %#v", value)
	}
	if cfg.transport != nil {
		t.Fatal("raw credentials should not set a custom transport")
	}
}

func TestNewCredentialConfigDefaultsToRaw(t *testing.T) {
	cfg, err := newCredentialConfig(context.Background(), "", "access", "secret", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	value, err := cfg.creds.Get()
	if err != nil {
		t.Fatalf("get credentials: %v", err)
	}
	if value.AccessKeyID != "access" || value.SecretAccessKey != "secret" {
		t.Fatalf("unexpected credential value: %#v", value)
	}
}

func TestNewCredentialConfigInfersRawWhenKeysProvided(t *testing.T) {
	cfg, err := newCredentialConfig(context.Background(), "aws", "access", "secret", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	value, err := cfg.creds.Get()
	if err != nil {
		t.Fatalf("get credentials: %v", err)
	}
	if value.AccessKeyID != "access" || value.SecretAccessKey != "secret" {
		t.Fatalf("unexpected credential value: %#v", value)
	}
}

func TestNewCredentialConfigRawAllowsAnonymous(t *testing.T) {
	cfg, err := newCredentialConfig(context.Background(), "raw", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.creds == nil {
		t.Fatal("expected credentials")
	}
	value, err := cfg.creds.Get()
	if err != nil {
		t.Fatalf("get credentials: %v", err)
	}
	if value.AccessKeyID != "" || value.SecretAccessKey != "" {
		t.Fatalf("unexpected credential value: %#v", value)
	}
}

func TestNewCredentialConfigRejectsUnknownMode(t *testing.T) {
	if _, err := newCredentialConfig(context.Background(), "other", "", "", ""); err == nil {
		t.Fatal("expected error")
	}
}
