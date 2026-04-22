package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

const (
	mediaMaxUploadBytes    = 10 << 20
	mediaImportTimeout     = 15 * time.Second
	mediaImportRedirectMax = 3
)

type storedMediaAsset struct {
	URL         string
	MarkdownURL string
	ContentType string
	Width       int
	Height      int
	Size        int
}

func immutableFileServer(prefix string, root string) http.Handler {
	fileServer := http.StripPrefix(prefix, http.FileServer(http.Dir(root)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(w, r)
	})
}

func (a *App) handleAdminImageUpload(w http.ResponseWriter, r *http.Request) {
	a.handleAdminMediaImageUpload(w, r)
}

func (a *App) handleAdminMediaImageUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(mediaMaxUploadBytes); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]any{
			"msg":  "invalid form data",
			"code": -1,
		})
		return
	}

	headers := r.MultipartForm.File["image"]
	if len(headers) == 0 {
		a.respondJSON(w, http.StatusBadRequest, map[string]any{
			"msg":  "missing image",
			"code": -1,
		})
		return
	}

	errFiles := make([]string, 0)
	succMap := make(map[string]string, len(headers))
	assets := make([]map[string]any, 0, len(headers))
	seenLabels := make(map[string]int)

	for _, header := range headers {
		label := uniqueUploadLabel(header.Filename, seenLabels)
		file, err := header.Open()
		if err != nil {
			errFiles = append(errFiles, label)
			continue
		}

		asset, storeErr := a.storeManagedImage(label, file)
		_ = file.Close()
		if storeErr != nil {
			errFiles = append(errFiles, label)
			continue
		}

		succMap[label] = asset.MarkdownURL
		assets = append(assets, map[string]any{
			"name":         label,
			"url":          asset.URL,
			"markdown_url": asset.MarkdownURL,
			"width":        asset.Width,
			"height":       asset.Height,
			"content_type": asset.ContentType,
			"size":         asset.Size,
		})
	}

	if len(succMap) == 0 {
		a.respondJSON(w, http.StatusBadRequest, map[string]any{
			"msg":  "no valid image uploaded",
			"code": -1,
			"data": map[string]any{
				"errFiles": errFiles,
				"succMap":  map[string]string{},
			},
		})
		return
	}

	a.respondJSON(w, http.StatusOK, map[string]any{
		"msg":  "",
		"code": 0,
		"data": map[string]any{
			"errFiles": errFiles,
			"succMap":  succMap,
		},
		"assets": assets,
	})
}

func (a *App) handleAdminMediaImport(w http.ResponseWriter, r *http.Request) {
	type payload struct {
		URL string `json:"url"`
	}

	var req payload
	if err := a.decodeJSON(r, &req); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]any{
			"msg":  "invalid json",
			"code": -1,
		})
		return
	}

	asset, err := a.importManagedImage(r.Context(), req.URL)
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]any{
			"msg":  err.Error(),
			"code": -1,
		})
		return
	}

	a.respondJSON(w, http.StatusOK, map[string]any{
		"msg":  "",
		"code": 0,
		"data": map[string]any{
			"originalURL": strings.TrimSpace(req.URL),
			"url":         asset.MarkdownURL,
		},
		"asset": map[string]any{
			"url":          asset.URL,
			"markdown_url": asset.MarkdownURL,
			"width":        asset.Width,
			"height":       asset.Height,
			"content_type": asset.ContentType,
			"size":         asset.Size,
		},
	})
}

func (a *App) importManagedImage(ctx context.Context, rawURL string) (storedMediaAsset, error) {
	target, err := normalizeRemoteMediaURL(rawURL)
	if err != nil {
		return storedMediaAsset{}, err
	}

	client := newRemoteMediaClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return storedMediaAsset{}, err
	}
	req.Header.Set("Accept", "image/webp,image/png,image/jpeg,image/gif,image/*;q=0.9,*/*;q=0.1")
	req.Header.Set("User-Agent", "snemc-blog-media-import/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return storedMediaAsset{}, fmt.Errorf("failed to fetch remote image")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return storedMediaAsset{}, fmt.Errorf("remote image request failed")
	}
	if resp.ContentLength > mediaMaxUploadBytes {
		return storedMediaAsset{}, fmt.Errorf("remote image too large")
	}

	name := remoteMediaName(target.Path)
	return a.storeManagedImage(name, resp.Body)
}

