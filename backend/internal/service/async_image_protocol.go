package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
)

const (
	AsyncImageDialectBB = "bb"
	AsyncImageDialectSC = "sc"

	AsyncImageKindText = "text_to_image"
	AsyncImageKindEdit = "image_to_image"

	defaultAsyncImageReferenceMaxBytes     = int64(32 << 20)
	defaultAsyncImageReferenceMaxPixels    = int64(80_000_000)
	defaultAsyncImageReferenceTimeout      = 30 * time.Second
	defaultAsyncImageReferenceMaxRedirects = 3
)

// AsyncImageInputPart preserves the order of text and reference-image parts in
// downstream BB requests. SC requests are normalized to images followed by text.
type AsyncImageInputPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	URL  string `json:"url,omitempty"`
}

// AsyncImageNormalizedRequest is the internal request shared by the BB and SC
// dialects. It is not exposed as a public wire format.
type AsyncImageNormalizedRequest struct {
	Dialect     string                `json:"dialect"`
	Platform    string                `json:"platform"`
	Kind        string                `json:"kind"`
	Model       string                `json:"model"`
	Prompt      string                `json:"prompt"`
	ImageSize   string                `json:"image_size,omitempty"`
	AspectRatio string                `json:"aspect_ratio,omitempty"`
	Parts       []AsyncImageInputPart `json:"parts"`
	SourcePath  string                `json:"source_path"`
}

func (r *AsyncImageNormalizedRequest) ReferenceCount() int {
	if r == nil {
		return 0
	}
	count := 0
	for _, part := range r.Parts {
		if part.Type == "image_url" && strings.TrimSpace(part.URL) != "" {
			count++
		}
	}
	return count
}

type bbGeminiRequest struct {
	Model     string            `json:"model"`
	Stream    bool              `json:"stream"`
	Messages  []bbGeminiMessage `json:"messages"`
	ExtraBody struct {
		Google struct {
			ImageConfig struct {
				ImageSize   string `json:"image_size"`
				AspectRatio string `json:"aspect_ratio"`
			} `json:"image_config"`
		} `json:"google"`
	} `json:"extra_body"`
}

type bbGeminiMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type bbGeminiContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

// ParseBBGeminiImageRequest validates the downstream BB Chat Completions
// dialect without changing the legacy /v1/chat/completions parser.
func ParseBBGeminiImageRequest(body []byte, sourcePath string) (*AsyncImageNormalizedRequest, error) {
	var in bbGeminiRequest
	if len(bytes.TrimSpace(body)) == 0 || json.Unmarshal(body, &in) != nil {
		return nil, errors.New("invalid JSON request body")
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		return nil, errors.New("model is required")
	}
	if in.Stream {
		return nil, errors.New("stream must be false for asynchronous image generation")
	}
	if len(in.Messages) == 0 {
		return nil, errors.New("messages must contain at least one user message")
	}

	parts := make([]AsyncImageInputPart, 0, len(in.Messages)*2)
	promptParts := make([]string, 0, len(in.Messages))
	userMessages := 0
	for _, message := range in.Messages {
		if strings.TrimSpace(strings.ToLower(message.Role)) != "user" {
			return nil, fmt.Errorf("unsupported message role %q; only user messages are accepted", message.Role)
		}
		userMessages++
		messageParts, texts, err := parseBBGeminiContent(message.Content)
		if err != nil {
			return nil, err
		}
		parts = append(parts, messageParts...)
		promptParts = append(promptParts, texts...)
	}
	if userMessages == 0 {
		return nil, errors.New("messages must contain at least one user message")
	}
	prompt := strings.TrimSpace(strings.Join(promptParts, "\n"))
	if prompt == "" {
		return nil, errors.New("a non-empty text prompt is required")
	}

	size, ratio, err := normalizeAsyncGeminiDimensions(
		in.ExtraBody.Google.ImageConfig.ImageSize,
		in.ExtraBody.Google.ImageConfig.AspectRatio,
		countAsyncImageReferences(parts) > 0,
	)
	if err != nil {
		return nil, err
	}
	kind := AsyncImageKindText
	if countAsyncImageReferences(parts) > 0 {
		kind = AsyncImageKindEdit
	}
	return &AsyncImageNormalizedRequest{
		Dialect:     AsyncImageDialectBB,
		Platform:    PlatformGemini,
		Kind:        kind,
		Model:       model,
		Prompt:      prompt,
		ImageSize:   size,
		AspectRatio: ratio,
		Parts:       parts,
		SourcePath:  strings.TrimSpace(sourcePath),
	}, nil
}

