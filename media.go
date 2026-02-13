package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const MAX_UPLOAD_SIZE = 10 << 20 // 10 MB

type BunnyFile struct {
	Guid            string `json:"Guid"`
	StorageZoneName string `json:"StorageZoneName"`
	Path            string `json:"Path"`
	ObjectName      string `json:"ObjectName"`
	Length          int64  `json:"Length"`
	LastChanged     string `json:"LastChanged"`
	IsDirectory     bool   `json:"IsDirectory"`
	ServerId        int    `json:"ServerId"`
	UserId          string `json:"UserId"`
	DateCreated     string `json:"DateCreated"`
	StorageZoneId   int64  `json:"StorageZoneId"`
}

type BunnyConfig struct {
	StorageZone   string
	AccessKey     string
	StorageRegion string
	PullZoneURL   string
}

// BunnyClient handles API requests to Bunny.net
type BunnyClient struct {
	config BunnyConfig
	client *http.Client
}

// NewBunnyClient creates a new Bunny.net storage client
func NewBunnyClient() *BunnyClient {
	return &BunnyClient{
		config: BunnyConfig{
			StorageZone:   os.Getenv("STORAGE_ZONE"),
			AccessKey:     os.Getenv("ACCESS_KEY"),
			StorageRegion: os.Getenv("REGION"),
			PullZoneURL:   os.Getenv("PULL_ZONE_URL"),
		},
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (app *App) handleAdminMedia(w http.ResponseWriter, r *http.Request) {
	client := NewBunnyClient()
	files, err := client.GetAllFilesRecursively("")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type FileDisplay struct {
		BunnyFile
		FormattedSize string
	}

	var displayFiles []FileDisplay
	var totalSize int64
	var totalDirs, totalFiles int

	for _, file := range files {
		if file.IsDirectory {
			totalDirs++
		} else {
			totalFiles++
			totalSize += file.Length
		}

		displayFiles = append(displayFiles, FileDisplay{
			BunnyFile:     file,
			FormattedSize: formatBytes(file.Length),
		})
	}

	// Sort by created date (newest first)
	sort.Slice(displayFiles, func(i, j int) bool {
		return displayFiles[i].DateCreated > displayFiles[j].DateCreated
	})

	data := map[string]any{
		"Files":            displayFiles,
		"TotalItems":       len(files),
		"TotalDirectories": totalDirs,
		"TotalFiles":       totalFiles,
		"TotalSize":        formatBytes(totalSize),
		"CSRFToken":        app.csrfToken,
	}

	err = app.templates["admin_media.html"].ExecuteTemplate(w, "admin_base", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (app *App) handleNewMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		data := map[string]any{
			"CSRFToken": app.csrfToken,
		}

		err := app.templates["admin_media_form.html"].ExecuteTemplate(w, "admin_base", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	bunnyClient := NewBunnyClient()

	// Limit upload size
	r.Body = http.MaxBytesReader(w, r.Body, MAX_UPLOAD_SIZE)

	// Parse multipart form
	err := r.ParseMultipartForm(MAX_UPLOAD_SIZE)
	if err != nil {
		log.Printf("Upload error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get the file from the form
	file, header, err := r.FormFile("image")
	if err != nil {
		log.Printf("Upload error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Generate unique filename
	uniqueFilename := generateUniqueFilename(header.Filename)

	// Upload to Bunny.net
	_, err = uploadToBunny(bunnyClient, uniqueFilename, file)
	if err != nil {
		log.Printf("Upload error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Success response
	fileSize := fmt.Sprintf("%.2f KB", float64(header.Size)/1024)
	log.Printf("Successfully uploaded: %s (%s)", uniqueFilename, fileSize)

	http.Redirect(w, r, "/admin/media", http.StatusSeeOther)
}

// uploadImageToBunny uploads an image to BunnyCDN storage and returns the public CDN URL
func uploadToBunny(bc *BunnyClient, filename string, file io.Reader) (string, error) {

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
	apiURL := fmt.Sprintf("https://%s.storage.bunnycdn.com/%s/%s",
		bc.config.StorageRegion,
		bc.config.StorageZone,
		remotePath,
	)

	// Create the PUT request
	req, err := http.NewRequest("PUT", apiURL, bytes.NewReader(imageData))
	if err != nil {
		return "", fmt.Errorf("Failed to create request: %w", err)
	}

	// Set required headers
	req.Header.Set("AccessKey", bc.config.AccessKey)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Checksum", checksum)

	resp, err := bc.client.Do(req)
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
	cdnURL := fmt.Sprintf("%s/%s", bc.config.PullZoneURL, remotePath)

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

// formatBytes converts bytes to human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ListFiles retrieves files from a specific path in the storage zone
func (bc *BunnyClient) ListFiles(folderPath string) ([]BunnyFile, error) {
	// Construct the URL
	url := fmt.Sprintf("https://%s.storage.bunnycdn.com/%s/%s", bc.config.StorageRegion, bc.config.StorageZone, folderPath)

	// Create the request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add the AccessKey header
	req.Header.Add("AccessKey", bc.config.AccessKey)
	req.Header.Add("Accept", "application/json")

	// Execute the request
	resp, err := bc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var files []BunnyFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return files, nil
}

// GetAllFilesRecursively fetches all files recursively from the storage zone
func (bc *BunnyClient) GetAllFilesRecursively(startPath string) ([]BunnyFile, error) {
	var allFiles []BunnyFile

	// Helper function for recursive traversal
	var traverse func(currentPath string) error
	traverse = func(currentPath string) error {
		files, err := bc.ListFiles(currentPath)
		if err != nil {
			return err
		}

		for _, file := range files {
			// Add the file to our collection
			allFiles = append(allFiles, file)

			// If it's a directory, recursively fetch its contents
			if file.IsDirectory {
				// Construct the subdirectory path
				subPath := path.Join(currentPath, file.ObjectName) + "/"
				if err := traverse(subPath); err != nil {
					return err
				}
			}
		}

		return nil
	}

	// Start the recursive traversal
	if err := traverse(startPath); err != nil {
		return nil, err
	}

	return allFiles, nil
}
