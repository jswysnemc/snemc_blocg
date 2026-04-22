package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/snemc/snemc-blog/internal/store"
)

func (a *App) handleAdminAgentKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.store.ListAgentAPIKeys(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (a *App) handleAdminCreateAgentKey(w http.ResponseWriter, r *http.Request) {
	type payload struct {
		Name string `json:"name"`
	}
	var req payload
	if err := a.decodeJSON(r, &req); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	key, rawKey, err := a.store.CreateAgentAPIKey(r.Context(), req.Name)
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusCreated, map[string]any{
		"key":     key,
		"raw_key": rawKey,
	})
}

func (a *App) handleAdminRevokeAgentKey(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := a.store.RevokeAgentAPIKey(r.Context(), id); err != nil {
		if err == store.ErrNotFound {
			a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "agent key not found"})
			return
		}
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleAgentSkills(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(a.agentSkillDocument()))
}

func (a *App) handleAgentPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := a.store.ListAdminPosts(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, map[string]any{"posts": posts})
}

func (a *App) handleAgentGetPost(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	post, err := a.store.GetAdminPost(r.Context(), id)
	if err == store.ErrNotFound {
		a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "post not found"})
		return
	}
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, post)
}

func (a *App) handleAgentCreatePost(w http.ResponseWriter, r *http.Request) {
	input, err := decodeAgentPostInput(r)
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	input.ID = 0
	post, err := a.store.SavePost(r.Context(), input)
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	a.invalidateContentCache(post.Slug)
	a.scheduleSemanticIndex(post.ID)
	a.respondJSON(w, http.StatusCreated, post)
}

func (a *App) handleAgentUpdatePost(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := a.store.GetAdminPost(r.Context(), id); err == store.ErrNotFound {
		a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "post not found"})
		return
	} else if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	input, err := decodeAgentPostInput(r)
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	input.ID = id
	post, err := a.store.SavePost(r.Context(), input)
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	a.invalidateContentCache(post.Slug)
	a.scheduleSemanticIndex(post.ID)
	a.respondJSON(w, http.StatusOK, post)
}

func (a *App) handleAgentPatchPost(w http.ResponseWriter, r *http.Request) {
	type patchPayload struct {
		Title        *string   `json:"title"`
		Summary      *string   `json:"summary"`
		Markdown     *string   `json:"markdown"`
		CoverImage   *string   `json:"cover_image"`
		Status       *string   `json:"status"`
		CategoryName *string   `json:"category_name"`
		Tags         *[]string `json:"tags"`
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	existing, err := a.store.GetAdminPost(r.Context(), id)
	if err == store.ErrNotFound {
		a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "post not found"})
		return
	}
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var patch patchPayload
	if err := a.decodeJSON(r, &patch); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	input := store.PostInput{
		ID:           existing.ID,
		Title:        existing.Title,
		Summary:      existing.Summary,
		Markdown:     existing.MarkdownSource,
		CoverImage:   existing.CoverImage,
		Status:       existing.Status,
		CategoryName: existing.CategoryName,
		Tags:         tagNames(existing.Tags),
	}
	if patch.Title != nil {
		input.Title = strings.TrimSpace(*patch.Title)
	}
	if patch.Summary != nil {
		input.Summary = *patch.Summary
	}
	if patch.Markdown != nil {
		input.Markdown = *patch.Markdown
	}
	if patch.CoverImage != nil {
		input.CoverImage = *patch.CoverImage
	}
	if patch.Status != nil {
		input.Status = *patch.Status
	}
	if patch.CategoryName != nil {
		input.CategoryName = *patch.CategoryName
	}
	if patch.Tags != nil {
		input.Tags = *patch.Tags
	}

	post, err := a.store.SavePost(r.Context(), input)
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	a.invalidateContentCache(post.Slug)
	a.scheduleSemanticIndex(post.ID)
	a.respondJSON(w, http.StatusOK, post)
}

func (a *App) handleAgentStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.store.Dashboard(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, stats)
}

func (a *App) handleAgentTaxonomies(w http.ResponseWriter, r *http.Request) {
	bundle, err := a.store.GetTaxonomies(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, bundle)
}