func parseBBGeminiContent(raw json.RawMessage) ([]AsyncImageInputPart, []string, error) {
	if len(raw) == 0 {
		return nil, nil, errors.New("message content is required")
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, nil, errors.New("message content must not be empty")
		}
		return []AsyncImageInputPart{{Type: "text", Text: text}}, []string{text}, nil
	}

	var inputParts []bbGeminiContentPart
	if json.Unmarshal(raw, &inputParts) != nil || len(inputParts) == 0 {
		return nil, nil, errors.New("message content must be a string or a non-empty content array")
	}
	parts := make([]AsyncImageInputPart, 0, len(inputParts))
	texts := make([]string, 0, len(inputParts))
	for _, part := range inputParts {
		switch strings.TrimSpace(strings.ToLower(part.Type)) {
		case "text":
			value := strings.TrimSpace(part.Text)
			if value == "" {
				return nil, nil, errors.New("text content part must not be empty")
			}
			parts = append(parts, AsyncImageInputPart{Type: "text", Text: value})
			texts = append(texts, value)
		case "image_url":
			value := strings.TrimSpace(part.ImageURL.URL)
			if value == "" {
				return nil, nil, errors.New("image_url.url is required")
			}
			parts = append(parts, AsyncImageInputPart{Type: "image_url", URL: value})
		default:
			return nil, nil, fmt.Errorf("unsupported content part type %q", part.Type)
		}
	}
	return parts, texts, nil
}

type scImageRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	ImageURLs   []string `json:"image_urls"`
	Resolution  string   `json:"resolution"`
	AspectRatio string   `json:"aspect_ratio"`
}

// ParseSCGeminiImageRequest validates the SC image-generation dialect. SC is
// intentionally Gemini-only in the first release.
func ParseSCGeminiImageRequest(body []byte, sourcePath string) (*AsyncImageNormalizedRequest, error) {
	var in scImageRequest
	if len(bytes.TrimSpace(body)) == 0 || json.Unmarshal(body, &in) != nil {
		return nil, errors.New("invalid JSON request body")
	}
	model := strings.TrimSpace(in.Model)
	prompt := strings.TrimSpace(in.Prompt)
	if model == "" {
		return nil, errors.New("model is required")
	}
	if prompt == "" {
		return nil, errors.New("prompt is required")
	}

	parts := make([]AsyncImageInputPart, 0, len(in.ImageURLs)+1)
	for _, rawURL := range in.ImageURLs {
		value := strings.TrimSpace(rawURL)
		if value == "" {
			return nil, errors.New("image_urls must not contain empty values")
		}
		parts = append(parts, AsyncImageInputPart{Type: "image_url", URL: value})
	}
	parts = append(parts, AsyncImageInputPart{Type: "text", Text: prompt})
	size, ratio, err := normalizeAsyncGeminiDimensions(in.Resolution, in.AspectRatio, len(in.ImageURLs) > 0)
	if err != nil {
		return nil, err
	}
	kind := AsyncImageKindText
	if len(in.ImageURLs) > 0 {
		kind = AsyncImageKindEdit
	}
	return &AsyncImageNormalizedRequest{
		Dialect:     AsyncImageDialectSC,
		Platform:    PlatformGemini,
		Kind:        kind,
		Model:       model,
		Prompt:      prompt,
		ImageSize:   size,
		AspectRatio: ratio,
		Parts:       parts,
		SourcePath:  strings.TrimSpace(sourcePath),
	}, nil
}

func normalizeAsyncGeminiDimensions(rawSize, rawRatio string, hasReference bool) (string, string, error) {
	size := strings.ToUpper(strings.TrimSpace(rawSize))
	if size != "" && size != "0.5K" && size != "1K" && size != "2K" && size != "4K" {
		return "", "", fmt.Errorf("unsupported_image_dimensions: unsupported image size %q", rawSize)
	}

	ratio := strings.ToLower(strings.TrimSpace(rawRatio))
	if ratio == "自动" {
		ratio = "auto"
	}
	if ratio == "auto" {
		if !hasReference {
			return "", "", errors.New("unsupported_image_dimensions: aspect_ratio=auto requires at least one reference image")
		}
		return size, "", nil
	}
	if ratio == "" {
		return size, "", nil
	}
	allowed := map[string]struct{}{
		"1:1": {}, "2:3": {}, "3:2": {}, "3:4": {}, "4:3": {},
		"4:5": {}, "5:4": {}, "9:16": {}, "16:9": {}, "21:9": {},
	}
	if _, ok := allowed[ratio]; !ok {
		return "", "", fmt.Errorf("unsupported_image_dimensions: unsupported aspect ratio %q", rawRatio)
	}
	return size, ratio, nil
}

func countAsyncImageReferences(parts []AsyncImageInputPart) int {
	count := 0
	for _, part := range parts {
		if part.Type == "image_url" && strings.TrimSpace(part.URL) != "" {
			count++
		}
	}
	return count
}

