package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/snemc/snemc-blog/internal/store"
)

const (
	searchModeKeyword  = "keyword"
	searchModeSemantic = "semantic"
)

func normalizeSearchMode(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case searchModeSemantic:
		return searchModeSemantic
	default:
		return searchModeKeyword
	}
}

func (a *App) executeSearch(ctx context.Context, query string, limit int, requestedMode string) (searchOutcome, error) {
	requestedMode = normalizeSearchMode(requestedMode)
	query = strings.TrimSpace(query)
	if query == "" {
		return searchOutcome{
			RequestedMode: requestedMode,
			ExecutedMode:  requestedMode,
			Results:       []store.SearchResult{},
		}, nil
	}
	cacheKey := requestedMode + ":" + strings.TrimSpace(query)
	return a.cachedSearch(ctx, cacheKey, func(ctx context.Context) (searchOutcome, error) {
		if requestedMode == searchModeSemantic {
			return a.executeSemanticSearch(ctx, query, limit)
		}
		results, err := a.store.SearchPublishedPostsKeyword(ctx, query, limit)
		if err != nil {
			return searchOutcome{}, err
		}
		return searchOutcome{
			RequestedMode: requestedMode,
			ExecutedMode:  searchModeKeyword,
			Results:       results,
		}, nil
	})
}

func (a *App) executeSemanticSearch(ctx context.Context, query string, limit int) (searchOutcome, error) {
	outcome := searchOutcome{
		RequestedMode: searchModeSemantic,
		ExecutedMode:  searchModeSemantic,
	}

	settings, err := a.appSettings(ctx)
	if err != nil {
		return outcome, err
	}
	if !settings.SemanticSearchEnabled || !a.embedder.Ready() {
		return a.keywordFallbackSearch(ctx, query, limit, "语义搜索尚未配置完成，已切换到关键词搜索。")
	}
	readyCount, err := a.store.CountReadySemanticIndexes(ctx)
	if err != nil {
		return outcome, err
	}
	if readyCount == 0 {
		return a.keywordFallbackSearch(ctx, query, limit, "语义索引正在构建，已先展示关键词搜索结果。")
	}

	embedding, err := a.embedder.EmbedText(ctx, query)
	if err != nil {
		log.Printf("semantic search embed failed for query %q: %v", query, err)
		return a.keywordFallbackSearch(ctx, query, limit, "语义服务暂时不可用，已切换到关键词搜索。")
	}
	recreated, err := a.store.EnsureSemanticVectorTable(ctx, len(embedding))
	if err != nil {
		return a.keywordFallbackSearch(ctx, query, limit, "语义索引暂时不可用，已切换到关键词搜索。")
	}
	if recreated {
		a.scheduleSemanticBackfill()
		return a.keywordFallbackSearch(ctx, query, limit, "语义索引已重建，正在回填数据，先展示关键词搜索结果。")
	}

	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return outcome, err
	}
	results, err := a.store.SearchPublishedPostsSemantic(ctx, query, string(embeddingJSON), limit)
	if err != nil {
		log.Printf("semantic search query failed for query %q: %v", query, err)
		if errors.Is(err, store.ErrSemanticSearchUnavailable) {
			return a.keywordFallbackSearch(ctx, query, limit, "语义搜索暂时不可用，已切换到关键词搜索。")
		}
		return a.keywordFallbackSearch(ctx, query, limit, "语义服务暂时不可用，已切换到关键词搜索。")
	}
	outcome.Results = results
	return outcome, nil
}

func (a *App) keywordFallbackSearch(ctx context.Context, query string, limit int, notice string) (searchOutcome, error) {
	results, err := a.store.SearchPublishedPostsKeyword(ctx, query, limit)
	if err != nil {
		return searchOutcome{}, err
	}
	return searchOutcome{
		RequestedMode: searchModeSemantic,
		ExecutedMode:  searchModeKeyword,
		Notice:        notice,
		Results:       results,
	}, nil
}

