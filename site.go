package main

import (
	"database/sql"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

func (app *App) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		app.handlePage(w, r)
		return
	}

	rows, err := app.db.Query(`
		WITH ranked_posts AS (
			SELECT
				id,
				title,
				slug,
				content,
				post_type,
				created_at,
				ROW_NUMBER() OVER (PARTITION BY post_type ORDER BY created_at DESC) as rn
			FROM posts
			WHERE post_type IN ('essay', 'note')
		)
		SELECT id, title, slug, content, post_type, created_at 
		FROM ranked_posts
		WHERE rn <= 5
		ORDER BY post_type, rn
	`)
	if err != nil {
		app.httpError(w, err, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var essays []Post
	var notes []Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.Content, &p.PostType, &p.CreatedAt); err != nil {
			continue
		}
		p.HTMLContent = app.markdownToHTML(p.Content)
		p.Tags = app.getPostTags(p.ID)

		switch p.PostType {
		case "essay":
			essays = append(essays, p)
		case "note":
			notes = append(notes, p)
		}
	}

	data := map[string]any{
		"Essays":          essays,
		"Notes":           notes,
		"IsAuthenticated": app.isAuthenticated(r),
	}

	err = app.templates["home.html"].ExecuteTemplate(w, "base", data)
	if err != nil {
		app.httpError(w, err, http.StatusInternalServerError)
		return
	}
}

func (app *App) handlePage(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/")

	var page Page
	err := app.db.QueryRow(`
		SELECT id, title, slug, content, created_at 
		FROM pages 
		WHERE slug = ? AND published = 1
	`, slug).Scan(&page.ID, &page.Title, &page.Slug, &page.Content, &page.CreatedAt)

	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		app.httpError(w, err, http.StatusInternalServerError)
		return
	}

	page.HTMLContent = app.markdownToHTML(page.Content)

	data := map[string]any{
		"Page":            page,
		"IsAuthenticated": app.isAuthenticated(r),
	}

	err = app.templates["page.html"].ExecuteTemplate(w, "base", data)
	if err != nil {
		app.httpError(w, err, http.StatusInternalServerError)
		return
	}
}

func (app *App) handlePosts(postType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathPrefix := "/" + postType + "s/"
		slug := strings.TrimPrefix(r.URL.Path, pathPrefix)

		var post Post
		err := app.db.QueryRow(`
			SELECT id, title, slug, content, post_type, created_at, updated_at
			FROM posts
			WHERE slug = ? AND post_type = ? AND published = 1
		`, slug, postType).Scan(&post.ID, &post.Title, &post.Slug, &post.Content, &post.PostType, &post.CreatedAt, &post.UpdatedAt)

		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			app.httpError(w, err, http.StatusInternalServerError)
			return
		}

		post.HTMLContent = app.markdownToHTML(post.Content)
		post.Tags = app.getPostTags(post.ID)

		data := map[string]any{
			"Post":            post,
			"IsAuthenticated": app.isAuthenticated(r),
		}

		err = app.templates["post.html"].ExecuteTemplate(w, "base", data)
		if err != nil {
			app.httpError(w, err, http.StatusInternalServerError)
			return
		}
	}
}

func (app *App) handlePostsList(postType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := app.db.Query(`
			SELECT id, title, slug, content, post_type, created_at, updated_at
			FROM posts
			WHERE post_type = ? AND published = 1
			ORDER BY created_at DESC
		`, postType)
		if err != nil {
			app.httpError(w, err, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var posts []Post
		for rows.Next() {
			var p Post
			if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.Content, &p.PostType, &p.CreatedAt, &p.UpdatedAt); err != nil {
				continue
			}
			p.HTMLContent = app.markdownToHTML(p.Content)
			p.Tags = app.getPostTags(p.ID)
			posts = append(posts, p)
		}

		data := map[string]any{
			"Posts":           posts,
			"PostType":        titleCase(postType),
			"IsAuthenticated": app.isAuthenticated(r),
		}

		err = app.templates["post_list.html"].ExecuteTemplate(w, "base", data)
		if err != nil {
			app.httpError(w, err, http.StatusInternalServerError)
			return
		}
	}
}

