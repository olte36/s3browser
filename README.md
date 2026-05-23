# s3browser

A terminal UI for browsing S3-compatible object storage.

The application lists buckets, lets you open a bucket, displays objects as a
hierarchical tree using `/`-separated object keys, and shows object metadata plus
a bounded content preview.

## Features

- Browse buckets from AWS S3 or an S3-compatible endpoint.
- Navigate objects hierarchically by key prefix.
- View object path, size, content type, ETag, last modified time, and metadata.
- Preview object content without downloading the whole object.
- Show binary objects as a safe hex preview so terminal output is not corrupted.
- Keyboard-driven Bubble Tea interface.

## Requirements

- Go 1.24.3 or newer.
- Credentials from one of the supported CLI modes: raw keys, the AWS default
  credential chain, or Google Cloud application default credentials.

AWS credentials can come from common AWS sources such as:

- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`
- `AWS_SESSION_TOKEN`
- shared AWS credentials/config files
- IAM metadata credentials where available

## Usage

Install the app:

```bash
go install github.com/olte36/s3browser@latest
```

For local builds from this checkout:

```bash
make build
```

The compiled binary is written to `out/s3browser`.

Run against the default AWS S3 endpoint:

```bash
s3browser -access-key ACCESS_KEY -secret-key SECRET_KEY
```

Raw credentials are the default, so this is equivalent to:

```bash
s3browser -storage aws -creds raw -access-key ACCESS_KEY -secret-key SECRET_KEY
```

Run against AWS S3 using the AWS SDK default credential chain:

```bash
s3browser -storage aws -creds aws
```

Run against Google Cloud Storage using Google Cloud application default
credentials:

```bash
s3browser -storage gcp -creds gcp
```

Run with raw S3 access keys:

```bash
s3browser -access-key ACCESS_KEY -secret-key SECRET_KEY
```

Run against a custom S3-compatible endpoint:

```bash
s3browser -storage https://minio.example.com:9000 -creds raw -access-key ACCESS_KEY -secret-key SECRET_KEY
```

For local MinIO over HTTP:

```bash
s3browser -storage http://localhost:9000 -creds raw -access-key minioadmin -secret-key minioadmin
```

Run against the public MinIO playground:

```bash
s3browser -storage https://play.min.io -access-key Q3AM3UQ867SPQQA43P2F -secret-key zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG
```

The playground endpoint and credentials come from the MinIO client configuration
in https://github.com/minio/mc/blob/master/cmd/config-v10.go.

Custom endpoints use path-style bucket lookup, which works well with MinIO and
many S3-compatible services.

The `-storage` flag is empty by default and accepts either a storage URL or one of these aliases:

- `aws`: `https://s3.amazonaws.com`
- `gcp`: `https://storage.googleapis.com`

The `-creds` flag accepts:

- `raw`: uses `-access-key`, `-secret-key`, `-session-token`.
- `aws`: loads credentials through the AWS SDK default config.
- `gcp`: loads Google Cloud application default credentials.

If both `-access-key` and `-secret-key` are provided, raw credentials are used
even when `-creds` is omitted.

## Controls

| Key | Action |
| --- | --- |
| `up`, `k` | Move selection up |
| `down`, `j` | Move selection down |
| `pgup` | Move up by 10 |
| `pgdown` | Move down by 10 |
| `enter` | Open selected bucket, folder, or object |
| `w` | Toggle line wrapping in text previews |
| `backspace`, `esc`, `left`, `h` | Go back |
| `r` | Reload current view |
| `q`, `ctrl+c` | Quit |

Bucket and object selection wraps around: moving past the last item selects the
first item, and moving above the first item selects the last item.

## Preview Behavior

Object previews are limited to the first 256 KiB. Text previews are sanitized so
terminal control bytes cannot affect the interface, include line numbers, and
can be wrapped to the terminal width with `w`. Binary previews are rendered as
hex dumps, while metadata remains visible above the preview.
