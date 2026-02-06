package main

import (
	"encoding/xml"
	"time"
)

type URLSet struct {
	XMLName xml.Name `xml:"urlset"`
	XMLNS   string   `xml:"xmlns,attr"`
	URLs    []URL    `xml:"url"`
}

type URL struct {
	Loc        string  `xml:"loc"`
	LastMod    string  `xml:"lastmod,omitempty"`
	ChangeFreq string  `xml:"changefreq,omitempty"`
	Priority   float64 `xml:"priority,omitempty"`
}

func (app *App) generateSitemap(baseURL string) (*URLSet, error) {
	urls := []URL{
		{
			Loc:        baseURL,
			ChangeFreq: "daily",
			Priority:   1.0,
		},
		{
			Loc:        baseURL + "/tags",
			ChangeFreq: "weekly",
			Priority:   0.8,
		},
		{
			Loc:        baseURL + "/now",
			ChangeFreq: "monthly",
			Priority:   0.6,
		},
	}

	// Add all published posts
	postRows, err := app.db.Query(`
        SELECT slug, post_type, updated_at 
        FROM posts 
        WHERE published = 1 
        ORDER BY updated_at DESC
    `)
	if err != nil {
		return nil, err
	}
	defer postRows.Close()

	for postRows.Next() {
		var slug, postType string
		var updatedAt time.Time

		if err := postRows.Scan(&slug, &postType, &updatedAt); err != nil {
			continue
		}

		urls = append(urls, URL{
			Loc:        baseURL + "/" + postType + "s/" + slug,
			LastMod:    updatedAt.Format("2006-01-02"),
			ChangeFreq: "monthly",
			Priority:   0.7,
		})
	}

	// Add all published pages
	pageRows, err := app.db.Query(`
        SELECT slug, updated_at 
        FROM pages 
        WHERE published = 1 
        ORDER BY updated_at DESC
    `)
	if err != nil {
		return nil, err
	}
	defer pageRows.Close()

	for pageRows.Next() {
		var slug string
		var updatedAt time.Time

		if err := pageRows.Scan(&slug, &updatedAt); err != nil {
			continue
		}

		urls = append(urls, URL{
			Loc:        baseURL + "/" + slug,
			LastMod:    updatedAt.Format("2006-01-02"),
			ChangeFreq: "monthly",
			Priority:   0.6,
		})
	}

	// Add post type listing pages
	postTypes := []string{"articles", "notes", "links", "photos"}
	for _, pt := range postTypes {
		urls = append(urls, URL{
			Loc:        baseURL + "/" + pt,
			ChangeFreq: "weekly",
			Priority:   0.8,
		})
	}

	// Add tag pages
	tagRows, err := app.db.Query(`SELECT DISTINCT name FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer tagRows.Close()

	for tagRows.Next() {
		var tagName string
		if err := tagRows.Scan(&tagName); err != nil {
			continue
		}

		urls = append(urls, URL{
			Loc:        baseURL + "/tags/" + tagName,
			ChangeFreq: "weekly",
			Priority:   0.5,
		})
	}

	sitemap := &URLSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}

	return sitemap, nil
}