func (a *App) handleAgentCreateCategory(w http.ResponseWriter, r *http.Request) {
	type payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	var req payload
	if err := a.decodeJSON(r, &req); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	category, err := a.store.SaveCategory(r.Context(), req.Name, req.Description)
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	a.taxonomyCache.Delete("all")
	a.respondJSON(w, http.StatusCreated, category)
}

func (a *App) handleAgentUpdateCategory(w http.ResponseWriter, r *http.Request) {
	type payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req payload
	if err := a.decodeJSON(r, &req); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	category, err := a.store.UpdateCategory(r.Context(), id, req.Name, req.Description)
	if err == store.ErrNotFound {
		a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "category not found"})
		return
	}
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	a.taxonomyCache.Delete("all")
	a.respondJSON(w, http.StatusOK, category)
}

func (a *App) handleAgentDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := a.store.DeleteCategory(r.Context(), id); err != nil {
		if err == store.ErrNotFound {
			a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "category not found"})
			return
		}
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.taxonomyCache.Delete("all")
	a.respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleAgentCreateTag(w http.ResponseWriter, r *http.Request) {
	type payload struct {
		Name string `json:"name"`
	}
	var req payload
	if err := a.decodeJSON(r, &req); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	tag, err := a.store.SaveTag(r.Context(), req.Name)
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	a.taxonomyCache.Delete("all")
	a.respondJSON(w, http.StatusCreated, tag)
}

func (a *App) handleAgentUpdateTag(w http.ResponseWriter, r *http.Request) {
	type payload struct {
		Name string `json:"name"`
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req payload
	if err := a.decodeJSON(r, &req); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	tag, err := a.store.UpdateTag(r.Context(), id, req.Name)
	if err == store.ErrNotFound {
		a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "tag not found"})
		return
	}
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	a.taxonomyCache.Delete("all")
	a.respondJSON(w, http.StatusOK, tag)
}

func (a *App) handleAgentDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := a.store.DeleteTag(r.Context(), id); err != nil {
		if err == store.ErrNotFound {
			a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "tag not found"})
			return
		}
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.taxonomyCache.Delete("all")
	a.respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func decodeAgentPostInput(r *http.Request) (store.PostInput, error) {
	var input store.PostInput
	if err := jsonDecodeStrict(r, &input); err != nil {
		return store.PostInput{}, err
	}
	return input, nil
}

func jsonDecodeStrict(r *http.Request, dest any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dest)
}

func tagNames(tags []store.Tag) []string {
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		result = append(result, tag.Name)
	}
	return result
}

