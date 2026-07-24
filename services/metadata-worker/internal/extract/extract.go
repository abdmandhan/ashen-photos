package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/minio/minio-go/v7"
)

type Extractor struct{ s3 *minio.Client }

func New(s3 *minio.Client) *Extractor { return &Extractor{s3: s3} }

// Raw is the merged metadata blob stored per asset.
type Raw struct {
	Exiftool json.RawMessage `json:"exiftool,omitempty"`
	Ffprobe  json.RawMessage `json:"ffprobe,omitempty"`
}

// Run downloads the original to a temp file and runs exiftool (+ ffprobe for
// video). Streams to disk — never loads the whole media into memory. The temp
// file is always removed. Original object is never modified (BR-003).
func (e *Extractor) Run(ctx context.Context, bucket, key, mediaType string) ([]byte, error) {
	tmp, err := os.CreateTemp("", "ashen-meta-*")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	if err := e.s3.FGetObject(ctx, bucket, key, path, minio.GetObjectOptions{}); err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}

	var raw Raw

	// exiftool works for both images and videos; -n gives numeric (signed decimal GPS).
	exifOut, err := exec.CommandContext(ctx, "exiftool", "-json", "-n", path).Output()
	if err != nil {
		return nil, fmt.Errorf("exiftool: %w", err)
	}
	// exiftool returns a JSON array with one object; unwrap to the object.
	var arr []json.RawMessage
	if err := json.Unmarshal(exifOut, &arr); err == nil && len(arr) > 0 {
		raw.Exiftool = arr[0]
	} else {
		raw.Exiftool = exifOut
	}

	if mediaType == "video" {
		probe, perr := exec.CommandContext(ctx, "ffprobe", "-v", "quiet",
			"-print_format", "json", "-show_format", "-show_streams", path).Output()
		if perr == nil {
			raw.Ffprobe = probe
		}
		// ffprobe failure is non-fatal — exiftool still gives useful fields.
	}

	return json.Marshal(raw)
}
