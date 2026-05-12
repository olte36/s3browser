# S3 Objects Browser

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

Run against the default AWS S3 endpoint:

```bash
go run . -access-key ACCESS_KEY -secret-key SECRET_KEY
```

Raw credentials are the default, so this is equivalent to:

```bash
go run . -storage aws -creds raw -access-key ACCESS_KEY -secret-key SECRET_KEY
```

Run against AWS S3 using the AWS SDK default credential chain:

```bash
go run . -storage aws -creds aws
```

Run against Google Cloud Storage using Google Cloud application default
credentials:

```bash
go run . -storage gcp -creds gcp
```

Run with raw S3 access keys:

```bash
go run . -access-key ACCESS_KEY -secret-key SECRET_KEY
```

Run against a custom S3-compatible endpoint:

```bash
go run . -storage https://minio.example.com:9000 -creds raw -access-key ACCESS_KEY -secret-key SECRET_KEY
```

For local MinIO over HTTP:

```bash
go run . -storage http://localhost:9000 -creds raw -access-key minioadmin -secret-key minioadmin
```

Custom endpoints use path-style bucket lookup, which works well with MinIO and
many S3-compatible services.

The `-storage` flag is empty by default and accepts either a storage URL or one of these aliases:

- `aws`: `https://s3.amazonaws.com`
- `gcp`: `https://storage.googleapis.com`

The `-creds` flag accepts:

- `raw`: requires `-access-key` and `-secret-key`; `-session-token` is optional.
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
| `backspace`, `esc`, `left`, `h` | Go back |
| `r` | Reload current view |
| `q`, `ctrl+c` | Quit |

Bucket and object selection wraps around: moving past the last item selects the
first item, and moving above the first item selects the last item.

## Preview Behavior

Object previews are limited to the first 256 KiB. Text previews are sanitized so
terminal control bytes cannot affect the interface. Binary previews are rendered
as hex dumps, while metadata remains visible above the preview.
