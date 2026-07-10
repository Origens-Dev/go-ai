package ai

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	FileDataTypeBytes     = "bytes"
	FileDataTypeText      = "text"
	FileDataTypeURL       = "url"
	FileDataTypeReference = "reference"

	DefaultMaxDownloadSize int64 = 2 * 1024 * 1024 * 1024
)

// DownloadFunction downloads a URL and returns its bytes and optional media type.
type DownloadFunction func(ctx context.Context, url string) ([]byte, string, error)

type DownloadURLOptions struct {
	MaxBytes int64
	Client   *http.Client
}

type DataURL struct {
	MediaType string
	Data      []byte
	Base64    bool
}

type FileDataInput struct {
	Data              []byte
	Base64            string
	Text              string
	URL               string
	Reference         string
	ProviderReference ProviderReference
	MediaType         string
	Filename          string
}

type NormalizeFileDataOptions struct {
	SupportedURLs map[string][]string
	Download      DownloadFunction
}

func ConvertDataContentToBase64String(content any) (string, error) {
	switch value := content.(type) {
	case string:
		return value, nil
	case []byte:
		return base64.StdEncoding.EncodeToString(value), nil
	default:
		return "", NewInvalidDataContentError(content, nil, "")
	}
}

func ConvertDataContentToBytes(content any) ([]byte, error) {
	switch value := content.(type) {
	case []byte:
		return cloneBytes(value), nil
	case string:
		data, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, NewInvalidDataContentError(
				content,
				err,
				"Invalid data content. Content string is not a base64-encoded media.",
			)
		}
		return data, nil
	default:
		return nil, NewInvalidDataContentError(content, nil, "")
	}
}

func SplitDataURL(value string) (DataURL, error) {
	if !strings.HasPrefix(value, "data:") {
		return DataURL{}, &SDKError{Kind: ErrInvalidArgument, Message: "data URL must start with data:"}
	}
	header, body, ok := strings.Cut(value[len("data:"):], ",")
	if !ok {
		return DataURL{}, &SDKError{Kind: ErrInvalidArgument, Message: "data URL is missing comma separator"}
	}

	mediaType := "text/plain;charset=US-ASCII"
	base64Encoded := false
	if header != "" {
		parts := strings.Split(header, ";")
		if parts[0] != "" {
			mediaType = parts[0]
		}
		for _, part := range parts[1:] {
			if strings.EqualFold(part, "base64") {
				base64Encoded = true
				break
			}
		}
	}

	if base64Encoded {
		data, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			return DataURL{}, &SDKError{Kind: ErrInvalidArgument, Message: "data URL contains invalid base64 data", Cause: err}
		}
		return DataURL{MediaType: normalizeMediaType(mediaType), Data: data, Base64: true}, nil
	}

	decoded, err := url.PathUnescape(body)
	if err != nil {
		return DataURL{}, &SDKError{Kind: ErrInvalidArgument, Message: "data URL contains invalid percent-encoding", Cause: err}
	}
	return DataURL{MediaType: normalizeMediaType(mediaType), Data: []byte(decoded)}, nil
}

func NormalizeFilePart(ctx context.Context, input FileDataInput, opts *NormalizeFileDataOptions) (FilePart, error) {
	data, mediaType, err := NormalizeFileData(ctx, input, opts)
	if err != nil {
		return FilePart{}, err
	}
	return FilePart{Data: data, MediaType: mediaType, Filename: input.Filename}, nil
}

