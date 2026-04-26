package server

import (
	"archive/zip"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/snemc/snemc-blog/internal/store"
)

const (
	staticSiteMaxUploadBytes = 128 << 20
	staticSiteMaxUploadFiles = 512
)

type staticSiteUploadFile struct {
	RelativePath string
	Data         []byte
}

type staticSiteUploadBundle struct {
	Files        []staticSiteUploadFile
	EntryPath    string
	StorageMode  string
	DownloadName string
	FileCount    int
	TotalSize    int64
}

func (a *App) handleAdminStaticSites(w http.ResponseWriter, r *http.Request) {
	sites, err := a.store.ListStaticSites(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, map[string]any{"sites": sites})
}

func (a *App) handleAdminCreateStaticSite(w http.ResponseWriter, r *http.Request) {
	site, err := a.store.CreateStaticSite(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusCreated, map[string]any{"site": site})
}

func (a *App) handleAdminUploadStaticSite(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	site, err := a.store.GetStaticSiteByID(r.Context(), id)
	if err == store.ErrNotFound {
		a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "static site not found"})
		return
	}
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, staticSiteMaxUploadBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	bundle, err := collectStaticSiteUpload(
		r.MultipartForm.File["files"],
		r.MultipartForm.Value["paths"],
		firstMultipartValue(r.MultipartForm.Value["entry_path"]),
		firstMultipartValue(r.MultipartForm.Value["download_name"]),
	)
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := a.replaceStaticSiteFiles(site.RouteID, bundle.Files); err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	site, err = a.store.UpdateStaticSiteUpload(r.Context(), site.ID, store.StaticSiteUploadState{
		EntryPath:    bundle.EntryPath,
		StorageMode:  bundle.StorageMode,
		DownloadName: bundle.DownloadName,
		FileCount:    bundle.FileCount,
		TotalSize:    bundle.TotalSize,
	})
	if err == store.ErrNotFound {
		a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "static site not found"})
		return
	}
	if err == store.ErrInvalidInput {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid static site upload"})
		return
	}
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, map[string]any{"site": site})
}

func (a *App) handleAdminDownloadStaticSite(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	site, err := a.store.GetStaticSiteByID(r.Context(), id)
	if err == store.ErrNotFound {
		a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "static site not found"})
		return
	}
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if site.EntryPath == "" || site.FileCount == 0 {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "static site has no uploaded files"})
		return
	}

	siteDir := a.staticSiteDir(site.RouteID)
	if site.StorageMode == store.StaticSiteStorageSingleFile {
		fullPath, err := resolveStaticSiteFSPath(siteDir, site.EntryPath)
		if err != nil {
			a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid entry path"})
			return
		}
		if _, err := os.Stat(fullPath); err != nil {
			if os.IsNotExist(err) {
				a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "static site file not found"})
				return
			}
			a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		filename := sanitizeAttachmentName(site.DownloadName, site.RouteID+".html")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, fullPath)
		return
	}

	filename := sanitizeAttachmentName(site.DownloadName, site.RouteID)
	if !strings.HasSuffix(strings.ToLower(filename), ".zip") {
		filename += ".zip"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if err := writeStaticSiteZip(w, siteDir, strings.TrimSuffix(filename, ".zip")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *App) handleAdminDeleteStaticSite(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	site, err := a.store.DeleteStaticSite(r.Context(), id)
	if err == store.ErrNotFound {
		a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "static site not found"})
		return
	}
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if removeErr := os.RemoveAll(a.staticSiteDir(site.RouteID)); removeErr != nil {
		a.respondJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"warning": "route deleted but static files could not be fully removed",
		})
		return
	}
	a.respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleStaticSite(w http.ResponseWriter, r *http.Request) {
	routeID := strings.TrimSpace(chi.URLParam(r, "route_id"))
	site, err := a.store.GetStaticSiteByRouteID(r.Context(), routeID)
	if err == store.ErrNotFound || site.EntryPath == "" || site.FileCount == 0 {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	siteDir := a.staticSiteDir(site.RouteID)
	requestPath := chi.URLParam(r, "*")
	hasTrailingSlash := strings.HasSuffix(r.URL.Path, "/")
	if requestPath == "" {
		if !hasTrailingSlash {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			return
		}
		if site.EntryPath == "index.html" || site.EntryPath == "index.htm" {
			a.serveStaticSiteFile(w, r, siteDir, site.EntryPath)
			return
		}
		http.Redirect(w, r, site.EntryPath, http.StatusFound)
		return
	}

	requestPath = strings.TrimSuffix(requestPath, "/")
	relativePath, err := cleanStaticSiteRelativePath(requestPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if hasTrailingSlash {
		for _, indexName := range []string{"index.html", "index.htm"} {
			candidate := path.Join(relativePath, indexName)
			if a.staticSiteFileExists(siteDir, candidate) {
				a.serveStaticSiteFile(w, r, siteDir, candidate)
				return
			}
		}
		http.NotFound(w, r)
		return
	}
	if a.staticSiteFileExists(siteDir, relativePath) {
		a.serveStaticSiteFile(w, r, siteDir, relativePath)
		return
	}
	for _, indexName := range []string{"index.html", "index.htm"} {
		candidate := path.Join(relativePath, indexName)
		if a.staticSiteFileExists(siteDir, candidate) {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			return
		}
	}
	http.NotFound(w, r)
}

func (a *App) staticSitesRootDir() string {
	return filepath.Join(a.cfg.DataDir, "static-sites")
}

func (a *App) staticSiteDir(routeID string) string {
	return filepath.Join(a.staticSitesRootDir(), routeID)
}

func (a *App) replaceStaticSiteFiles(routeID string, files []staticSiteUploadFile) error {
	rootDir := a.staticSitesRootDir()
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(rootDir, ".site-upload-*")
	if err != nil {
		return err
	}
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.RemoveAll(tempDir)
		}
	}()

	for _, item := range files {
		fullPath, err := resolveStaticSiteFSPath(tempDir, item.RelativePath)
		if err != nil {
			return err
		}
		fullDir := filepath.Dir(fullPath)
		if err := os.MkdirAll(fullDir, 0o755); err != nil {
			return err
		}
		if err := writeMediaFile(fullDir, fullPath, item.Data); err != nil {
			return err
		}
	}

	targetDir := a.staticSiteDir(routeID)
	if err := os.RemoveAll(targetDir); err != nil {
		return err
	}
	if err := os.Rename(tempDir, targetDir); err != nil {
		return err
	}
	keepTemp = true
	return nil
}