func (a *App) storeManagedImage(originalName string, reader io.Reader) (storedMediaAsset, error) {
	data, err := readMediaBytes(reader, mediaMaxUploadBytes)
	if err != nil {
		return storedMediaAsset{}, err
	}

	width, height, contentType, ext, err := detectImageMetadata(data)
	if err != nil {
		return storedMediaAsset{}, err
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	relativePath := filepath.Join(hash[:2], hash[2:4], hash+ext)
	fullDir := filepath.Join(a.cfg.MediaDir, hash[:2], hash[2:4])
	fullPath := filepath.Join(a.cfg.MediaDir, relativePath)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		return storedMediaAsset{}, err
	}

	if _, err := os.Stat(fullPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return storedMediaAsset{}, err
		}
		if err := writeMediaFile(fullDir, fullPath, data); err != nil {
			return storedMediaAsset{}, err
		}
	}

	cleanURL := "/media/" + filepath.ToSlash(relativePath)
	return storedMediaAsset{
		URL:         cleanURL,
		MarkdownURL: buildMarkdownMediaURL(cleanURL, width, height),
		ContentType: contentType,
		Width:       width,
		Height:      height,
		Size:        len(data),
	}, nil
}

func buildMarkdownMediaURL(cleanURL string, width int, height int) string {
	return cleanURL + "?w=" + strconv.Itoa(width) + "&h=" + strconv.Itoa(height)
}

func writeMediaFile(dir string, destination string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, destination)
}

func detectImageMetadata(data []byte) (width int, height int, contentType string, ext string, err error) {
	if len(data) == 0 {
		return 0, 0, "", "", fmt.Errorf("empty image")
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, "", "", fmt.Errorf("invalid image content")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, "", "", fmt.Errorf("invalid image dimensions")
	}

	switch strings.ToLower(format) {
	case "jpeg":
		return cfg.Width, cfg.Height, "image/jpeg", ".jpg", nil
	case "png":
		return cfg.Width, cfg.Height, "image/png", ".png", nil
	case "gif":
		return cfg.Width, cfg.Height, "image/gif", ".gif", nil
	case "webp":
		return cfg.Width, cfg.Height, "image/webp", ".webp", nil
	default:
		return 0, 0, "", "", fmt.Errorf("unsupported image type")
	}
}

func readMediaBytes(reader io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(reader, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("image too large")
	}
	return data, nil
}

func normalizeRemoteMediaURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid image url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported image url")
	}
	if parsed.Hostname() == "" || parsed.User != nil {
		return nil, fmt.Errorf("unsupported image url")
	}
	return parsed, nil
}

func remoteMediaName(rawPath string) string {
	name := path.Base(rawPath)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		return "image"
	}
	return name
}

func uniqueUploadLabel(name string, seen map[string]int) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "image"
	}
	if seen[base] == 0 {
		seen[base] = 1
		return base
	}
	seen[base]++
	return fmt.Sprintf("%s-%d", base, seen[base])
}

func newRemoteMediaClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: 10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			targetIP, err := resolveSafeMediaIP(ctx, host)
			if err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(targetIP, port))
		},
	}

	return &http.Client{
		Timeout:   mediaImportTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= mediaImportRedirectMax {
				return fmt.Errorf("too many redirects")
			}
			_, err := normalizeRemoteMediaURL(req.URL.String())
			return err
		},
	}
}

func resolveSafeMediaIP(ctx context.Context, host string) (string, error) {
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedMediaIP(ip) {
			return "", fmt.Errorf("blocked remote host")
		}
		return ip.String(), nil
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return "", fmt.Errorf("failed to resolve remote host")
	}
	for _, addr := range addrs {
		if isBlockedMediaIP(addr.IP) {
			continue
		}
		return addr.IP.String(), nil
	}
	return "", fmt.Errorf("blocked remote host")
}

func isBlockedMediaIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}
