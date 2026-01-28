package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
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

func (app *App) getPostTags(postID int) []string {
	rows, err := app.db.Query(`
		SELECT t.name
		FROM tags t
		JOIN post_tags pt ON t.id = pt.tag_id
		WHERE pt.post_id = ?
		ORDER BY t.name asc
	`, postID)
	if err != nil {
		return []string{}
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			continue
		}
		tags = append(tags, tag)
	}
	return tags
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

/*
	func (app *App) handleNow(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimPrefix(r.URL.Path, "/")

		var page Page
		err := app.db.QueryRow(`
			SELECT TOP(1) p.id, p.title, p.slug, p.content, p.post_type, p.created_at
			FROM posts p
			JOIN post_tags pt ON p.id = pt.post_id
			JOIN tags t ON pt.tag_id = t.id
			WHERE t.name = ? AND p.published = 1
			ORDER BY p.created_at DESC
		`, now).Scan(&page.ID, &page.Title, &page.Slug, &page.Content, &page.CreatedAt)

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
*/
func (app *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		data := map[string]any{
			"CSRFToken": app.csrfToken,
		}
		err := app.templates["login.html"].ExecuteTemplate(w, "base", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	if !app.validateCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	var user User
	err := app.db.QueryRow("SELECT id, username, password FROM users WHERE username = ?", username).
		Scan(&user.ID, &user.Username, &user.Password)

	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		data := map[string]any{
			"Error":     "Invalid credentials",
			"CSRFToken": app.csrfToken,
		}

		err = app.templates["login.html"].ExecuteTemplate(w, "base", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	token := generateToken()
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   86400 * 7,
		SameSite: http.SameSiteStrictMode,
	})

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (app *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "auth_token",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *App) handleAdmin(w http.ResponseWriter, r *http.Request) {
	var postCount, pageCount int
	app.db.QueryRow("SELECT COUNT(*) FROM posts").Scan(&postCount)
	app.db.QueryRow("SELECT COUNT(*) FROM pages").Scan(&pageCount)

	data := map[string]any{
		"PostCount": postCount,
		"PageCount": pageCount,
		"CSRFToken": app.csrfToken,
	}

	err := app.templates["admin.html"].ExecuteTemplate(w, "admin_base", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (app *App) handleAdminPosts(w http.ResponseWriter, r *http.Request) {
	rows, err := app.db.Query(`
		SELECT id, title, slug, post_type, published, created_at, updated_at
		FROM posts
		ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.PostType, &p.Published, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		p.Tags = app.getPostTags(p.ID)
		posts = append(posts, p)
	}

	data := map[string]any{
		"Posts":     posts,
		"CSRFToken": app.csrfToken,
	}

	err = app.templates["admin_posts.html"].ExecuteTemplate(w, "admin_base", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (app *App) handleNewPost(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		data := map[string]any{
			"CSRFToken": app.csrfToken,
		}

		err := app.templates["admin_post_form.html"].ExecuteTemplate(w, "admin_base", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	if !app.validateCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	title := r.FormValue("title")
	slug := r.FormValue("slug")
	content := r.FormValue("content")
	postType := r.FormValue("post_type")
	published := r.FormValue("published") == "on"
	tags := r.FormValue("tags")

	result, err := app.db.Exec(`
		INSERT INTO posts (title, slug, content, post_type, published)
		VALUES (?, ?, ?, ?, ?)
	`, title, slug, content, postType, published)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	postID, _ := result.LastInsertId()
	app.updatePostTags(int(postID), tags)

	http.Redirect(w, r, "/admin/posts", http.StatusSeeOther)
}

func (app *App) handleEditPost(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.Atoi(idStr)

	if r.Method == "GET" {
		var post Post
		var tagsStr string
		err := app.db.QueryRow(`
			SELECT id, title, slug, content, post_type, published
			FROM posts
			WHERE id = ?
		`, id).Scan(&post.ID, &post.Title, &post.Slug, &post.Content, &post.PostType, &post.Published)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		tags := app.getPostTags(post.ID)
		tagsStr = strings.Join(tags, ", ")

		data := map[string]any{
			"Post":      post,
			"Tags":      tagsStr,
			"CSRFToken": app.csrfToken,
		}

		err = app.templates["admin_post_form.html"].ExecuteTemplate(w, "admin_base", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	if !app.validateCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	title := r.FormValue("title")
	slug := r.FormValue("slug")
	content := r.FormValue("content")
	postType := r.FormValue("post_type")
	published := r.FormValue("published") == "on"
	tags := r.FormValue("tags")

	_, err := app.db.Exec(`
		UPDATE posts
		SET title = ?, slug = ?, content = ?, post_type = ?, published = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, title, slug, content, postType, published, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.updatePostTags(id, tags)

	http.Redirect(w, r, "/admin/posts", http.StatusSeeOther)
}

func (app *App) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !app.validateCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	idStr := r.FormValue("id")
	id, _ := strconv.Atoi(idStr)

	_, err := app.db.Exec("DELETE FROM posts WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/posts", http.StatusSeeOther)
}

func (app *App) handleAdminPages(w http.ResponseWriter, r *http.Request) {
	rows, err := app.db.Query(`
		SELECT id, title, slug, published, created_at, updated_at
		FROM pages
		ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var pages []Page
	for rows.Next() {
		var p Page
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.Published, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		pages = append(pages, p)
	}

	data := map[string]any{
		"Pages":     pages,
		"CSRFToken": app.csrfToken,
	}

	err = app.templates["admin_pages.html"].ExecuteTemplate(w, "admin_base", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (app *App) handleNewPage(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		data := map[string]any{
			"CSRFToken": app.csrfToken,
		}

		err := app.templates["admin_page_form.html"].ExecuteTemplate(w, "admin_base", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	if !app.validateCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	title := r.FormValue("title")
	slug := r.FormValue("slug")
	content := r.FormValue("content")
	published := r.FormValue("published") == "on"

	_, err := app.db.Exec(`
		INSERT INTO pages (title, slug, content, published)
		VALUES (?, ?, ?, ?)
	`, title, slug, content, published)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/pages", http.StatusSeeOther)
}

func (app *App) handleEditPage(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.Atoi(idStr)

	if r.Method == "GET" {
		var page Page
		err := app.db.QueryRow(`
			SELECT id, title, slug, content, published
			FROM pages
			WHERE id = ?
		`, id).Scan(&page.ID, &page.Title, &page.Slug, &page.Content, &page.Published)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		data := map[string]any{
			"Page":      page,
			"CSRFToken": app.csrfToken,
		}

		err = app.templates["admin_page_form.html"].ExecuteTemplate(w, "admin_base", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	if !app.validateCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	title := r.FormValue("title")
	slug := r.FormValue("slug")
	content := r.FormValue("content")
	published := r.FormValue("published") == "on"

	_, err := app.db.Exec(`
		UPDATE pages
		SET title = ?, slug = ?, content = ?, published = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, title, slug, content, published, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/pages", http.StatusSeeOther)
}

func (app *App) handleDeletePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !app.validateCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	idStr := r.FormValue("id")
	id, _ := strconv.Atoi(idStr)

	_, err := app.db.Exec("DELETE FROM pages WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/pages", http.StatusSeeOther)
}

func (app *App) updatePostTags(postID int, tagsStr string) {
	app.db.Exec("DELETE FROM post_tags WHERE post_id = ?", postID)

	if tagsStr == "" {
		return
	}

	tags := strings.Split(tagsStr, ",")
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}

		var tagID int
		err := app.db.QueryRow("SELECT id FROM tags WHERE name = ?", tag).Scan(&tagID)
		if err == sql.ErrNoRows {
			result, _ := app.db.Exec("INSERT INTO tags (name) VALUES (?)", tag)
			id, _ := result.LastInsertId()
			tagID = int(id)
		}

		app.db.Exec("INSERT INTO post_tags (post_id, tag_id) VALUES (?, ?)", postID, tagID)
	}
}
