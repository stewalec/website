package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const MAX_UPLOAD_SIZE = 10 << 20 // 10 MB

type BunnyConfig struct {
	StorageZone   string
	AccessKey     string
	StorageRegion string
	PullZoneURL   string
}

func (app *App) handleUpload(w http.ResponseWriter, r *http.Request) {

	config := BunnyConfig{
		StorageZone:   os.Getenv("STORAGEZONE"),
		AccessKey:     os.Getenv("ACCESSKEY"),
		StorageRegion: os.Getenv("REGION"),
		PullZoneURL:   os.Getenv("PULLEDZONEURL"),
	}

	// Limit upload size
	r.Body = http.MaxBytesReader(w, r.Body, MAX_UPLOAD_SIZE)

	// Parse multipart form
	err := r.ParseMultipartForm(MAX_UPLOAD_SIZE)
	if err != nil {
		data := map[string]any{
			"Error":     true,
			"Message":   fmt.Sprintf("File too large. Maximum size is %d MB", MAX_UPLOAD_SIZE/(1<<20)),
			"CSRFToken": app.csrfToken,
		}

		err = app.templates["admin.html"].ExecuteTemplate(w, "admin_base", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Get the file from the form
	file, header, err := r.FormFile("image")
	if err != nil {
		data := map[string]any{
			"Error":     true,
			"Message":   "No file uploaded or invalid file",
			"CSRFToken": app.csrfToken,
		}

		err = app.templates["admin.html"].ExecuteTemplate(w, "admin_base", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	defer file.Close()

	// Generate unique filename
	uniqueFilename := generateUniqueFilename(header.Filename)

	// Upload to Bunny.net
	url, err := uploadToBunny(config, uniqueFilename, file)
	if err != nil {
		log.Printf("Upload error: %v", err)
		data := map[string]any{
			"Error":     true,
			"Message":   fmt.Sprintf("Upload to Bunny.net failed: %v", err),
			"CSRFToken": app.csrfToken,
		}

		err = app.templates["admin.html"].ExecuteTemplate(w, "admin_base", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Success response
	fileSize := fmt.Sprintf("%.2f KB", float64(header.Size)/1024)

	log.Printf("Successfully uploaded: %s (%s)", uniqueFilename, fileSize)

	data := map[string]any{
		"Success":   true,
		"Message":   "Upload succcessful!",
		"URL":       url,
		"Filename":  uniqueFilename,
		"FileSize":  fileSize,
		"CSRFToken": app.csrfToken,
	}

	err = app.templates["admin.html"].ExecuteTemplate(w, "admin_base", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// uploadImageToBunny uploads an image to BunnyCDN storage and returns the public CDN URL
func uploadToBunny(config BunnyConfig, filename string, file io.Reader) (string, error) {

	// Read the data into a buffer so we can calculate checksum and upload
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, file); err != nil {
		return "", fmt.Errorf("failed to read image data: %w", err)
	}
	imageData := buf.Bytes()

	// Calculate SHA256 checksum for integrity verification
	hash := sha256.Sum256(imageData)
	checksum := hex.EncodeToString(hash[:])

	// Generate a unique path for the image
	now := time.Now()
	remotePath := fmt.Sprintf("%d/%s", now.Year(), filename)

	// Construct the storage API endpoint
	// Format: https://{region}.storage.bunnycdn.com/{storageZoneName}/{path}
	region := config.StorageRegion

	apiURL := fmt.Sprintf("https://%s.storage.bunnycdn.com/%s/%s",
		region,
		config.StorageZone,
		remotePath,
	)

	// Create the PUT request
	req, err := http.NewRequest("PUT", apiURL, bytes.NewReader(imageData))
	if err != nil {
		return "", fmt.Errorf("Failed to create request: %w", err)
	}

	// Set required headers
	req.Header.Set("AccessKey", config.AccessKey)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Checksum", checksum)

	// Execute the upload
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Failed to upload to BunnyCDN: %w", err)
	}
	defer resp.Body.Close()

	// Check for successful upload (201 Created)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Construct and return the public CDN URL
	cdnURL := fmt.Sprintf("%s/%s", config.PullZoneURL, remotePath)

	return cdnURL, nil
}

func generateUniqueFilename(originalFilename string) string {
	ext := filepath.Ext(originalFilename)
	nameWithoutExt := strings.TrimSuffix(originalFilename, ext)

	// Clean the filename (remove special characters)
	nameWithoutExt = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, nameWithoutExt)

	// Add timestamp to make it unique
	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s_%d%s", nameWithoutExt, timestamp, ext)
}
