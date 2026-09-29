// Package auth 负责管理员认证、会话、CSRF 与限流。
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id 参数。针对 ARM64 4GB 设备调优：内存 32MB、迭代 2 次。
const (
	argonTime    = 2
	argonMemory  = 32 * 1024
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// ErrWeakPassword 密码强度不足。
var ErrWeakPassword = errors.New("密码长度至少 8 位，且不能为常见弱密码")

// HashPassword 使用 Argon2id 生成密码摘要。
// 输出格式：$argon2id$v=19$m=..,t=..,p=..$<salt-b64>$<hash-b64>
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("生成随机盐失败: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Key := base64.RawStdEncoding.EncodeToString(key)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads, b64Salt, b64Key), nil
}

// VerifyPassword 校验密码是否匹配摘要。
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("密码摘要格式非法")
	}
	var memory uint32
	var timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return false, errors.New("密码摘要参数非法")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errors.New("密码摘要盐非法")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errors.New("密码摘要校验值非法")
	}
	got := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// ValidatePassword 校验密码强度。
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}
	lower := strings.ToLower(strings.TrimSpace(password))
	for _, weak := range weakPasswords {
		if lower == weak {
			return ErrWeakPassword
		}
	}
	return nil
}

var weakPasswords = []string{
	"12345678", "123456789", "1234567890", "password", "admin123", "apphub",
	"qwertyui", "11111111", "00000000", "aaaaaaaa", "abc12345", "iloveyou",
	"administrator", "letmein", "welcome1",
}

// ValidateUsername 校验用户名。
func ValidateUsername(u string) error {
	u = strings.TrimSpace(u)
	if len(u) < 3 || len(u) > 32 {
		return errors.New("用户名长度需在 3 到 32 个字符之间")
	}
	for _, r := range u {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !ok {
			return errors.New("用户名只能包含字母、数字、下划线、连字符和点号")
		}
	}
	return nil
}
