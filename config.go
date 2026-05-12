package main

import (
	"fmt"
	"net/url"
	"strings"
)

const defaultS3URL = "https://s3.amazonaws.com"
const defaultGCSURL = "https://storage.googleapis.com"

type endpointConfig struct {
	Raw      string
	Display  string
	Endpoint string
	Secure   bool
}

func parseEndpoint(raw string) (endpointConfig, error) {
	display := storageDisplayName(raw)
	if strings.TrimSpace(raw) == "" {
		raw = defaultS3URL
	}
	raw = resolveEndpointAlias(raw)

	parseRaw := raw
	if !strings.Contains(parseRaw, "://") {
		parseRaw = "https://" + parseRaw
	}

	u, err := url.Parse(parseRaw)
	if err != nil {
		return endpointConfig{}, fmt.Errorf("parse endpoint URL: %w", err)
	}

	switch u.Scheme {
	case "http", "https":
	default:
		return endpointConfig{}, fmt.Errorf("unsupported endpoint scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return endpointConfig{}, fmt.Errorf("endpoint URL must include a host")
	}
	if u.Path != "" && u.Path != "/" {
		return endpointConfig{}, fmt.Errorf("endpoint URL must not include a path")
	}

	return endpointConfig{
		Raw:      raw,
		Display:  display,
		Endpoint: u.Host,
		Secure:   u.Scheme == "https",
	}, nil
}

func storageDisplayName(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "aws":
		return "AWS"
	case "gcp":
		return "GCP"
	default:
		return strings.TrimSpace(raw)
	}
}

func resolveEndpointAlias(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "aws":
		return defaultS3URL
	case "gcp":
		return defaultGCSURL
	default:
		return raw
	}
}