func (a *App) scheduleSemanticBackfill() {
	a.semanticBackfillMu.Lock()
	if a.semanticBackfillActive {
		a.semanticBackfillMu.Unlock()
		return
	}
	a.semanticBackfillActive = true
	a.semanticBackfillMu.Unlock()

	go func() {
		defer func() {
			a.semanticBackfillMu.Lock()
			a.semanticBackfillActive = false
			a.semanticBackfillMu.Unlock()
		}()
		a.backfillSemanticIndex()
	}()
}

func (a *App) scheduleSemanticIndex(postID int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := a.syncSemanticIndex(ctx, postID); err != nil {
			log.Printf("semantic index sync failed for post %d: %v", postID, err)
		}
	}()
}

func (a *App) backfillSemanticIndex() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	settings, err := a.appSettings(ctx)
	if err != nil {
		log.Printf("semantic backfill skipped: %v", err)
		return
	}
	if !settings.SemanticSearchEnabled || !a.embedder.Ready() {
		return
	}

	ids, err := a.store.ListPublishedPostIDs(ctx)
	if err != nil {
		log.Printf("semantic backfill list posts failed: %v", err)
		return
	}
	for _, id := range ids {
		if err := a.syncSemanticIndex(ctx, id); err != nil {
			log.Printf("semantic backfill post %d failed: %v", id, err)
		}
	}
}

func (a *App) syncSemanticIndex(ctx context.Context, postID int64) error {
	settings, err := a.appSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.SemanticSearchEnabled || !a.embedder.Ready() {
		return nil
	}

	source, err := a.store.GetSemanticPostSource(ctx, postID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if source.Status != "published" {
		return a.store.DeleteSemanticIndex(ctx, postID)
	}

	sourceText := store.BuildSemanticSourceText(source)
	if strings.TrimSpace(sourceText) == "" {
		return nil
	}
	contentHash := semanticContentHash(sourceText)
	record, err := a.store.GetSemanticIndexRecord(ctx, postID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if err == nil &&
		record.Status == "ready" &&
		record.ContentHash == contentHash &&
		record.EmbeddingModel == settings.EmbeddingModel &&
		(settings.EmbeddingDimensions == 0 || record.EmbeddingDimensions == settings.EmbeddingDimensions) {
		return nil
	}

	embedding, err := a.embedder.EmbedText(ctx, sourceText)
	if err != nil {
		_ = a.store.UpsertSemanticIndexRecord(ctx, store.SemanticIndexRecord{
			PostID:              postID,
			EmbeddingModel:      settings.EmbeddingModel,
			EmbeddingDimensions: settings.EmbeddingDimensions,
			ContentHash:         contentHash,
			SourceText:          sourceText,
			Status:              "failed",
			ErrorMessage:        err.Error(),
		})
		return err
	}

	recreated, err := a.store.EnsureSemanticVectorTable(ctx, len(embedding))
	if err != nil {
		_ = a.store.UpsertSemanticIndexRecord(ctx, store.SemanticIndexRecord{
			PostID:              postID,
			EmbeddingModel:      settings.EmbeddingModel,
			EmbeddingDimensions: len(embedding),
			ContentHash:         contentHash,
			SourceText:          sourceText,
			Status:              "failed",
			ErrorMessage:        err.Error(),
		})
		return err
	}

	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return err
	}
	if err := a.store.UpsertSemanticVector(ctx, postID, string(embeddingJSON)); err != nil {
		_ = a.store.UpsertSemanticIndexRecord(ctx, store.SemanticIndexRecord{
			PostID:              postID,
			EmbeddingModel:      settings.EmbeddingModel,
			EmbeddingDimensions: len(embedding),
			ContentHash:         contentHash,
			SourceText:          sourceText,
			Status:              "failed",
			ErrorMessage:        err.Error(),
		})
		return err
	}

	if err := a.store.UpsertSemanticIndexRecord(ctx, store.SemanticIndexRecord{
		PostID:              postID,
		EmbeddingModel:      settings.EmbeddingModel,
		EmbeddingDimensions: len(embedding),
		ContentHash:         contentHash,
		SourceText:          sourceText,
		Status:              "ready",
		ErrorMessage:        "",
	}); err != nil {
		return err
	}

	if recreated {
		a.scheduleSemanticBackfill()
	}
	return nil
}

func semanticContentHash(input string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(input)))
	return hex.EncodeToString(sum[:])
}
