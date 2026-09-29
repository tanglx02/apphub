package auth

import (
	"strings"
	"testing"
	"time"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPassword("Admin@2026")
	if err != nil {
		t.Fatalf("生成哈希失败: %v", err)
	}
	if strings.Contains(hash, "Admin@2026") {
		t.Fatal("哈希中不应包含明文密码")
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("哈希格式错误: %s", hash)
	}

	ok, err := VerifyPassword("Admin@2026", hash)
	if err != nil {
		t.Fatalf("校验失败: %v", err)
	}
	if !ok {
		t.Fatal("正确密码应校验通过")
	}

	ok, _ = VerifyPassword("wrong-password", hash)
	if ok {
		t.Fatal("错误密码不应通过校验")
	}
}

func TestHashSaltUnique(t *testing.T) {
	h1, _ := HashPassword("same-password")
	h2, _ := HashPassword("same-password")
	if h1 == h2 {
		t.Fatal("相同密码的两次哈希不应相同（盐值必须随机）")
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("short"); err == nil {
		t.Fatal("过短密码应被拒绝")
	}
	if err := ValidatePassword("12345678"); err == nil {
		t.Fatal("弱密码应被拒绝")
	}
	if err := ValidatePassword("Admin@2026"); err != nil {
		t.Fatalf("合规密码不应被拒绝: %v", err)
	}
}

func TestValidateUsername(t *testing.T) {
	if err := ValidateUsername("ab"); err == nil {
		t.Fatal("过短用户名应被拒绝")
	}
	if err := ValidateUsername("admin user"); err == nil {
		t.Fatal("含空格的用户名应被拒绝")
	}
	if err := ValidateUsername("admin_01"); err != nil {
		t.Fatalf("合规用户名不应被拒绝: %v", err)
	}
}

func TestCSRF(t *testing.T) {
	m := NewCSRFManager()
	token := m.Token("session-abc")
	if token == "" {
		t.Fatal("CSRF token 不应为空")
	}
	if !m.Validate("session-abc", token) {
		t.Fatal("同会话的 token 应校验通过")
	}
	if m.Validate("session-xyz", token) {
		t.Fatal("跨会话的 token 不应通过")
	}
	if m.Validate("session-abc", "") {
		t.Fatal("空 token 不应通过")
	}
}

func TestRateLimiter(t *testing.T) {
	r := NewRateLimiter(3, 60*1e9)
	for i := 0; i < 3; i++ {
		if !r.Allow("1.2.3.4") {
			t.Fatalf("第 %d 次请求应被放行", i+1)
		}
	}
	if r.Allow("1.2.3.4") {
		t.Fatal("超过限制后应被拒绝")
	}
	if !r.Allow("5.6.7.8") {
		t.Fatal("不同 IP 应互不影响")
	}
	r.Reset("1.2.3.4")
	if !r.Allow("1.2.3.4") {
		t.Fatal("重置后应重新放行")
	}
}

func TestLoginGuard(t *testing.T) {
	g := NewLoginGuard(3, 15)
	key := "admin|1.2.3.4"
	if g.Check(key) != 0 {
		t.Fatal("初始状态不应被锁定")
	}
	locked := false
	for i := 0; i < 3; i++ {
		var remain time.Duration
		locked, remain = g.Fail(key)
		_ = remain
	}
	if !locked {
		t.Fatal("达到阈值后应锁定")
	}
	if g.Check(key) <= 0 {
		t.Fatal("锁定期内 Check 应返回剩余时间")
	}
	g.Success(key)
	if g.Check(key) != 0 {
		t.Fatal("成功后应解除锁定")
	}
}
