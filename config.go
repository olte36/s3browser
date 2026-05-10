package main

import (
	"fmt"
	"net/url"
	"strings"
)

const defaultS3URL = "https://s3.amazonaws.com"

type endpointConfig struct {
	Raw      string
	Endpoint string
	Secure   bool
}

func parseEndpoint(raw string) (endpointConfig, error) {
	if strings.TrimSpace(raw) == "" {
		raw = defaultS3URL
	}

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
		Endpoint: u.Host,
		Secure:   u.Scheme == "https",
	}, nil
}
