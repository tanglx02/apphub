package systemd

import (
	"testing"

	"github.com/tanglx02/apphub/internal/models"
)

func TestValidateUnit(t *testing.T) {
	valid := []string{
		"apphub.service",
		"cyberstrike-ai.service",
		"docker.socket",
		"backup.timer",
		"nginx@reload.service",
	}
	for _, u := range valid {
		if _, err := ValidateUnit(u); err != nil {
			t.Fatalf("合法服务名被拒绝: %s (%v)", u, err)
		}
	}

	invalid := []string{
		"",
		"apphub",
		"apphub.service; rm -rf /",
		"apphub.service && cat /etc/passwd",
		"$(whoami).service",
		"../etc/passwd.service",
		"my service.service",
		"apphub.exe",
		"a`b`.service",
	}
	for _, u := range invalid {
		if _, err := ValidateUnit(u); err == nil {
			t.Fatalf("非法服务名未被拦截: %q", u)
		}
	}
}

func TestParseActiveState(t *testing.T) {
	cases := map[string]models.AppStatus{
		"active":       models.StatusOnline,
		"inactive":     models.StatusOffline,
		"failed":       models.StatusError,
		"activating":   models.StatusStarting,
		"deactivating": models.StatusStopping,
		"unknown":      models.StatusUnknown,
		"":             models.StatusUnknown,
	}
	for in, want := range cases {
		if got := ParseActiveState(in); got != want {
			t.Fatalf("ParseActiveState(%q) = %s, 期望 %s", in, got, want)
		}
	}
}
