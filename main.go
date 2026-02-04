package main

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/base64"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	_ "modernc.org/sqlite"
)

//go:embed templates/*
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

var baseUrl = "http://localhost:8080"

type App struct {
	db        *sql.DB
	templates map[string]*template.Template
	csrfToken string
	markdown  goldmark.Markdown
}

type Post struct {
	ID          int
	Title       string
	Slug        string
	Content     string
	HTMLContent template.HTML
	PostType    string
	Published   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Tags        []string
}

type Page struct {
	ID          int
	Title       string
	Slug        string
	Content     string
	HTMLContent template.HTML
	Published   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type User struct {
	ID       int
	Username string
	Password string
}

type Tag struct {
	Name  string
	Count int
}

func main() {
	app := &App{}

	if err := app.initDB(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer app.db.Close()

	if err := app.runMigrations(); err != nil {
		log.Fatal("Failed to run migrations:", err)
	}

	if err := app.loadTemplates(); err != nil {
		log.Fatal("Failed to load templates:", err)
	}

	if err := app.createDefaultUser(); err != nil {
		log.Fatal("Failed to create default user:", err)
	}

	app.csrfToken = generateToken()
	app.initMarkdown()

	mux := http.NewServeMux()

	// Static files
	mux.Handle("GET /static/", http.FileServer(http.FS(staticFS)))

	// Public routes
	mux.HandleFunc("GET /", app.handleHome)
	mux.HandleFunc("GET /articles", app.handlePostsList("article"))
	mux.HandleFunc("GET /articles/{slug}", app.handlePosts("article"))
	mux.HandleFunc("GET /notes", app.handlePostsList("note"))
	mux.HandleFunc("GET /notes/{slug}", app.handlePosts("note"))
	mux.HandleFunc("GET /links", app.handlePostsList("link"))
	mux.HandleFunc("GET /links/{slug}", app.handlePosts("link"))
	mux.HandleFunc("GET /photos", app.handlePostsList("photo"))
	mux.HandleFunc("GET /photos/{slug}", app.handlePosts("photo"))
	mux.HandleFunc("GET /tags", app.handleTags)
	mux.HandleFunc("GET /tags/{slug}", app.handleTagPosts)
	mux.HandleFunc("GET /now", app.handleNow)

	// RSS feeds
	mux.HandleFunc("GET /feed.xml", app.handleRSSFeed)
	mux.HandleFunc("GET /articles/feed.xml", app.handlePostTypeRSS("article"))
	mux.HandleFunc("GET /notes/feed.xml", app.handlePostTypeRSS("note"))
	mux.HandleFunc("GET /links/feed.xml", app.handlePostTypeRSS("link"))
	mux.HandleFunc("GET /photos/feed.xml", app.handlePostTypeRSS("photo"))

	// Admin routes
	mux.HandleFunc("GET /login", app.handleLogin)
	mux.HandleFunc("POST /login", app.handleLogin)
	mux.HandleFunc("GET /logout", app.handleLogout)
	mux.HandleFunc("GET /admin", app.requireAuth(app.handleAdmin))
	mux.HandleFunc("GET /admin/posts", app.requireAuth(app.handleAdminPosts))
	mux.HandleFunc("GET /admin/posts/new", app.requireAuth(app.handleNewPost))
	mux.HandleFunc("POST /admin/posts/new", app.requireAuth(app.handleNewPost))
	mux.HandleFunc("GET /admin/posts/edit/{id}", app.requireAuth(app.handleEditPost))
	mux.HandleFunc("POST /admin/posts/edit/{id}", app.requireAuth(app.handleEditPost))
	mux.HandleFunc("POST /admin/posts/delete", app.requireAuth(app.handleDeletePost))
	mux.HandleFunc("GET /admin/pages", app.requireAuth(app.handleAdminPages))
	mux.HandleFunc("GET /admin/pages/new", app.requireAuth(app.handleNewPage))
	mux.HandleFunc("POST /admin/pages/new", app.requireAuth(app.handleNewPage))
	mux.HandleFunc("GET /admin/pages/edit/{id}", app.requireAuth(app.handleEditPage))
	mux.HandleFunc("POST /admin/pages/edit/{id}", app.requireAuth(app.handleEditPage))
	mux.HandleFunc("POST /admin/pages/delete", app.requireAuth(app.handleDeletePage))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func (app *App) loadTemplates() error {
	var err error
	app.templates = make(map[string]*template.Template)

	tmplFiles, err := templateFS.ReadDir("templates")
	if err != nil {
		return err
	}

	for _, tmpl := range tmplFiles {
		if tmpl.IsDir() {
			continue
		}

		patterns := []string{
			"templates/layouts/*.html",
			"templates/" + tmpl.Name(),
		}

		t, err := template.ParseFS(templateFS, patterns...)
		if err != nil {
			return err
		}
		app.templates[tmpl.Name()] = t
	}

	return err
}

func (app *App) initMarkdown() {
	app.markdown = goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Typographer,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
			html.WithUnsafe(),
		),
	)
}

func (app *App) markdownToHTML(md string) template.HTML {
	var buf strings.Builder
	if err := app.markdown.Convert([]byte(md), &buf); err != nil {
		return template.HTML("")
	}
	return template.HTML(buf.String())
}

func (app *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("auth_token")
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func (app *App) validateCSRF(r *http.Request) bool {
	token := r.FormValue("csrf_token")
	return token == app.csrfToken
}

func generateToken() string {
	b := make([]byte, 32)
	io.ReadFull(rand.Reader, b)
	return base64.URLEncoding.EncodeToString(b)
}

func (app *App) isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie("auth_token")
	return err == nil && cookie.Value != ""
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

func titleCase(str string) string {
	return strings.ToUpper(string(str[0])) + string(str[1:])
}