func (a *App) staticSiteFileExists(siteDir string, relativePath string) bool {
	fullPath, err := resolveStaticSiteFSPath(siteDir, relativePath)
	if err != nil {
		return false
	}
	info, err := os.Stat(fullPath)
	return err == nil && !info.IsDir()
}

func (a *App) serveStaticSiteFile(w http.ResponseWriter, r *http.Request, siteDir string, relativePath string) {
	fullPath, err := resolveStaticSiteFSPath(siteDir, relativePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	setStaticSiteResponseHeaders(w, relativePath)
	http.ServeFile(w, r, fullPath)
}

func collectStaticSiteUpload(headers []*multipart.FileHeader, paths []string, requestedEntry string, requestedName string) (staticSiteUploadBundle, error) {
	if len(headers) == 0 {
		return staticSiteUploadBundle{}, fmt.Errorf("missing upload files")
	}
	if len(headers) > staticSiteMaxUploadFiles {
		return staticSiteUploadBundle{}, fmt.Errorf("too many files in one upload")
	}
	if len(paths) > 0 && len(paths) != len(headers) {
		return staticSiteUploadBundle{}, fmt.Errorf("uploaded file paths do not match files")
	}

	rawPaths := make([]string, len(headers))
	for i, header := range headers {
		if len(paths) == len(headers) {
			rawPaths[i] = paths[i]
			continue
		}
		rawPaths[i] = header.Filename
	}

	cleanedPaths := make([]string, len(rawPaths))
	for i, rawPath := range rawPaths {
		cleaned, err := cleanStaticSiteRelativePath(rawPath)
		if err != nil {
			return staticSiteUploadBundle{}, fmt.Errorf("invalid file path: %s", rawPath)
		}
		cleanedPaths[i] = cleaned
	}
	strippedPaths, rootName := stripCommonUploadRoot(cleanedPaths)

	files := make([]staticSiteUploadFile, 0, len(headers))
	seenPaths := make(map[string]struct{}, len(headers))
	var totalSize int64
	for index, header := range headers {
		relativePath := strippedPaths[index]
		if _, exists := seenPaths[relativePath]; exists {
			return staticSiteUploadBundle{}, fmt.Errorf("duplicate uploaded path: %s", relativePath)
		}
		seenPaths[relativePath] = struct{}{}

		file, err := header.Open()
		if err != nil {
			return staticSiteUploadBundle{}, err
		}
		data, readErr := io.ReadAll(file)
		_ = file.Close()
		if readErr != nil {
			return staticSiteUploadBundle{}, readErr
		}
		totalSize += int64(len(data))
		files = append(files, staticSiteUploadFile{
			RelativePath: relativePath,
			Data:         data,
		})
	}

	entryPath, err := resolveStaticSiteEntryPath(strippedPaths, requestedEntry)
	if err != nil {
		return staticSiteUploadBundle{}, err
	}
	storageMode := store.StaticSiteStorageDirectory
	if len(files) == 1 && !strings.Contains(files[0].RelativePath, "/") {
		storageMode = store.StaticSiteStorageSingleFile
	}

	downloadName := requestedName
	if strings.TrimSpace(downloadName) == "" {
		if storageMode == store.StaticSiteStorageSingleFile {
			downloadName = path.Base(entryPath)
		} else if rootName != "" {
			downloadName = rootName
		} else {
			downloadName = "static-site"
		}
	}
	downloadName = sanitizeAttachmentName(downloadName, "static-site")

	return staticSiteUploadBundle{
		Files:        files,
		EntryPath:    entryPath,
		StorageMode:  storageMode,
		DownloadName: downloadName,
		FileCount:    len(files),
		TotalSize:    totalSize,
	}, nil
}

func resolveStaticSiteEntryPath(paths []string, requestedEntry string) (string, error) {
	fileSet := make(map[string]struct{}, len(paths))
	htmlPaths := make([]string, 0)
	for _, item := range paths {
		fileSet[item] = struct{}{}
		if isHTMLFilePath(item) {
			htmlPaths = append(htmlPaths, item)
		}
	}
	if len(htmlPaths) == 0 {
		return "", fmt.Errorf("no html file found in upload")
	}
	if strings.TrimSpace(requestedEntry) != "" {
		entryPath, err := cleanStaticSiteRelativePath(requestedEntry)
		if err != nil {
			return "", fmt.Errorf("invalid entry path")
		}
		if !isHTMLFilePath(entryPath) {
			return "", fmt.Errorf("entry path must point to an html file")
		}
		if _, ok := fileSet[entryPath]; !ok {
			return "", fmt.Errorf("entry path not found in uploaded files")
		}
		return entryPath, nil
	}
	if _, ok := fileSet["index.html"]; ok {
		return "index.html", nil
	}
	if _, ok := fileSet["index.htm"]; ok {
		return "index.htm", nil
	}
	if len(htmlPaths) == 1 {
		return htmlPaths[0], nil
	}
	sort.Strings(htmlPaths)
	return "", fmt.Errorf("multiple html files found, set entry path: %s", strings.Join(htmlPaths, ", "))
}

func cleanStaticSiteRelativePath(input string) (string, error) {
	value := strings.TrimSpace(strings.ReplaceAll(input, "\\", "/"))
	for strings.HasPrefix(value, "./") {
		value = strings.TrimPrefix(value, "./")
	}
	if value == "" || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("invalid path")
	}
	parts := strings.Split(value, "/")
	cleanedParts := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			return "", fmt.Errorf("invalid path")
		}
		if strings.ContainsRune(part, 0) {
			return "", fmt.Errorf("invalid path")
		}
		cleanedParts = append(cleanedParts, part)
	}
	if len(cleanedParts) == 0 {
		return "", fmt.Errorf("invalid path")
	}
	return strings.Join(cleanedParts, "/"), nil
}