func NormalizeFileData(ctx context.Context, input FileDataInput, opts *NormalizeFileDataOptions) (FileData, string, error) {
	count := 0
	for _, present := range []bool{
		len(input.Data) > 0,
		input.Base64 != "",
		input.Text != "",
		input.URL != "",
		input.Reference != "",
		len(input.ProviderReference) > 0,
	} {
		if present {
			count++
		}
	}
	if count != 1 {
		return FileData{}, "", &SDKError{Kind: ErrInvalidArgument, Message: "exactly one file data source must be provided"}
	}

	mediaType := normalizeMediaType(input.MediaType)
	switch {
	case len(input.Data) > 0:
		if mediaType == "" {
			mediaType = DetectMediaType(input.Data, input.Filename)
		}
		return FileData{Type: FileDataTypeBytes, Data: cloneBytes(input.Data)}, mediaType, nil
	case input.Base64 != "":
		data, err := ConvertDataContentToBytes(input.Base64)
		if err != nil {
			return FileData{}, "", err
		}
		if mediaType == "" {
			mediaType = DetectMediaType(data, input.Filename)
		}
		return FileData{Type: FileDataTypeBytes, Data: data}, mediaType, nil
	case input.Text != "":
		if mediaType == "" {
			mediaType = "text/plain"
		}
		return FileData{Type: FileDataTypeText, Text: input.Text}, mediaType, nil
	case input.Reference != "":
		if mediaType == "" {
			mediaType = DetectMediaType(nil, input.Filename)
		}
		return FileData{Type: FileDataTypeReference, Reference: input.Reference}, mediaType, nil
	case len(input.ProviderReference) > 0:
		if mediaType == "" {
			mediaType = DetectMediaType(nil, input.Filename)
		}
		return FileData{Type: FileDataTypeReference, ProviderReference: cloneProviderReference(input.ProviderReference)}, mediaType, nil
	case input.URL != "":
		return normalizeURLFileData(ctx, input.URL, mediaType, input.Filename, opts)
	default:
		panic("unreachable")
	}
}

func DetectMediaType(data []byte, filename string) string {
	if len(data) > 0 {
		mediaType := normalizeMediaType(http.DetectContentType(data))
		if mediaType != "" && mediaType != "application/octet-stream" {
			return mediaType
		}
	}
	if filename != "" {
		if mediaType := normalizeMediaType(mime.TypeByExtension(filepath.Ext(filename))); mediaType != "" {
			return mediaType
		}
	}
	if len(data) > 0 {
		return "application/octet-stream"
	}
	return ""
}

func IsURLSupported(supportedURLs map[string][]string, mediaType string, rawURL string) bool {
	if len(supportedURLs) == 0 || rawURL == "" {
		return false
	}
	mediaType = normalizeMediaType(mediaType)
	for supportedMediaType, patterns := range supportedURLs {
		if !mediaTypeMatches(normalizeMediaType(supportedMediaType), mediaType) {
			continue
		}
		for _, pattern := range patterns {
			if urlPatternMatches(pattern, rawURL) {
				return true
			}
		}
	}
	return false
}

func DownloadURL(ctx context.Context, rawURL string, opts *DownloadURLOptions) ([]byte, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.HasPrefix(rawURL, "data:") {
		parsed, err := SplitDataURL(rawURL)
		if err != nil {
			return nil, "", NewDownloadError(rawURL, 0, "", err.Error(), err)
		}
		return cloneBytes(parsed.Data), parsed.MediaType, nil
	}
	if err := ValidateDownloadURL(rawURL); err != nil {
		return nil, "", err
	}

	maxBytes := DefaultMaxDownloadSize
	var client http.Client
	if opts != nil {
		if opts.MaxBytes > 0 {
			maxBytes = opts.MaxBytes
		}
		if opts.Client != nil {
			client = *opts.Client
		}
	}
	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := ValidateDownloadURL(req.URL.String()); err != nil {
			return err
		}
		if previousRedirect != nil {
			return previousRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", NewDownloadError(rawURL, 0, "", "Invalid URL: "+rawURL, err)
	}
	req.Header.Set("User-Agent", "go-ai/"+Version)
	resp, err := client.Do(req)
	if err != nil {
		if IsDownloadError(err) {
			return nil, "", err
		}
		return nil, "", NewDownloadError(rawURL, 0, "", "", err)
	}
	defer resp.Body.Close()

	if resp.Request != nil && resp.Request.URL != nil {
		if err := ValidateDownloadURL(resp.Request.URL.String()); err != nil {
			return nil, "", err
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", NewDownloadError(rawURL, resp.StatusCode, http.StatusText(resp.StatusCode), "", nil)
	}
	if resp.ContentLength > maxBytes {
		return nil, "", NewDownloadError(
			rawURL,
			0,
			"",
			fmt.Sprintf("Download of %s exceeded maximum size of %d bytes (Content-Length: %d).", rawURL, maxBytes, resp.ContentLength),
			nil,
		)
	}
	limited := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", NewDownloadError(rawURL, 0, "", "", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", NewDownloadError(
			rawURL,
			0,
			"",
			fmt.Sprintf("Download of %s exceeded maximum size of %d bytes.", rawURL, maxBytes),
			nil,
		)
	}
	return data, normalizeMediaType(resp.Header.Get("Content-Type")), nil
}

func ValidateDownloadURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" {
		return NewDownloadError(rawURL, 0, "", "Invalid URL: "+rawURL, err)
	}
	if parsed.Scheme == "data" {
		return nil
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return NewDownloadError(rawURL, 0, "", fmt.Sprintf("URL scheme must be http, https, or data, got %s:", parsed.Scheme), nil)
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return NewDownloadError(rawURL, 0, "", "URL must have a hostname", nil)
	}
	lowerHost := strings.TrimRight(strings.ToLower(hostname), ".")
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".local") || strings.HasSuffix(lowerHost, ".localhost") {
		return NewDownloadError(rawURL, 0, "", fmt.Sprintf("URL with hostname %s is not allowed", lowerHost), nil)
	}
	ip := net.ParseIP(lowerHost)
	if ip == nil {
		ip = parseLegacyIPv4(lowerHost)
	}
	if ip != nil && isPrivateDownloadIP(ip) {
		return NewDownloadError(rawURL, 0, "", fmt.Sprintf("URL with IP address %s is not allowed", hostname), nil)
	}
	return nil
}

func normalizeURLFileData(ctx context.Context, rawURL string, mediaType string, filename string, opts *NormalizeFileDataOptions) (FileData, string, error) {
	if strings.HasPrefix(rawURL, "data:") {
		parsed, err := SplitDataURL(rawURL)
		if err != nil {
			return FileData{}, "", err
		}
		if mediaType == "" {
			mediaType = parsed.MediaType
		}
		return FileData{Type: FileDataTypeBytes, Data: parsed.Data}, mediaType, nil
	}

	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return FileData{}, "", &SDKError{Kind: ErrInvalidArgument, Message: "file URL is invalid", Cause: err}
	}
	if mediaType == "" {
		mediaType = DetectMediaType(nil, filename)
	}
	if opts != nil && IsURLSupported(opts.SupportedURLs, mediaType, rawURL) {
		return FileData{Type: FileDataTypeURL, URL: rawURL}, mediaType, nil
	}
	if opts != nil && opts.Download != nil {
		data, downloadedMediaType, err := opts.Download(ctx, rawURL)
		if err != nil {
			return FileData{}, "", err
		}
		if mediaType == "" {
			mediaType = normalizeMediaType(downloadedMediaType)
		}
		if mediaType == "" {
			mediaType = DetectMediaType(data, filename)
		}
		return FileData{Type: FileDataTypeBytes, Data: cloneBytes(data)}, mediaType, nil
	}
	return FileData{Type: FileDataTypeURL, URL: rawURL}, mediaType, nil
}

func isPrivateDownloadIP(ip net.IP) bool {
	if ipv4 := ip.To4(); ipv4 != nil {
		return isPrivateDownloadIPv4(ipv4)
	}
	if len(ip) != net.IPv6len {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// fec0::/10 (deprecated site-local range).
	if ip[0] == 0xfe && ip[1]&0xc0 == 0xc0 {
		return true
	}
	if embedsDownloadIPv4(ip) {
		return isPrivateDownloadIPv4(net.IPv4(ip[12], ip[13], ip[14], ip[15]))
	}
	return false
}

func isPrivateDownloadIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return true
	}
	a, b, c := ip[0], ip[1], ip[2]
	return a == 0 ||
		a == 10 ||
		(a == 100 && b >= 64 && b <= 127) ||
		a == 127 ||
		(a == 169 && b == 254) ||
		(a == 172 && b >= 16 && b <= 31) ||
		(a == 192 && b == 0 && c == 0) ||
		(a == 192 && b == 168) ||
		(a == 198 && (b == 18 || b == 19)) ||
		a >= 240
}

