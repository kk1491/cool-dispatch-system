package payuni

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// ============================================================================
// PAYUNi 资料加解密工具
// 基于官方 Java 范例实现，使用 AES/GCM/NoPadding 算法。
// ============================================================================

// Encrypt 将明文使用 AES-GCM-256 加密，返回 PAYUNi 格式的 hex 编码密文。
//
// 加密流程（对标官方 Java Encrypt 方法）：
//  1. AES/GCM/NoPadding 加密，tag 长度 128 bit
//  2. 拆分密文和 AuthTag（各 16 字节）
//  3. 分别 Base64 编码后用 ":::" 连接
//  4. 整个字符串做 hex 编码
//
// 参数说明：
//   - plainText: 待加密的 URL-encoded query string
//   - hashKey: 32 字节 AES-256 密钥
//   - hashIV: 16 字节 GCM Nonce/IV
func Encrypt(plainText, hashKey, hashIV string) (string, error) {
	// 创建 AES cipher 块
	block, err := aes.NewCipher([]byte(hashKey))
	if err != nil {
		return "", fmt.Errorf("payuni: 创建 AES cipher 失败: %w", err)
	}

	// PAYUNi Java 范例使用 16 字节 IV，需要 NewGCMWithNonceSize(16)
	// 标准 GCM nonce 是 12 字节，此处必须使用自定义大小
	aesGCM, err := cipher.NewGCMWithNonceSize(block, len([]byte(hashIV)))
	if err != nil {
		return "", fmt.Errorf("payuni: 创建 GCM 失败: %w", err)
	}

	// AES-GCM 加密：返回 密文+Tag 拼接体
	nonce := []byte(hashIV)
	sealed := aesGCM.Seal(nil, nonce, []byte(plainText), nil)

	// 拆分密文和 AuthTag（GCM tag 固定 16 字节）
	tagSize := aesGCM.Overhead() // 16 字节
	if len(sealed) < tagSize {
		return "", fmt.Errorf("payuni: 加密结果长度异常")
	}
	encryptedInfo := sealed[:len(sealed)-tagSize] // 纯密文
	tagInfo := sealed[len(sealed)-tagSize:]       // GCM AuthTag

	// 分别 Base64 编码
	encodeText := base64.StdEncoding.EncodeToString(encryptedInfo)
	encodeTag := base64.StdEncoding.EncodeToString(tagInfo)

	// 拼接并 hex 编码
	finalString := encodeText + ":::" + encodeTag
	hexResult := hex.EncodeToString([]byte(finalString))

	return hexResult, nil
}

// Decrypt 将 PAYUNi 格式的 hex 编码密文解密为明文。
//
// 解密流程（对标官方 Java Decrypt 方法）：
//  1. Hex 解码 → 得到 UTF-8 字符串
//  2. 按 ":::" 拆分 → [encryptInfo, tagString]
//  3. 分别 Base64 解码
//  4. 拼接后 AES-GCM 解密
//
// 参数说明：
//   - cipherHex: PAYUNi 返回的 EncryptInfo（hex 编码字符串）
//   - hashKey: 32 字节 AES-256 密钥
//   - hashIV: 16 字节 GCM Nonce/IV
func Decrypt(cipherHex, hashKey, hashIV string) (string, error) {
	// Hex 解码 → UTF-8 字符串
	hexBytes, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", fmt.Errorf("payuni: hex 解码失败: %w", err)
	}
	encryptStr := string(hexBytes)

	// 按 ":::" 拆分
	parts := strings.SplitN(encryptStr, ":::", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("payuni: 密文格式错误，缺少 ':::' 分隔符")
	}

	// 分别 Base64 解码
	encryptInfoBytes, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("payuni: Base64 解码密文失败: %w", err)
	}
	tagBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("payuni: Base64 解码 AuthTag 失败: %w", err)
	}

	// 拼接密文 + Tag（GCM Open 需要两者合并）
	encryptData := make([]byte, len(encryptInfoBytes)+len(tagBytes))
	copy(encryptData, encryptInfoBytes)
	copy(encryptData[len(encryptInfoBytes):], tagBytes)

	// 创建 AES cipher 块
	block, err := aes.NewCipher([]byte(hashKey))
	if err != nil {
		return "", fmt.Errorf("payuni: 创建 AES cipher 失败: %w", err)
	}

	// PAYUNi 使用 16 字节 IV
	aesGCM, err := cipher.NewGCMWithNonceSize(block, len([]byte(hashIV)))
	if err != nil {
		return "", fmt.Errorf("payuni: 创建 GCM 失败: %w", err)
	}

	// AES-GCM 解密
	nonce := []byte(hashIV)
	decrypted, err := aesGCM.Open(nil, nonce, encryptData, nil)
	if err != nil {
		return "", fmt.Errorf("payuni: AES-GCM 解密失败: %w", err)
	}

	return string(decrypted), nil
}

// GetHash 计算 PAYUNi SHA256 签名（对标官方 Java GetHash 方法）。
//
// 公式：SHA256( HashKey + EncryptInfo + HashIV ).toUpperCase()
// 注意：是直接字符串拼接，不是 key=value 格式。
//
// 参数说明：
//   - encryptInfo: 已加密的 hex 字符串（Encrypt 的返回值）
//   - hashKey: 32 字节 AES-256 密钥
//   - hashIV: 16 字节 GCM Nonce/IV
func GetHash(encryptInfo, hashKey, hashIV string) string {
	// 拼接：HashKey + EncryptInfo + HashIV
	data := hashKey + encryptInfo + hashIV
	// SHA256 哈希
	hash := sha256.Sum256([]byte(data))
	// 转大写 hex 字符串
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}
