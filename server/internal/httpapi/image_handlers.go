package httpapi

import (
	"net/http"
	"strings"

	"cool-dispatch/internal/cloudflare"
	"cool-dispatch/internal/logger"

	"github.com/gin-gonic/gin"
)

// imageUploadResponse 是图片上传成功后返回给前端的响应结构。
type imageUploadResponse struct {
	// ID 是 Cloudflare Images 分配的唯一图片标识，删除时需要此值。
	ID string `json:"id"`
	// URL 是图片的公开访问地址。
	URL string `json:"url"`
}

// imageDeletePayload 是前端请求删除图片时提交的载荷。
type imageDeletePayload struct {
	// URL 是要删除的图片公开访问地址，后端会从中提取图片 ID 进行删除。
	URL string `json:"url"`
}

// UploadImage 接收前端上传的图片文件，转存到 Cloudflare Images 图床，
// 返回图片的公开访问 URL 和唯一标识。
// 前端拿到 URL 后将其存入 photos 数组，替代之前的 Base64 Data URL 方案。
func (h *Handler) UploadImage(c *gin.Context) {
	if !h.cfClient.IsConfigured() {
		respondMessage(c, http.StatusServiceUnavailable, "cloudflare images is not configured")
		return
	}

	// 限制上传文件大小为 10MB（Cloudflare Images 的上限）
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)

	// 从 multipart/form-data 中读取 file 字段
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		if isRequestBodyTooLarge(err) {
			abortWithMessage(c, http.StatusRequestEntityTooLarge, "image file too large, max 10MB")
			return
		}
		respondMessage(c, http.StatusBadRequest, "missing or invalid image file")
		return
	}
	defer file.Close()

	// 校验文件类型，只允许常见图片格式
	contentType := header.Header.Get("Content-Type")
	if !isAllowedImageType(contentType) {
		respondMessage(c, http.StatusBadRequest, "unsupported image type, only JPEG/PNG/GIF/WebP/HEIC allowed")
		return
	}

	// 调用 Cloudflare Images API 上传
	result, err := h.cfClient.UploadImage(header.Filename, file)
	if err != nil {
		logger.Errorf("[cloudflare] upload error: %v", err)
		respondMessage(c, http.StatusInternalServerError, "failed to upload image to cloudflare")
		return
	}

	c.JSON(http.StatusOK, imageUploadResponse{
		ID:  result.ID,
		URL: result.URL,
	})
}

// DeleteImage 从 Cloudflare Images 图床删除指定图片。
// 前端提交图片的公开访问 URL，后端自动提取图片 ID 并调用删除接口。
func (h *Handler) DeleteImage(c *gin.Context) {
	if !h.cfClient.IsConfigured() {
		respondMessage(c, http.StatusServiceUnavailable, "cloudflare images is not configured")
		return
	}

	var payload imageDeletePayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid delete payload") {
		return
	}

	imageURL := strings.TrimSpace(payload.URL)
	if imageURL == "" {
		respondMessage(c, http.StatusBadRequest, "image url is required")
		return
	}

	// 从 URL 中提取图片 ID
	imageID := cloudflare.ExtractImageIDFromURL(imageURL)
	if imageID == "" {
		// 如果无法从 URL 提取 ID，可能是非 Cloudflare 图片（如旧的 Base64 数据），静默成功
		c.JSON(http.StatusOK, gin.H{"deleted": true, "message": "non-cloudflare image, skipped"})
		return
	}

	// 调用 Cloudflare Images API 删除
	if err := h.cfClient.DeleteImage(imageID); err != nil {
		logger.Errorf("[cloudflare] delete error: %v", err)
		respondMessage(c, http.StatusInternalServerError, "failed to delete image from cloudflare")
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// isAllowedImageType 检查上传文件的 MIME 类型是否属于允许的图片格式。
func isAllowedImageType(contentType string) bool {
	allowed := []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
		"image/heic",
		"image/heif",
		"image/svg+xml",
		"image/bmp",
		"image/tiff",
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	for _, t := range allowed {
		if ct == t {
			return true
		}
	}
	// 部分浏览器/手机拍照上传时 Content-Type 可能为空或 application/octet-stream，
	// 这种情况也放行，让 Cloudflare 做最终校验。
	return ct == "" || ct == "application/octet-stream"
}