func stripCommonUploadRoot(paths []string) ([]string, string) {
	if len(paths) == 0 {
		return paths, ""
	}
	rootName := ""
	stripped := make([]string, len(paths))
	for i, item := range paths {
		parts := strings.SplitN(item, "/", 2)
		if len(parts) < 2 {
			return paths, ""
		}
		if i == 0 {
			rootName = parts[0]
		} else if parts[0] != rootName {
			return paths, ""
		}
		stripped[i] = parts[1]
	}
	return stripped, rootName
}

func resolveStaticSiteFSPath(rootDir string, relativePath string) (string, error) {
	baseDir, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}
	fullPath := filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(relativePath)))
	if fullPath == baseDir {
		return "", fmt.Errorf("invalid static site path")
	}
	if !strings.HasPrefix(fullPath, baseDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid static site path")
	}
	return fullPath, nil
}

func setStaticSiteResponseHeaders(w http.ResponseWriter, relativePath string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if isHTMLFilePath(relativePath) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Security-Policy", "sandbox allow-scripts allow-forms allow-popups allow-modals allow-downloads allow-top-navigation-by-user-activation")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(), payment=(), usb=()")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
}

func writeStaticSiteZip(w io.Writer, siteDir string, archiveRoot string) error {
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	rootName := sanitizeAttachmentName(archiveRoot, "static-site")
	return filepath.Walk(siteDir, func(fullPath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		relativePath, err := filepath.Rel(siteDir, fullPath)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Method = zip.Deflate
		header.Name = path.Join(rootName, filepath.ToSlash(relativePath))
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(fullPath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		_ = file.Close()
		return copyErr
	})
}

func isHTMLFilePath(value string) bool {
	switch strings.ToLower(path.Ext(value)) {
	case ".html", ".htm":
		return true
	default:
		return false
	}
}

func sanitizeAttachmentName(input string, fallback string) string {
	value := strings.TrimSpace(strings.ReplaceAll(input, "\\", "/"))
	value = path.Base(value)
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\n", ""), "\r", ""))
	if value == "" || value == "." || value == "/" {
		return fallback
	}
	return value
}

func firstMultipartValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
