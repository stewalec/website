package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

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
			"Error":     "Invalid username or password",
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

// TODO: Change to POST request to follow spec
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
	idStr := r.PathValue("id")
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
	idStr := r.PathValue("id")
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
