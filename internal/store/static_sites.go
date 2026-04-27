package store

import (
	"context"
	"database/sql"
	"strings"
)

const (
	StaticSiteStorageEmpty      = "empty"
	StaticSiteStorageSingleFile = "single_file"
	StaticSiteStorageDirectory  = "directory"
)

type StaticSiteUploadState struct {
	EntryPath    string
	StorageMode  string
	DownloadName string
	FileCount    int
	TotalSize    int64
}

func normalizeStaticSiteStorageMode(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case StaticSiteStorageSingleFile:
		return StaticSiteStorageSingleFile
	case StaticSiteStorageDirectory:
		return StaticSiteStorageDirectory
	default:
		return StaticSiteStorageEmpty
	}
}

func (s *Store) ListStaticSites(ctx context.Context) ([]StaticSite, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, route_id, entry_path, storage_mode, download_name, file_count, total_size, created_at, updated_at
FROM static_sites
ORDER BY datetime(updated_at) DESC, id DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sites := make([]StaticSite, 0)
	for rows.Next() {
		item, err := scanStaticSite(rows)
		if err != nil {
			return nil, err
		}
		sites = append(sites, item)
	}
	return sites, rows.Err()
}

func (s *Store) ListPublicStaticSites(ctx context.Context, limit int) ([]StaticSite, error) {
	query := `
SELECT id, route_id, entry_path, storage_mode, download_name, file_count, total_size, created_at, updated_at
FROM static_sites
WHERE entry_path <> '' AND file_count > 0 AND storage_mode <> ?
ORDER BY datetime(updated_at) DESC, id DESC
`
	args := []any{StaticSiteStorageEmpty}
	if limit > 0 {
		query += `LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sites := make([]StaticSite, 0)
	for rows.Next() {
		item, err := scanStaticSite(rows)
		if err != nil {
			return nil, err
		}
		sites = append(sites, item)
	}
	return sites, rows.Err()
}

func (s *Store) CreateStaticSite(ctx context.Context) (StaticSite, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StaticSite{}, err
	}
	defer tx.Rollback()

	routeID, err := s.uniqueStaticSiteRouteIDTx(ctx, tx, 0)
	if err != nil {
		return StaticSite{}, err
	}
	res, err := tx.ExecContext(ctx, `
INSERT INTO static_sites (route_id, entry_path, storage_mode, download_name, file_count, total_size, updated_at)
VALUES (?, '', ?, '', 0, 0, CURRENT_TIMESTAMP)
`, routeID, StaticSiteStorageEmpty)
	if err != nil {
		return StaticSite{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return StaticSite{}, err
	}
	if err := tx.Commit(); err != nil {
		return StaticSite{}, err
	}
	return s.GetStaticSiteByID(ctx, id)
}

func (s *Store) GetStaticSiteByID(ctx context.Context, id int64) (StaticSite, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, route_id, entry_path, storage_mode, download_name, file_count, total_size, created_at, updated_at
FROM static_sites
WHERE id = ?
`, id)
	site, err := scanStaticSite(row)
	if errorsIsNoRows(err) {
		return StaticSite{}, ErrNotFound
	}
	return site, err
}

func (s *Store) GetStaticSiteByRouteID(ctx context.Context, routeID string) (StaticSite, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, route_id, entry_path, storage_mode, download_name, file_count, total_size, created_at, updated_at
FROM static_sites
WHERE route_id = ?
`, strings.TrimSpace(routeID))
	site, err := scanStaticSite(row)
	if errorsIsNoRows(err) {
		return StaticSite{}, ErrNotFound
	}
	return site, err
}

func (s *Store) UpdateStaticSiteUpload(ctx context.Context, id int64, input StaticSiteUploadState) (StaticSite, error) {
	input.EntryPath = strings.TrimSpace(input.EntryPath)
	input.DownloadName = strings.TrimSpace(input.DownloadName)
	input.StorageMode = normalizeStaticSiteStorageMode(input.StorageMode)
	if input.EntryPath == "" || input.DownloadName == "" || input.StorageMode == StaticSiteStorageEmpty || input.FileCount <= 0 || input.TotalSize <= 0 {
		return StaticSite{}, ErrInvalidInput
	}

	result, err := s.db.ExecContext(ctx, `
UPDATE static_sites
SET entry_path = ?, storage_mode = ?, download_name = ?, file_count = ?, total_size = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, input.EntryPath, input.StorageMode, input.DownloadName, input.FileCount, input.TotalSize, id)
	if err != nil {
		return StaticSite{}, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return StaticSite{}, err
	}
	if rowsAffected == 0 {
		return StaticSite{}, ErrNotFound
	}
	return s.GetStaticSiteByID(ctx, id)
}

func (s *Store) DeleteStaticSite(ctx context.Context, id int64) (StaticSite, error) {
	site, err := s.GetStaticSiteByID(ctx, id)
	if err != nil {
		return StaticSite{}, err
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM static_sites WHERE id = ?`, id)
	if err != nil {
		return StaticSite{}, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return StaticSite{}, err
	}
	if rowsAffected == 0 {
		return StaticSite{}, ErrNotFound
	}
	return site, nil
}

func (s *Store) uniqueStaticSiteRouteIDTx(ctx context.Context, tx *sql.Tx, excludeID int64) (string, error) {
	for i := 0; i < 8; i++ {
		candidate, err := generateRouteID()
		if err != nil {
			return "", err
		}

		var count int
		query := `SELECT COUNT(*) FROM static_sites WHERE route_id = ?`
		args := []any{candidate}
		if excludeID > 0 {
			query += ` AND id <> ?`
			args = append(args, excludeID)
		}
		if err := tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
	}
	return "", ErrInvalidInput
}

type staticSiteScanner interface {
	Scan(dest ...any) error
}

func scanStaticSite(scanner staticSiteScanner) (StaticSite, error) {
	var item StaticSite
	var createdAt sql.NullString
	var updatedAt sql.NullString
	err := scanner.Scan(
		&item.ID,
		&item.RouteID,
		&item.EntryPath,
		&item.StorageMode,
		&item.DownloadName,
		&item.FileCount,
		&item.TotalSize,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return StaticSite{}, err
	}
	item.StorageMode = normalizeStaticSiteStorageMode(item.StorageMode)
	item.CreatedAt = parseDBTime(createdAt)
	item.UpdatedAt = parseDBTime(updatedAt)
	return item, nil
}
