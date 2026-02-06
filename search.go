package main

import (
	"html/template"
	"net/http"
	"sort"
	"strings"
)

type SearchResult struct {
	Type      string
	Post      *Post
	Page      *Page
	Rank      float64
	Snippet   template.HTML
	MatchInfo string
}

func (app *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

	if query == "" {
		data := map[string]any{
			"Query":   "",
			"Results": []SearchResult{},
			"Total":   0,
		}
		err := app.templates["search.html"].ExecuteTemplate(w, "base", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	// Prepare FTS query with prefix matching
	ftsQuery := prepareFTSQuery(query)

	var results []SearchResult

	// Search posts using FTS5
	// FTS5 uses BM25 ranking algorithm by default
	postRows, err := app.db.Query(`
		SELECT 
			p.id, 
			p.title, 
			p.slug, 
			p.content, 
			p.post_type, 
			p.created_at,
			fts.rank,
			snippet(posts_fts, 1, '<mark>', '</mark>', '...', 64) as snippet
		FROM posts p
		JOIN posts_fts fts ON p.id = fts.rowid
		WHERE posts_fts MATCH ? AND p.published = 1
		ORDER BY fts.rank
		LIMIT 50
	`, ftsQuery)

	if err == nil {
		defer postRows.Close()
		for postRows.Next() {
			var p Post
			var rank float64
			var snippet template.HTML

			if err := postRows.Scan(&p.ID, &p.Title, &p.Slug, &p.Content, &p.PostType, &p.CreatedAt, &rank, &snippet); err != nil {
				continue
			}

			p.Tags = app.getPostTags(p.ID)

			results = append(results, SearchResult{
				Type:    "post",
				Post:    &p,
				Rank:    rank,
				Snippet: snippet,
			})
		}
	}

	// Search pages using FTS5
	pageRows, err := app.db.Query(`
		SELECT 
			p.id, 
			p.title, 
			p.slug, 
			p.content, 
			p.created_at,
			fts.rank,
			snippet(pages_fts, 1, '<mark>', '</mark>', '...', 64) as snippet
		FROM pages p
		JOIN pages_fts fts ON p.id = fts.rowid
		WHERE pages_fts MATCH ? AND p.published = 1
		ORDER BY fts.rank
		LIMIT 50
	`, ftsQuery)

	if err == nil {
		defer pageRows.Close()
		for pageRows.Next() {
			var p Page
			var rank float64
			var snippet template.HTML

			if err := pageRows.Scan(&p.ID, &p.Title, &p.Slug, &p.Content, &p.CreatedAt, &rank, &snippet); err != nil {
				continue
			}

			results = append(results, SearchResult{
				Type:    "page",
				Page:    &p,
				Rank:    rank,
				Snippet: snippet,
			})
		}
	}

	// Sort all results by rank (lower rank = better match in FTS5)
	sortResultsByRank(results)

	data := map[string]any{
		"Query":   query,
		"Results": results,
		"Total":   len(results),
	}

	err = app.templates["search.html"].ExecuteTemplate(w, "base", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func sortResultsByRank(results []SearchResult) {
	// Lower rank is better (FTS5 rank is negative)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Rank < results[j].Rank
	})
}

// Prepare FTS query to support prefix matching
func prepareFTSQuery(query string) string {
	// Remove any existing wildcards to prevent injection
	query = strings.ReplaceAll(query, "*", "")
	query = strings.TrimSpace(query)

	if query == "" {
		return query
	}

	// Split into words
	words := strings.Fields(query)

	// Add prefix wildcard to each word
	for i, word := range words {
		// Don't add wildcard to operators
		if strings.ToUpper(word) == "AND" ||
			strings.ToUpper(word) == "OR" ||
			strings.ToUpper(word) == "NOT" {
			continue
		}

		// Don't add wildcard if it's a quoted phrase
		if strings.HasPrefix(word, `"`) && strings.HasSuffix(word, `"`) {
			continue
		}

		// Add prefix wildcard
		words[i] = word + "*"
	}

	return strings.Join(words, " ")
}