func (a *App) agentSkillDocument() string {
	baseURL := strings.TrimRight(a.cfg.SiteURL, "/")
	bt := "`"
	codeFence := "```"
	document := fmt.Sprintf(`---
name: snemc-blog-agent
description: Use this skill when an agent needs to read or operate the Snemc Blog through its public agent API, including posts, taxonomies, stats, and controlled content updates.
---

# snemc-blog-agent

Operate the Snemc Blog through HTTP APIs exposed by the blog server.

## Site

- Site name: %s
- Site URL: %s
- Skills document endpoint: %s/api/agent/skills
- Markdown alias: %s/api/agent/skills.md

## When to Use This Skill

Use this skill when you need to:

- Read all blog posts as summaries
- Fetch a full post including raw Markdown
- Create a new blog post
- Update an existing post fully or incrementally
- Read dashboard statistics
- Read, create, update, or delete categories and tags

## Authentication

- {{BT}}GET /api/agent/skills{{BT}} and {{BT}}GET /api/agent/skills.md{{BT}} do not require authentication
- All other {{BT}}/api/agent/*{{BT}} endpoints require an agent key
- Send the key as {{BT}}Authorization: Bearer <agent_key>{{BT}}
- Create or revoke the key in the admin settings page

## Blog Model Notes

- Public post URLs use a system-generated route id stored in {{BT}}slug{{BT}}
- For agent operations, prefer post {{BT}}id{{BT}} instead of {{BT}}slug{{BT}}
- {{BT}}PATCH /api/agent/posts/{id}{{BT}} only updates fields explicitly present in the request body

## Recommended Workflow

1. Call {{BT}}GET /api/agent/posts{{BT}} to discover the target post id.
2. Call {{BT}}GET /api/agent/posts/{id}{{BT}} before editing to read the current Markdown, tags, category, and status.
3. Use {{BT}}PATCH /api/agent/posts/{id}{{BT}} for small changes.
4. Use {{BT}}PUT /api/agent/posts/{id}{{BT}} when replacing the full post source.
5. Refresh taxonomies with {{BT}}GET /api/agent/taxonomies{{BT}} before assigning categories or tags.

## Endpoints

### Posts

| Method | Path | Purpose |
| --- | --- | --- |
| {{BT}}GET{{BT}} | {{BT}}/api/agent/posts{{BT}} | List all posts with summary fields |
| {{BT}}GET{{BT}} | {{BT}}/api/agent/posts/{id}{{BT}} | Get full post details including {{BT}}markdown_source{{BT}} |
| {{BT}}POST{{BT}} | {{BT}}/api/agent/posts{{BT}} | Create a new post |
| {{BT}}PUT{{BT}} | {{BT}}/api/agent/posts/{id}{{BT}} | Replace a post fully |
| {{BT}}PATCH{{BT}} | {{BT}}/api/agent/posts/{id}{{BT}} | Apply incremental updates |

### Taxonomies

| Method | Path | Purpose |
| --- | --- | --- |
| {{BT}}GET{{BT}} | {{BT}}/api/agent/taxonomies{{BT}} | List categories and tags |
| {{BT}}POST{{BT}} | {{BT}}/api/agent/categories{{BT}} | Create a category |
| {{BT}}PUT{{BT}} | {{BT}}/api/agent/categories/{id}{{BT}} | Update a category |
| {{BT}}DELETE{{BT}} | {{BT}}/api/agent/categories/{id}{{BT}} | Delete a category |
| {{BT}}POST{{BT}} | {{BT}}/api/agent/tags{{BT}} | Create a tag |
| {{BT}}PUT{{BT}} | {{BT}}/api/agent/tags/{id}{{BT}} | Update a tag |
| {{BT}}DELETE{{BT}} | {{BT}}/api/agent/tags/{id}{{BT}} | Delete a tag |

### Stats

| Method | Path | Purpose |
| --- | --- | --- |
| {{BT}}GET{{BT}} | {{BT}}/api/agent/stats{{BT}} | Read blog dashboard statistics |

## Post Payload

Use these fields when creating or updating a post:

{{CODE}}
{
  "title": "string",
  "summary": "string",
  "markdown": "string",
  "cover_image": "/uploads/example.webp",
  "status": "draft or published",
  "category_name": "Engineering",
  "tags": ["Go", "Vue3"]
}
{{CODE}}

## Incremental Update Example

{{CODE}}
curl -X PATCH %s/api/agent/posts/1 \
  -H "Authorization: Bearer <agent_key>" \
  -H "Content-Type: application/json" \
  -d '{
    "summary": "Updated summary",
    "tags": ["Go", "SQLite", "Agent"]
  }'
{{CODE}}

## Read Example

{{CODE}}
curl %s/api/agent/posts \
  -H "Authorization: Bearer <agent_key>"
{{CODE}}

## Error Handling

- {{BT}}401{{BT}} means the agent key is missing, invalid, or revoked
- {{BT}}404{{BT}} means the target resource does not exist
- {{BT}}400{{BT}} usually means the request body is invalid
- {{BT}}500{{BT}} means the blog server failed while processing the request

## Constraints

- The skills endpoint is documentation only; it does not perform operations
- Image upload for post content is still handled by the admin upload endpoint, not by the agent API
- Unknown fields are rejected on create and full update requests
`, a.cfg.SiteName, baseURL, baseURL, baseURL, baseURL, baseURL)
	return strings.NewReplacer(
		"{{BT}}", bt,
		"{{CODE}}", codeFence,
	).Replace(document)
}
