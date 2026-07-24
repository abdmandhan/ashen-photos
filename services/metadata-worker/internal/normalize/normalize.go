package normalize

import (
	"encoding/json"
	"time"

	"ashen/metadata-worker/internal/extract"
	"ashen/metadata-worker/internal/store"
)

// Normalize converts merged raw metadata into store.Technical (BR-004: missing
// fields are simply omitted, never an error).
func Normalize(raw []byte) store.Technical {
	var r extract.Raw
	_ = json.Unmarshal(raw, &r)

	exif := map[string]any{}
	if len(r.Exiftool) > 0 {
		_ = json.Unmarshal(r.Exiftool, &exif)
	}

	var t store.Technical
	t.MimeType = strP(exif, "MIMEType")
	// NOTE: exiftool's FileName is our temp path, not the user's original filename
	// (we store objects by sha). Real filename needs the client to send it — TODO.
	t.FileSize = intP64(exif, "FileSize")
	t.Width = firstIntP(exif, "ImageWidth", "ExifImageWidth")
	t.Height = firstIntP(exif, "ImageHeight", "ExifImageHeight")
	t.Orientation = intP(exif, "Orientation")
	t.CameraMake = strP(exif, "Make")
	t.CameraModel = strP(exif, "Model")
	t.LensMake = strP(exif, "LensMake")
	t.LensModel = firstStrP(exif, "LensModel", "LensID")
	t.ISO = firstIntP(exif, "ISO", "ISOSpeed")
	t.Aperture = firstFloatP(exif, "FNumber", "ApertureValue")
	t.FocalLength = floatP(exif, "FocalLength")
	t.ExposureTime = strAnyP(exif, "ExposureTime")
	t.Latitude = floatP(exif, "GPSLatitude")
	t.Longitude = floatP(exif, "GPSLongitude")
	t.Altitude = floatP(exif, "GPSAltitude")
	t.ContainerFormat = firstStrP(exif, "FileType", "FileTypeExtension")

	// Capture time: EXIF original > CreateDate > media create (documented order).
	if ts, src := captureTime(exif); ts != nil {
		t.CapturedAt = ts
		t.CapturedAtSource = &src
		conf := "HIGH"
		if src != "EXIF_DATE_TIME_ORIGINAL" {
			conf = "MEDIUM"
		}
		t.CapturedAtConfidence = &conf
	}

	// Video technical from ffprobe.
	if len(r.Ffprobe) > 0 {
		applyFfprobe(r.Ffprobe, &t)
	} else if d := floatP(exif, "Duration"); d != nil {
		ms := int(*d * 1000)
		t.DurationMs = &ms
	}

	// Validate GPS range (FR-007): drop invalid coords.
	if t.Latitude != nil && (*t.Latitude < -90 || *t.Latitude > 90) {
		t.Latitude, t.Longitude = nil, nil
	}
	if t.Longitude != nil && (*t.Longitude < -180 || *t.Longitude > 180) {
		t.Latitude, t.Longitude = nil, nil
	}
	return t
}

func applyFfprobe(raw json.RawMessage, t *store.Technical) {
	var p struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			RFrameRate string `json:"r_frame_rate"`
		} `json:"streams"`
		Format struct {
			FormatName string `json:"format_name"`
			Duration   string `json:"duration"`
			BitRate    string `json:"bit_rate"`
		} `json:"format"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	for _, s := range p.Streams {
		switch s.CodecType {
		case "video":
			cn := s.CodecName
			t.VideoCodec = &cn
			if s.Width > 0 {
				w := s.Width
				t.Width = &w
			}
			if s.Height > 0 {
				h := s.Height
				t.Height = &h
			}
			if fr := parseFrameRate(s.RFrameRate); fr > 0 {
				t.FrameRate = &fr
			}
		case "audio":
			cn := s.CodecName
			t.AudioCodec = &cn
		}
	}
	if p.Format.FormatName != "" {
		fn := p.Format.FormatName
		t.ContainerFormat = &fn
	}
	if d := parseFloat(p.Format.Duration); d > 0 {
		ms := int(d * 1000)
		t.DurationMs = &ms
	}
	if b := parseInt64(p.Format.BitRate); b > 0 {
		t.Bitrate = &b
	}
}

// captureTime returns the best timestamp + its source label.
func captureTime(exif map[string]any) (*time.Time, string) {
	order := []struct{ key, src string }{
		{"DateTimeOriginal", "EXIF_DATE_TIME_ORIGINAL"},
		{"SubSecDateTimeOriginal", "EXIF_DATE_TIME_ORIGINAL"},
		{"CreateDate", "EXIF_CREATE_DATE"},
		{"MediaCreateDate", "MEDIA_CREATE_DATE"},
	}
	for _, o := range order {
		if s, ok := exif[o.key].(string); ok && s != "" {
			if ts := parseExifTime(s); ts != nil {
				return ts, o.src
			}
		}
	}
	return nil, ""
}

func parseExifTime(s string) *time.Time {
	layouts := []string{
		"2006:01:02 15:04:05.999-07:00",
		"2006:01:02 15:04:05-07:00",
		"2006:01:02 15:04:05.999",
		"2006:01:02 15:04:05",
	}
	for _, l := range layouts {
		if ts, err := time.Parse(l, s); err == nil {
			return &ts
		}
	}
	return nil
}
