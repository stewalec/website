package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

func (app *App) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		app.handleDynamicPage(w, r)
		return
	}

	rows, err := app.db.Query(`
		SELECT id, title, slug, content, post_type, created_at 
		FROM posts 
		WHERE published = 1
		AND post_type = 'article'
		ORDER BY created_at DESC 
		LIMIT 5
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		"IsAuthenticated": app.isAuthenticated(r),
	}

	err = app.templates["home.html"].ExecuteTemplate(w, "base", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (app *App) handleDynamicPage(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	page.HTMLContent = app.markdownToHTML(page.Content)

	data := map[string]any{
		"Page":            page,
		"IsAuthenticated": app.isAuthenticated(r),
	}

	err = app.templates["page.html"].ExecuteTemplate(w, "base", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (app *App) handlePostsByType(postType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathPrefix := "/" + postType + "s/"
		slug := strings.TrimPrefix(r.URL.Path, pathPrefix)

		if slug == "" {
			rows, err := app.db.Query(`
				SELECT id, title, slug, content, post_type, created_at
				FROM posts
				WHERE post_type = ? AND published = 1
				ORDER BY created_at DESC
			`, postType)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
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
				"PostType":        strings.ToUpper(string(postType[0])) + string(postType[1:]),
				"IsAuthenticated": app.isAuthenticated(r),
			}

			err = app.templates["post_list.html"].ExecuteTemplate(w, "base", data)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			return
		}

		var post Post
		err := app.db.QueryRow(`
			SELECT id, title, slug, content, post_type, created_at
			FROM posts
			WHERE slug = ? AND post_type = ? AND published = 1
		`, slug, postType).Scan(&post.ID, &post.Title, &post.Slug, &post.Content, &post.PostType, &post.CreatedAt)

		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var tag Tag
		if err := rows.Scan(&tag.Name, &tag.Count); err != nil {
			continue
		}
		tags = append(tags, tag)
	}

	data := map[string]any{
		"Tags":            tags,
		"IsAuthenticated": app.isAuthenticated(r),
	}

	err = app.templates["tags.html"].ExecuteTemplate(w, "base", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (app *App) handlePage(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/pages/")

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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	page.HTMLContent = app.markdownToHTML(page.Content)

	data := map[string]any{
		"Page":            page,
		"IsAuthenticated": app.isAuthenticated(r),
	}

	err = app.templates["page.html"].ExecuteTemplate(w, "base", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (app *App) handleNow(w http.ResponseWriter, r *http.Request) {
	var now Post
	err := app.db.QueryRow(`
		SELECT p.id, p.title, p.slug, p.content, p.created_at
		FROM posts p
		JOIN post_tags pt ON p.id = pt.post_id
		JOIN tags t ON pt.tag_id = t.id
		WHERE t.name = ? AND p.published = 1
		ORDER BY p.created_at DESC LIMIT 1
	`, "now").Scan(&now.ID, &now.Title, &now.Slug, &now.Content, &now.CreatedAt)

	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	now.HTMLContent = app.markdownToHTML(now.Content)

	canonicalURL := fmt.Sprintf("%s/notes/%s", host, now.Slug)

	data := map[string]any{
		"Page":            now,
		"CanonicalURL":    canonicalURL,
		"IsAuthenticated": app.isAuthenticated(r),
	}

	err = app.templates["page.html"].ExecuteTemplate(w, "base", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
