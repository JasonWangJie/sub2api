package service

import (
	"encoding/base64"
	"strings"

	apperrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// DecodeImagePayload accepts raw base64 or data URL.
func DecodeImagePayload(raw string) (data []byte, mime string, format string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", "", apperrors.BadRequest("INVALID_IMAGE", "image is required")
	}
	if strings.HasPrefix(raw, "data:") {
		parts := strings.SplitN(raw, ",", 2)
		if len(parts) != 2 {
			return nil, "", "", apperrors.BadRequest("INVALID_IMAGE", "invalid data url")
		}
		meta := parts[0]
		payload := parts[1]
		mime = "image/png"
		if i := strings.Index(meta, ":"); i >= 0 {
			rest := meta[i+1:]
			if j := strings.Index(rest, ";"); j >= 0 {
				mime = rest[:j]
			} else {
				mime = strings.TrimSuffix(rest, ";base64")
			}
		}
		data, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, "", "", apperrors.BadRequest("INVALID_IMAGE", "invalid base64 image")
		}
		format = formatFromMime(mime)
		return data, mime, format, nil
	}
	data, err = base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, "", "", apperrors.BadRequest("INVALID_IMAGE", "invalid base64 image")
	}
	return data, "image/png", "png", nil
}

func mimeFromFormat(format string) string {
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func formatFromMime(mime string) string {
	switch strings.ToLower(mime) {
	case "image/jpeg", "image/jpg":
		return "jpeg"
	case "image/webp":
		return "webp"
	default:
		return "png"
	}
}

func extFromFormat(format string) string {
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		return "jpg"
	case "webp":
		return "webp"
	default:
		return "png"
	}
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func truncateRunes(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}
