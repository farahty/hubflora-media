package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"

	"github.com/farahty/hubflora-media/internal/model"
)

// JobStatusResponse represents the status of an async job.
type JobStatusResponse struct {
	JobID        string `json:"jobId"`
	State        string `json:"state"`
	Progress     int    `json:"progress"`
	ProcessedOn  int64  `json:"processedOn,omitempty"`
	FinishedOn   int64  `json:"finishedOn,omitempty"`
	FailedReason string `json:"failedReason,omitempty"`
}

// Client is a Go SDK for the hubflora-media service.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New creates a new media service client.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

// Upload uploads a file to the media service.
func (c *Client) Upload(filename string, data []byte, contentType string, orgSlug string, generateVariants, async bool) (*model.UploadResponse, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, fmt.Errorf("failed to write file data: %w", err)
	}

	writer.WriteField("orgSlug", orgSlug)
	if generateVariants {
		writer.WriteField("generateVariants", "true")
	}
	if async {
		writer.WriteField("async", "true")
	}

	writer.Close()

	req, err := http.NewRequest("POST", c.baseURL+"/api/v1/media/upload", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Media-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	var result model.UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// Delete deletes a file and its variants from storage.
func (c *Client) Delete(objectKey, bucketName string) (*model.DeleteResponse, error) {
	payload, _ := json.Marshal(map[string]string{
		"objectKey":  objectKey,
		"bucketName": bucketName,
	})

	req, err := http.NewRequest("DELETE", c.baseURL+"/api/v1/media", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Media-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("delete request failed: %w", err)
	}
	defer resp.Body.Close()

	var result model.DeleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// CropOptions configures the crop request.
type CropOptions struct {
	ObjectKey          string  `json:"objectKey"`
	BucketName         string  `json:"bucketName"`
	X                  int     `json:"x"`
	Y                  int     `json:"y"`
	Width              int     `json:"width"`
	Height             int     `json:"height"`
	Rotate             float64 `json:"rotate,omitempty"`
	Scale              float64 `json:"scale,omitempty"`
	Quality            int     `json:"quality,omitempty"`
	Format             string  `json:"format,omitempty"`
	RegenerateVariants bool    `json:"regenerateVariants"`
	Async              bool    `json:"async"`
}

// Crop crops an image, replaces the original, and optionally regenerates variants.
func (c *Client) Crop(opts CropOptions) (*model.CropResponse, error) {
	payload, _ := json.Marshal(opts)

	req, err := http.NewRequest("POST", c.baseURL+"/api/v1/media/crop", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Media-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crop request failed: %w", err)
	}
	defer resp.Body.Close()

	var result model.CropResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// GetPresignedURL gets a pre-signed download URL.
func (c *Client) GetPresignedURL(objectKey, bucketName string, expirySec int) (*model.PresignedDownloadResponse, error) {
	params := url.Values{}
	params.Set("objectKey", objectKey)
	if bucketName != "" {
		params.Set("bucket", bucketName)
	}
	if expirySec > 0 {
		params.Set("expiry", fmt.Sprintf("%d", expirySec))
	}

	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/media/presign?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Media-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("presign request failed: %w", err)
	}
	defer resp.Body.Close()

	var result model.PresignedDownloadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// GetPresignedUploadURL gets a pre-signed upload URL.
func (c *Client) GetPresignedUploadURL(orgSlug, filename, mimeType string) (*model.PresignedUploadResponse, error) {
	payload, _ := json.Marshal(model.PresignedUploadRequest{
		OrgSlug:  orgSlug,
		Filename: filename,
		MimeType: mimeType,
	})

	req, err := http.NewRequest("POST", c.baseURL+"/api/v1/media/upload/presigned", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Media-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("presigned upload request failed: %w", err)
	}
	defer resp.Body.Close()

	var result model.PresignedUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// JobStatus checks the status of an async job.
func (c *Client) JobStatus(jobID string) (*JobStatusResponse, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/media/job/%s", c.baseURL, jobID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Media-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("job status request failed: %w", err)
	}
	defer resp.Body.Close()

	var result JobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// RegenerateVariants triggers async variant regeneration for an existing file.
func (c *Client) RegenerateVariants(objectKey, bucketName string) (string, error) {
	payload, _ := json.Marshal(map[string]string{
		"objectKey":  objectKey,
		"bucketName": bucketName,
	})

	req, err := http.NewRequest("POST", c.baseURL+"/api/v1/media/variants", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Media-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("variant regen request failed: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	return result["jobId"], nil
}

// DownloadFile downloads a file from the media service.
func (c *Client) DownloadFile(bucket, objectKey string) ([]byte, string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/media/download/%s/%s", c.baseURL, bucket, objectKey), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("X-Media-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body: %w", err)
	}

	return data, resp.Header.Get("Content-Type"), nil
}
