package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GarageClient talks to the Garage admin API (v2) over the Docker network.
type GarageClient struct {
	adminURL   string // e.g. "http://talos-svc-mysvc:3903"
	adminToken string
	httpClient *http.Client
}

// GarageBucketInfo represents a bucket from the Garage admin API.
type GarageBucketInfo struct {
	ID            string   `json:"id"`
	GlobalAliases []string `json:"globalAliases"`
	Created       string   `json:"created"`
	Objects       int64    `json:"objects,omitempty"`
	Bytes         int64    `json:"bytes,omitempty"`
}

// GarageKeyInfo represents a key from the Garage admin API.
type GarageKeyInfo struct {
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	Name            string `json:"name"`
}

func NewGarageClient(adminURL, adminToken string) *GarageClient {
	return &GarageClient{
		adminURL:   adminURL,
		adminToken: adminToken,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ListBuckets returns all buckets known to the Garage cluster.
func (c *GarageClient) ListBuckets(ctx context.Context) ([]GarageBucketInfo, error) {
	var buckets []GarageBucketInfo
	if err := c.do(ctx, http.MethodGet, "/v2/ListBuckets", nil, &buckets); err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	return buckets, nil
}

// CreateBucket creates a new bucket with the given global alias.
func (c *GarageClient) CreateBucket(ctx context.Context, globalAlias string) (*GarageBucketInfo, error) {
	body := map[string]string{"globalAlias": globalAlias}
	var info GarageBucketInfo
	if err := c.do(ctx, http.MethodPost, "/v2/CreateBucket", body, &info); err != nil {
		return nil, fmt.Errorf("create bucket: %w", err)
	}
	return &info, nil
}

// DeleteBucket deletes a bucket by its ID.
func (c *GarageClient) DeleteBucket(ctx context.Context, bucketID string) error {
	if err := c.do(ctx, http.MethodPost, "/v2/DeleteBucket?id="+bucketID, nil, nil); err != nil {
		return fmt.Errorf("delete bucket: %w", err)
	}
	return nil
}

// CreateKey creates a new access key with the given name.
func (c *GarageClient) CreateKey(ctx context.Context, name string) (*GarageKeyInfo, error) {
	body := map[string]string{"name": name}
	var info GarageKeyInfo
	if err := c.do(ctx, http.MethodPost, "/v2/CreateKey", body, &info); err != nil {
		return nil, fmt.Errorf("create key: %w", err)
	}
	return &info, nil
}

// AllowKey grants full read/write/owner permissions on a bucket for the given access key.
func (c *GarageClient) AllowKey(ctx context.Context, bucketID, accessKeyID string) error {
	body := map[string]interface{}{
		"bucketId":    bucketID,
		"accessKeyId": accessKeyID,
		"permissions": map[string]bool{
			"read":  true,
			"write": true,
			"owner": true,
		},
	}
	if err := c.do(ctx, http.MethodPost, "/v2/AllowBucketKey", body, nil); err != nil {
		return fmt.Errorf("allow key: %w", err)
	}
	return nil
}

// do executes an HTTP request against the Garage admin API.
func (c *GarageClient) do(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.adminURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.adminToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("garage admin API unreachable: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("garage admin API returned %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}
