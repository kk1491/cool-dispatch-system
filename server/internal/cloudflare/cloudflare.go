// Package cloudflare 封装 Cloudflare Images 图床的上传与删除操作，
// 对外仅暴露 Client 接口，避免业务层直接依赖 Cloudflare API 细节。
package cloudflare

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// Client 是 Cloudflare Images 图床客户端，封装认证与 HTTP 请求细节。
type Client struct {
	// accountID 是 Cloudflare 账户 ID，用于拼接 API 请求路径。
	accountID string
	// apiToken 是 Cloudflare API Token，需具备 Images Write 权限。
	apiToken string
	// httpClient 是复用的 HTTP 客户端实例，带超时控制。
	httpClient *http.Client
}

// UploadResult 是图片上传成功后的返回结构。
type UploadResult struct {
	// ID 是 Cloudflare Images 分配的唯一图片标识，删除时需要此值。
	ID string `json:"id"`
	// URL 是图片的公开访问地址（delivery URL）。
	URL string `json:"url"`
}

// cloudflareAPIResponse 是 Cloudflare API 的通用响应外壳。
type cloudflareAPIResponse struct {
	// Success 表示 API 调用是否成功。
	Success bool `json:"success"`
	// Errors 是 Cloudflare 返回的错误列表。
	Errors []cloudflareAPIError `json:"errors"`
	// Result 是成功时返回的业务数据。
	Result json.RawMessage `json:"result"`
}

// cloudflareAPIError 是 Cloudflare API 错误条目。
type cloudflareAPIError struct {
	// Code 是错误代码。
	Code int `json:"code"`
	// Message 是错误描述文本。
	Message string `json:"message"`
}

// cloudflareImageResult 是 Cloudflare Images 上传成功后的 result 结构。
type cloudflareImageResult struct {
	// ID 是图片唯一标识。
	ID string `json:"id"`
	// Variants 是所有可用的图片变体 URL 列表。
	Variants []string `json:"variants"`
}

// NewClient 创建 Cloudflare Images 客户端实例。
// accountID 和 apiToken 不能为空，否则后续 API 调用会全部失败。
func NewClient(accountID, apiToken string) *Client {
	return &Client{
		accountID: strings.TrimSpace(accountID),
		apiToken:  strings.TrimSpace(apiToken),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsConfigured 判断客户端是否已完成必要配置（account_id + api_token 均非空）。
// 未配置时不应调用上传/删除方法，调用方应回退到本地存储或跳过图床操作。
func (c *Client) IsConfigured() bool {
	return c.accountID != "" && c.apiToken != ""
}

// UploadImage 将图片文件上传到 Cloudflare Images 图床。
// filename 是文件名（含扩展名），reader 是图片二进制流。
// 成功返回 UploadResult（包含图片 ID 和公开访问 URL），失败返回 error。
func (c *Client) UploadImage(filename string, reader io.Reader) (*UploadResult, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("cloudflare images client is not configured")
	}

	// 构造 multipart/form-data 请求体
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// 异步写入 multipart 数据，避免阻塞主协程
	errCh := make(chan error, 1)
	go func() {
		defer pw.Close()
		defer writer.Close()

		// 创建 file 字段，写入图片二进制数据
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			errCh <- fmt.Errorf("failed to create form file: %w", err)
			return
		}
		if _, err := io.Copy(part, reader); err != nil {
			errCh <- fmt.Errorf("failed to copy image data: %w", err)
			return
		}
		errCh <- nil
	}()

	// 拼接 Cloudflare Images V1 上传接口
	apiURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/images/v1", c.accountID)

	req, err := http.NewRequest(http.MethodPost, apiURL, pr)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	// 等待 multipart 写入完成，检查是否有写入错误
	if writeErr := <-errCh; writeErr != nil {
		return nil, writeErr
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read upload response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cloudflare upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp cloudflareAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse upload response: %w", err)
	}

	if !apiResp.Success {
		errMsg := "unknown error"
		if len(apiResp.Errors) > 0 {
			errMsg = apiResp.Errors[0].Message
		}
		return nil, fmt.Errorf("cloudflare upload error: %s", errMsg)
	}

	var imageResult cloudflareImageResult
	if err := json.Unmarshal(apiResp.Result, &imageResult); err != nil {
		return nil, fmt.Errorf("failed to parse image result: %w", err)
	}

	// 从返回的 variants 中选取第一个作为公开访问地址
	imageURL := ""
	if len(imageResult.Variants) > 0 {
		// 优先选择 "public" 变体，否则用第一个
		for _, v := range imageResult.Variants {
			if strings.HasSuffix(v, "/public") {
				imageURL = v
				break
			}
		}
		if imageURL == "" {
			imageURL = imageResult.Variants[0]
		}
	}

	return &UploadResult{
		ID:  imageResult.ID,
		URL: imageURL,
	}, nil
}

// DeleteImage 从 Cloudflare Images 图床删除指定图片。
// imageID 是上传时返回的图片唯一标识。
func (c *Client) DeleteImage(imageID string) error {
	if !c.IsConfigured() {
		return fmt.Errorf("cloudflare images client is not configured")
	}

	imageID = strings.TrimSpace(imageID)
	if imageID == "" {
		return fmt.Errorf("image id is empty")
	}

	// 拼接 Cloudflare Images V1 删除接口
	apiURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/images/v1/%s", c.accountID, imageID)

	req, err := http.NewRequest(http.MethodDelete, apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read delete response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cloudflare delete failed with status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp cloudflareAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("failed to parse delete response: %w", err)
	}

	if !apiResp.Success {
		errMsg := "unknown error"
		if len(apiResp.Errors) > 0 {
			errMsg = apiResp.Errors[0].Message
		}
		return fmt.Errorf("cloudflare delete error: %s", errMsg)
	}

	return nil
}

// ExtractImageIDFromURL 从 Cloudflare Images delivery URL 中提取图片 ID。
// Cloudflare delivery URL 格式通常为:
// https://imagedelivery.net/{account_hash}/{image_id}/{variant}
// 如果 URL 不符合预期格式，返回空字符串。
func ExtractImageIDFromURL(imageURL string) string {
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return ""
	}

	// 处理 imagedelivery.net 格式的 URL
	// 格式: https://imagedelivery.net/{hash}/{image_id}/{variant}
	if strings.Contains(imageURL, "imagedelivery.net") {
		parts := strings.Split(imageURL, "/")
		// 至少需要: [https:, , imagedelivery.net, hash, image_id, variant]
		if len(parts) >= 6 {
			return parts[len(parts)-2]
		}
	}

	return ""
}