type AsyncImageReference struct {
	MIMEType string `json:"mime_type"`
	Data     []byte `json:"-"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	SHA256   string `json:"sha256"`
}

func (r *AsyncImageReference) DataURI() string {
	if r == nil {
		return ""
	}
	return "data:" + r.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(r.Data)
}

type AsyncImageReferenceDownloader struct {
	MaxBytes     int64
	MaxPixels    int64
	Timeout      time.Duration
	MaxRedirects int
	Resolver     *net.Resolver
}

// ValidateBytes applies the same MIME, decoder, pixel, and byte limits used
// for remote reference images to bytes received from multipart uploads.
func (d AsyncImageReferenceDownloader) ValidateBytes(data []byte, declaredType string) (*AsyncImageReference, error) {
	if int64(len(data)) > d.maxBytes() {
		return nil, errors.New("reference image exceeds the configured size limit")
	}
	return d.validateImage(data, declaredType)
}

func (d AsyncImageReferenceDownloader) Download(ctx context.Context, rawURL string) (*AsyncImageReference, error) {
	rawURL = strings.TrimSpace(rawURL)
	if strings.HasPrefix(strings.ToLower(rawURL), "data:") {
		return d.decodeDataURI(rawURL)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" {
		return nil, errors.New("reference image URL must be an absolute HTTPS URL or an image data URI")
	}
	if err := validateAsyncImagePublicHost(ctx, d.resolver(), parsed.Hostname()); err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = d.safeDialContext
	client := &http.Client{
		Transport: transport,
		Timeout:   d.timeout(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= d.maxRedirects() {
				return errors.New("reference image redirect limit exceeded")
			}
			if req.URL == nil || !strings.EqualFold(req.URL.Scheme, "https") {
				return errors.New("reference image redirects must use HTTPS")
			}
			return validateAsyncImagePublicHost(req.Context(), d.resolver(), req.URL.Hostname())
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/gif")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download reference image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("download reference image: unexpected HTTP status %d", resp.StatusCode)
	}
	if resp.ContentLength > d.maxBytes() {
		return nil, errors.New("reference image exceeds the configured size limit")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, d.maxBytes()+1))
	if err != nil {
		return nil, fmt.Errorf("read reference image: %w", err)
	}
	if int64(len(data)) > d.maxBytes() {
		return nil, errors.New("reference image exceeds the configured size limit")
	}
	return d.validateImage(data, resp.Header.Get("Content-Type"))
}

func (d AsyncImageReferenceDownloader) decodeDataURI(raw string) (*AsyncImageReference, error) {
	comma := strings.IndexByte(raw, ',')
	if comma <= len("data:") {
		return nil, errors.New("invalid image data URI")
	}
	meta := raw[len("data:"):comma]
	if !strings.HasSuffix(strings.ToLower(meta), ";base64") {
		return nil, errors.New("image data URI must use base64 encoding")
	}
	contentType := strings.TrimSpace(meta[:len(meta)-len(";base64")])
	decodedLen := base64.StdEncoding.DecodedLen(len(raw) - comma - 1)
	if int64(decodedLen) > d.maxBytes() {
		return nil, errors.New("reference image exceeds the configured size limit")
	}
	data, err := base64.StdEncoding.DecodeString(raw[comma+1:])
	if err != nil {
		return nil, errors.New("invalid base64 image data URI")
	}
	return d.validateImage(data, contentType)
}

func (d AsyncImageReferenceDownloader) validateImage(data []byte, declaredType string) (*AsyncImageReference, error) {
	if len(data) == 0 {
		return nil, errors.New("reference image is empty")
	}
	detected := http.DetectContentType(data)
	if mediaType, _, err := mime.ParseMediaType(detected); err == nil {
		detected = mediaType
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return nil, errors.New("reference image data is invalid or unsupported")
	}
	formatMIME := imageFormatMIME(format)
	if !strings.HasPrefix(strings.ToLower(detected), "image/") {
		detected = formatMIME
	}
	if !strings.HasPrefix(strings.ToLower(detected), "image/") {
		return nil, errors.New("reference URL did not return an image")
	}
	if declaredType != "" {
		mediaType, _, parseErr := mime.ParseMediaType(declaredType)
		if parseErr != nil || !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
			return nil, errors.New("reference image declared content type is not an image")
		}
		if !strings.EqualFold(mediaType, detected) {
			return nil, fmt.Errorf("reference image content type mismatch: declared %s, detected %s", mediaType, detected)
		}
	}
	if int64(config.Width)*int64(config.Height) > d.maxPixels() {
		return nil, errors.New("reference image exceeds the configured pixel limit")
	}
	sum := sha256.Sum256(data)
	return &AsyncImageReference{
		MIMEType: detected,
		Data:     data,
		Width:    config.Width,
		Height:   config.Height,
		SHA256:   hex.EncodeToString(sum[:]),
	}, nil
}

func imageFormatMIME(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

func (d AsyncImageReferenceDownloader) safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := d.resolver().LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("reference image host could not be resolved")
	}
	var lastErr error
	for _, ip := range addresses {
		if !isAsyncImagePublicIP(ip) {
			continue
		}
		dialer := net.Dialer{Timeout: d.timeout()}
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("reference image host resolves only to blocked addresses")
}

func validateAsyncImagePublicHost(ctx context.Context, resolver *net.Resolver, host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return errors.New("reference image URL host is required")
	}
	if parsed, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if !isAsyncImagePublicIP(parsed) {
			return errors.New("reference image URL points to a blocked network address")
		}
		return nil
	}
	addresses, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return errors.New("reference image host could not be resolved")
	}
	for _, ip := range addresses {
		if !isAsyncImagePublicIP(ip) {
			return errors.New("reference image host resolves to a blocked network address")
		}
	}
	return nil
}

var asyncImageBlockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func isAsyncImagePublicIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	if ip.Is4In6() {
		ip = ip.Unmap()
	}
	for _, prefix := range asyncImageBlockedPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func (d AsyncImageReferenceDownloader) resolver() *net.Resolver {
	if d.Resolver != nil {
		return d.Resolver
	}
	return net.DefaultResolver
}

func (d AsyncImageReferenceDownloader) maxBytes() int64 {
	if d.MaxBytes > 0 {
		return d.MaxBytes
	}
	return defaultAsyncImageReferenceMaxBytes
}

func (d AsyncImageReferenceDownloader) maxPixels() int64 {
	if d.MaxPixels > 0 {
		return d.MaxPixels
	}
	return defaultAsyncImageReferenceMaxPixels
}

func (d AsyncImageReferenceDownloader) timeout() time.Duration {
	if d.Timeout > 0 {
		return d.Timeout
	}
	return defaultAsyncImageReferenceTimeout
}

func (d AsyncImageReferenceDownloader) maxRedirects() int {
	if d.MaxRedirects > 0 {
		return d.MaxRedirects
	}
	return defaultAsyncImageReferenceMaxRedirects
}

// BuildGeminiAsyncChatBody downloads reference images and produces a normal
// Chat Completions request. Existing compatibility code then converts the data
// URI parts into Gemini inlineData while retaining the new image_config block.
func BuildGeminiAsyncChatBody(ctx context.Context, req *AsyncImageNormalizedRequest, downloader AsyncImageReferenceDownloader) ([]byte, error) {
	if req == nil {
		return nil, errors.New("normalized image request is required")
	}
	content := make([]any, 0, len(req.Parts))
	for _, part := range req.Parts {
		switch part.Type {
		case "text":
			content = append(content, map[string]any{"type": "text", "text": part.Text})
		case "image_url":
			imageRef, err := downloader.Download(ctx, part.URL)
			if err != nil {
				return nil, fmt.Errorf("invalid reference image: %w", err)
			}
			content = append(content, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": imageRef.DataURI()},
			})
		default:
			return nil, fmt.Errorf("unsupported normalized content part %q", part.Type)
		}
	}
	imageConfig := map[string]any{}
	if req.ImageSize != "" {
		imageConfig["image_size"] = req.ImageSize
	}
	if req.AspectRatio != "" {
		imageConfig["aspect_ratio"] = req.AspectRatio
	}
	out := map[string]any{
		"model":  req.Model,
		"stream": false,
		"messages": []any{map[string]any{
			"role":    "user",
			"content": content,
		}},
		"extra_body": map[string]any{
			"google": map[string]any{"image_config": imageConfig},
		},
	}
	return json.Marshal(out)
}

func AsyncImageTaskRequestHash(platform, dialect, sourcePath string, body []byte) string {
	h := sha256.New()
	_, _ = io.WriteString(h, strings.TrimSpace(platform))
	_, _ = io.WriteString(h, "\x00")
	_, _ = io.WriteString(h, strings.TrimSpace(dialect))
	_, _ = io.WriteString(h, "\x00")
	_, _ = io.WriteString(h, strings.TrimSpace(sourcePath))
	_, _ = io.WriteString(h, "\x00")
	_, _ = h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// AsyncImageSignedURLExpiryUnix returns zero for a public URL and an absolute
// expiry for signed URLs. Keeping this helper here makes the SC response's
// expires_at semantics explicit and testable.
func AsyncImageSignedURLExpiryUnix(now time.Time, expiry time.Duration, public bool) int64 {
	if public || expiry <= 0 {
		return 0
	}
	return now.Add(expiry).Unix()
}

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