func embedsDownloadIPv4(ip net.IP) bool {
	if len(ip) != net.IPv6len {
		return false
	}
	allZero := func(start, end int) bool {
		for _, b := range ip[start:end] {
			if b != 0 {
				return false
			}
		}
		return true
	}
	// ::/96, ::ffff:0:0/96, and ::ffff:0:0:0/96 translated forms.
	if allZero(0, 12) || (allZero(0, 10) && ip[10] == 0xff && ip[11] == 0xff) ||
		(allZero(0, 8) && ip[8] == 0xff && ip[9] == 0xff && ip[10] == 0 && ip[11] == 0) {
		return true
	}
	// 64:ff9b::/96 (well-known NAT64) and 64:ff9b:1::/48 (local-use NAT64).
	if ip[0] == 0x00 && ip[1] == 0x64 && ip[2] == 0xff && ip[3] == 0x9b {
		return allZero(4, 12) || (ip[4] == 0 && ip[5] == 1)
	}
	return false
}

// parseLegacyIPv4 implements the numeric forms normalized by WHATWG URL
// parsers (for example 2130706433, 0x7f000001, and 0177.0.0.1).
func parseLegacyIPv4(host string) net.IP {
	parts := strings.Split(host, ".")
	if len(parts) == 0 || len(parts) > 4 {
		return nil
	}
	values := make([]uint64, len(parts))
	for i, part := range parts {
		if part == "" {
			return nil
		}
		base := 10
		digits := part
		if strings.HasPrefix(part, "0x") || strings.HasPrefix(part, "0X") {
			base, digits = 16, part[2:]
		} else if len(part) > 1 && part[0] == '0' {
			base, digits = 8, part[1:]
		}
		if digits == "" {
			digits = "0"
		}
		value, err := strconv.ParseUint(digits, base, 32)
		if err != nil {
			return nil
		}
		values[i] = value
	}
	var raw uint64
	switch len(values) {
	case 1:
		raw = values[0]
	case 2:
		if values[0] > 0xff || values[1] > 0xffffff {
			return nil
		}
		raw = values[0]<<24 | values[1]
	case 3:
		if values[0] > 0xff || values[1] > 0xff || values[2] > 0xffff {
			return nil
		}
		raw = values[0]<<24 | values[1]<<16 | values[2]
	case 4:
		for _, value := range values {
			if value > 0xff {
				return nil
			}
		}
		raw = values[0]<<24 | values[1]<<16 | values[2]<<8 | values[3]
	}
	return net.IPv4(byte(raw>>24), byte(raw>>16), byte(raw>>8), byte(raw))
}

func normalizeMediaType(mediaType string) string {
	mediaType = strings.TrimSpace(mediaType)
	if mediaType == "" {
		return ""
	}
	parsed, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		return strings.ToLower(mediaType)
	}
	return strings.ToLower(parsed)
}

func mediaTypeMatches(pattern string, mediaType string) bool {
	if pattern == "" || pattern == "*" || pattern == "*/*" {
		return true
	}
	if mediaType == "" {
		return false
	}
	if pattern == mediaType {
		return true
	}
	prefix, ok := strings.CutSuffix(pattern, "/*")
	return ok && strings.HasPrefix(mediaType, prefix+"/")
}

func urlPatternMatches(pattern string, rawURL string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if pattern == "*" || pattern == rawURL {
		return true
	}
	if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") && len(pattern) > 1 {
		matched, err := regexp.MatchString(pattern[1:len(pattern)-1], rawURL)
		return err == nil && matched
	}
	if !strings.ContainsAny(pattern, "*?") {
		return strings.HasPrefix(rawURL, pattern)
	}

	re, err := regexp.Compile("^" + wildcardPatternToRegexp(pattern) + "$")
	return err == nil && re.MatchString(rawURL)
}

func wildcardPatternToRegexp(pattern string) string {
	var b strings.Builder
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return b.String()
}

func cloneBytes(data []byte) []byte {
	if data == nil {
		return nil
	}
	clone := make([]byte, len(data))
	copy(clone, data)
	return clone
}

func cloneProviderReference(ref ProviderReference) ProviderReference {
	if len(ref) == 0 {
		return nil
	}
	clone := make(ProviderReference, len(ref))
	for provider, reference := range ref {
		clone[provider] = reference
	}
	return clone
}
