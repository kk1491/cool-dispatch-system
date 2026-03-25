package payuni

import (
	"testing"
)

// ============================================================================
// PAYUNi 加解密单元测试
// 使用官方 Java 范例提供的测试向量验证加密/解密/Hash 的正确性。
// ============================================================================

// 官方测试向量
const (
	// 测试用 Hash Key（32 字节）
	testHashKey = "12345678901234567890123456789012"
	// 测试用 Hash IV（16 字节）
	testHashIV = "1234567890123456"
	// 测试用明文（URL-encoded query string）
	testPlainText = "MerID=ABC&MerTradeNo=1658198662_93966&TradeAmt=7017&Timestamp=1658198662&ProdDesc=%E5%95%86%E5%93%81%E8%AA%AA%E6%98%8E&UsrMail=a%40presco.ws&ReturnURL=http%3A%2F%2Flapi-epay.presco.com.tw%2Fapi%2Fupp%2Freturn"
	// 官方范例加密后的 EncryptInfo（用于解密和 Hash 测试，注意：GCM 每次加密结果不同，因此不能用于加密结果断言）
	testEncryptInfo = "47396636346f66735853653367396942344f587a3775696b34752b593765564a6e337365625a6176316a72706d7377536938436a41695239773545764f3251784b6257665273715374476b70385232564a4643306d655151764855616c7a7a45764c4b4e5462654c574a536553346d527572413357794379324f59555466494a5977344b6f50432f72733564723853546a516d44396c4744672b5a7132696967337345664e4b6f625759637579737a47715767706d6e76786f3773693139534165485374612b673343594a4e65744a4d6b396b6f6b304c6b716d2f596e64494e4863456f35655a693833494f346b4f307679733346695a48734751454b386a4453494c613955556661774234697770506752306e70673d3d3a3a3a50724e743974526e6332704775547a7a7362494a33413d3d"
	// 官方范例 HashInfo
	testHashInfo = "5CED70BDE1027F5DB2512C6B0957D698DADA0DBB67F3051C19A0F48C7455E249"
)

// TestDecrypt 使用官方测试向量验证解密正确性。
func TestDecrypt(t *testing.T) {
	result, err := Decrypt(testEncryptInfo, testHashKey, testHashIV)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if result != testPlainText {
		t.Errorf("解密结果不匹配\n期望: %s\n实际: %s", testPlainText, result)
	}
}

// TestGetHash 使用官方测试向量验证 Hash 计算正确性。
func TestGetHash(t *testing.T) {
	result := GetHash(testEncryptInfo, testHashKey, testHashIV)
	if result != testHashInfo {
		t.Errorf("Hash 结果不匹配\n期望: %s\n实际: %s", testHashInfo, result)
	}
}

// TestEncryptDecryptRoundTrip 验证加密后解密能还原明文（GCM 每次加密结果不同，但解密必须一致）。
func TestEncryptDecryptRoundTrip(t *testing.T) {
	// 加密
	encrypted, err := Encrypt(testPlainText, testHashKey, testHashIV)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}

	// 确认加密结果不为空
	if encrypted == "" {
		t.Fatal("加密结果不应为空")
	}

	// 解密
	decrypted, err := Decrypt(encrypted, testHashKey, testHashIV)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}

	// 验证还原
	if decrypted != testPlainText {
		t.Errorf("加密-解密往返不一致\n期望: %s\n实际: %s", testPlainText, decrypted)
	}
}

// TestGetHashWithEncryptedData 验证自行加密后计算的 Hash 格式正确（64 位大写 hex）。
func TestGetHashWithEncryptedData(t *testing.T) {
	encrypted, err := Encrypt(testPlainText, testHashKey, testHashIV)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}

	hash := GetHash(encrypted, testHashKey, testHashIV)

	// Hash 应为 64 字符的大写 hex 字符串
	if len(hash) != 64 {
		t.Errorf("Hash 长度应为 64，实际: %d", len(hash))
	}
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F')) {
			t.Errorf("Hash 应为大写 hex 字符串，包含非法字符: %c", c)
			break
		}
	}
}

// TestEncryptInvalidKey 验证无效密钥长度时报错。
func TestEncryptInvalidKey(t *testing.T) {
	_, err := Encrypt("test", "short-key", testHashIV)
	if err == nil {
		t.Error("使用无效密钥长度应返回错误")
	}
}

// TestDecryptInvalidHex 验证无效 hex 输入时报错。
func TestDecryptInvalidHex(t *testing.T) {
	_, err := Decrypt("invalid-hex!!!", testHashKey, testHashIV)
	if err == nil {
		t.Error("使用无效 hex 输入应返回错误")
	}
}

// TestDecryptInvalidFormat 验证缺少 ":::" 分隔符时报错。
func TestDecryptInvalidFormat(t *testing.T) {
	// hex 编码一个不含 ":::" 的字符串
	_, err := Decrypt("48656c6c6f", testHashKey, testHashIV)
	if err == nil {
		t.Error("缺少 ':::' 分隔符应返回错误")
	}
}
