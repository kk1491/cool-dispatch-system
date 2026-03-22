package security

import "golang.org/x/crypto/bcrypt"

// PasswordMinLength 统一定义系统接受的最小密码长度。
// 当前值只作为后端兜底校验，避免把明显过弱或空密码直接写入数据库。
const PasswordMinLength = 8

// HashPassword 使用 bcrypt 生成口令哈希，统一收敛项目内的密码持久化实现。
func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// VerifyPassword 校验明文密码与数据库哈希是否匹配。
// 所有登录路径统一通过该函数比对，避免再次回到硬编码口令判断。
func VerifyPassword(password string, passwordHash string) bool {
	if passwordHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil
}
