package main

import "testing"

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		display  string
		endpoint string
		secure   bool
		wantErr  bool
	}{
		{name: "default", display: "AWS", endpoint: "s3.amazonaws.com", secure: true},
		{name: "aws alias", raw: "aws", display: "AWS", endpoint: "s3.amazonaws.com", secure: true},
		{name: "gcp alias", raw: "gcp", display: "GCP", endpoint: "storage.googleapis.com", secure: true},
		{name: "https", raw: "https://minio.example.com:9000", display: "https://minio.example.com:9000", endpoint: "minio.example.com:9000", secure: true},
		{name: "http", raw: "http://localhost:9000", display: "http://localhost:9000", endpoint: "localhost:9000", secure: false},
		{name: "host only defaults https", raw: "localhost:9000", display: "localhost:9000", endpoint: "localhost:9000", secure: true},
		{name: "reject path", raw: "https://localhost:9000/base", wantErr: true},
		{name: "reject scheme", raw: "ftp://localhost", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseEndpoint(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Endpoint != tt.endpoint || got.Secure != tt.secure {
				t.Fatalf("got endpoint=%q secure=%v, want endpoint=%q secure=%v", got.Endpoint, got.Secure, tt.endpoint, tt.secure)
			}
			if got.Display != tt.display {
				t.Fatalf("display = %q, want %q", got.Display, tt.display)
			}
		})
	}
}
