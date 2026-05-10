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
- S3-compatible credentials available through the AWS default credential chain.

Credentials can come from common AWS sources such as:

- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`
- `AWS_SESSION_TOKEN`
- shared AWS credentials/config files
- IAM metadata credentials where available

## Usage

Run against the default AWS S3 endpoint:

```bash
go run .
```

Run against a custom S3-compatible endpoint:

```bash
go run . -url https://minio.example.com:9000
```

For local MinIO over HTTP:

```bash
go run . -url http://localhost:9000
```

Custom endpoints use path-style bucket lookup, which works well with MinIO and
many S3-compatible services.

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
