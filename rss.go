package main

import (
	"encoding/xml"
	"net/http"
	"time"
)

type RSS struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel *Channel `xml:"channel"`
}

type Channel struct {
	Title         string `xml:"title"`
	Link          string `xml:"link"`
	Description   string `xml:"description"`
	Language      string `xml:"language,omitempty"`
	LastBuildDate string `xml:"lastBuildDate,omitempty"`
	Items         []Item `xml:"item"`
}

type Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

func (app *App) generateRSSFeed(postType, baseURL, title, description string) (*RSS, error) {
	var query string
	var args []any

	if postType == "" {
		// All posts
		query = `SELECT id, title, slug, content, post_type, created_at 
		         FROM posts WHERE published = 1 ORDER BY created_at DESC`
	} else {
		// Specific post type
		query = `SELECT id, title, slug, content, post_type, created_at 
		         FROM posts WHERE post_type = ? AND published = 1 
		         ORDER BY created_at DESC`
		args = append(args, postType)
	}

	rows, err := app.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	var lastBuildDate time.Time

	for rows.Next() {
		var id int
		var postTitle, slug, content, pType string
		var createdAt time.Time

		if err := rows.Scan(&id, &postTitle, &slug, &content, &pType, &createdAt); err != nil {
			continue
		}

		if createdAt.After(lastBuildDate) {
			lastBuildDate = createdAt
		}

		// Convert markdown to HTML for description
		htmlContent := app.markdownToHTML(content)

		// Truncate description to first 500 chars
		desc := string(htmlContent)
		if len(desc) > 500 {
			desc = desc[:500] + "..."
		}

		items = append(items, Item{
			Title:       postTitle,
			Link:        baseURL + "/" + pType + "s/" + slug,
			Description: desc,
			PubDate:     createdAt.Format(time.RFC1123Z),
			GUID:        baseURL + "/" + pType + "s/" + slug,
		})
	}

	feed := &RSS{
		Version: "2.0",
		Channel: &Channel{
			Title:         title,
			Link:          baseURL,
			Description:   description,
			Language:      "en-us",
			LastBuildDate: lastBuildDate.Format(time.RFC1123Z),
			Items:         items,
		},
	}

	return feed, nil
}

func (app *App) handleRSSFeed(w http.ResponseWriter, r *http.Request) {
	baseURL := "http://" + r.Host

	feed, err := app.generateRSSFeed("", baseURL, "My Blog", "Recent posts from my blog")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")

	output, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte(xml.Header))
	w.Write(output)
}

func (app *App) handlePostTypeRSS(postType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		baseURL := "http://" + r.Host

		title := "My Blog - " + postType + "s"
		description := "Recent " + postType + "s from my blog"

		feed, err := app.generateRSSFeed(postType, baseURL, title, description)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")

		output, err := xml.MarshalIndent(feed, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write([]byte(xml.Header))
		w.Write(output)
	}
}