func (app *App) handleTags(w http.ResponseWriter, r *http.Request) {
	rows, err := app.db.Query(`
		SELECT t.name, COUNT(pt.post_id) as count
		FROM tags t
		LEFT JOIN post_tags pt ON t.id = pt.tag_id
		GROUP BY t.id, t.name
		ORDER BY t.name
	`)
	if err != nil {
		app.httpError(w, err, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var tag Tag
		if err := rows.Scan(&tag.Name, &tag.Count); err != nil {
			continue
		}
		if tag.Count > 0 {
			tags = append(tags, tag)
		}
	}

	data := map[string]any{
		"Tags":            tags,
		"IsAuthenticated": app.isAuthenticated(r),
	}

	err = app.templates["tags.html"].ExecuteTemplate(w, "base", data)
	if err != nil {
		app.httpError(w, err, http.StatusInternalServerError)
		return
	}
}

func (app *App) handleTagPosts(w http.ResponseWriter, r *http.Request) {
	tagName := strings.TrimPrefix(r.URL.Path, "/tags/")

	rows, err := app.db.Query(`
		SELECT p.id, p.title, p.slug, p.content, p.post_type, p.created_at
		FROM posts p
		JOIN post_tags pt ON p.id = pt.post_id
		JOIN tags t ON pt.tag_id = t.id
		WHERE t.name = ? AND p.published = 1
		ORDER BY p.created_at DESC
	`, tagName)
	if err != nil {
		app.httpError(w, err, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.Content, &p.PostType, &p.CreatedAt); err != nil {
			continue
		}
		p.HTMLContent = app.markdownToHTML(p.Content)
		p.Tags = app.getPostTags(p.ID)
		posts = append(posts, p)
	}

	data := map[string]any{
		"Posts":           posts,
		"TagName":         tagName,
		"IsAuthenticated": app.isAuthenticated(r),
	}

	err = app.templates["tag_posts.html"].ExecuteTemplate(w, "base", data)
	if err != nil {
		app.httpError(w, err, http.StatusInternalServerError)
		return
	}
}

func (app *App) handleNow(w http.ResponseWriter, r *http.Request) {
	var now Post
	err := app.db.QueryRow(`
		SELECT p.id, p.title, p.slug, p.content, p.created_at, p.updated_at
		FROM posts p
		JOIN post_tags pt ON p.id = pt.post_id
		JOIN tags t ON pt.tag_id = t.id
		WHERE t.name = ? AND p.published = 1
		ORDER BY p.created_at DESC LIMIT 1
	`, "now").Scan(&now.ID, &now.Title, &now.Slug, &now.Content, &now.CreatedAt, &now.UpdatedAt)

	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		app.httpError(w, err, http.StatusInternalServerError)
		return
	}

	now.HTMLContent = app.markdownToHTML(now.Content)

	canonicalURL := fmt.Sprintf("%s/notes/%s", baseURL, now.Slug)

	data := map[string]any{
		"Page":            now,
		"CanonicalURL":    canonicalURL,
		"IsAuthenticated": app.isAuthenticated(r),
	}

	err = app.templates["now.html"].ExecuteTemplate(w, "base", data)
	if err != nil {
		app.httpError(w, err, http.StatusInternalServerError)
		return
	}
}

func (app *App) handleSitemap(w http.ResponseWriter, r *http.Request) {
	// Determine base URL
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	baseURL := scheme + "://" + r.Host

	sitemap, err := app.generateSitemap(baseURL)
	if err != nil {
		app.httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")

	output, err := xml.MarshalIndent(sitemap, "", "  ")
	if err != nil {
		app.httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.Write([]byte(xml.Header))
	w.Write(output)
}

func (app *App) handleRobotsTxt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	robotsTxt := `User-agent: *
Allow: /

Sitemap: ` + scheme + `://` + r.Host + `/sitemap.xml`

	w.Write([]byte(robotsTxt))
}
