package apps

import (
	"path/filepath"
	"testing"

	"github.com/tanglx02/apphub/internal/database"
	"github.com/tanglx02/apphub/internal/models"
)

func newTestRepo(t *testing.T) (*Repository, *CategoryRepository) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "apps.db"), 5000)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(db.DB), NewCategoryRepository(db.DB)
}

func TestAppCRUD(t *testing.T) {
	repo, _ := newTestRepo(t)

	app := &models.App{
		Name:         "Vaultwarden",
		Description:  "私有密码管理",
		Icon:         "🔐",
		AppType:      models.AppTypeCommand,
		StartCommand: "/opt/apps/vaultwarden/start.sh",
		StatusType:   models.CheckTCP,
		StatusTarget: "127.0.0.1:8222",
		InternalURL:  "http://192.168.1.72:8222",
		ExternalURL:  "https://vault.example.com",
		Enabled:      true,
		Tags:         "密码, 私有化",
	}
	if err := repo.Create(app); err != nil {
		t.Fatalf("创建应用失败: %v", err)
	}
	if app.ID == 0 {
		t.Fatal("创建后 ID 不应为 0")
	}
	if app.Slug != "vaultwarden" {
		t.Fatalf("slug 自动生成错误: %s", app.Slug)
	}

	got, err := repo.Get(app.ID)
	if err != nil {
		t.Fatalf("读取应用失败: %v", err)
	}
	if got.Name != app.Name || got.InternalURL != app.InternalURL {
		t.Fatalf("读取到的应用与写入不一致: %+v", got)
	}

	tags := got.TagList()
	if len(tags) != 2 || tags[0] != "密码" {
		t.Fatalf("标签解析错误: %#v", tags)
	}

	got.Description = "修改后的描述"
	got.Favorite = true
	if err := repo.Update(got); err != nil {
		t.Fatalf("更新应用失败: %v", err)
	}
	updated, _ := repo.Get(app.ID)
	if updated.Description != "修改后的描述" || !updated.Favorite {
		t.Fatalf("更新未生效: %+v", updated)
	}

	list, err := repo.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("列表查询失败: %v len=%d", err, len(list))
	}

	if err := repo.Delete(app.ID); err != nil {
		t.Fatalf("删除应用失败: %v", err)
	}
	if _, err := repo.Get(app.ID); err != ErrNotFound {
		t.Fatalf("删除后应返回 ErrNotFound，实际: %v", err)
	}
}

func TestAppSortOrderAutoIncrement(t *testing.T) {
	repo, _ := newTestRepo(t)
	a := &models.App{Name: "A", AppType: models.AppTypeCommand, StartCommand: "/bin/true", StatusType: models.CheckNone}
	b := &models.App{Name: "B", AppType: models.AppTypeCommand, StartCommand: "/bin/true", StatusType: models.CheckNone}
	_ = repo.Create(a)
	_ = repo.Create(b)
	if a.SortOrder >= b.SortOrder {
		t.Fatalf("排序应递增: a=%d b=%d", a.SortOrder, b.SortOrder)
	}
}

func TestReorder(t *testing.T) {
	repo, _ := newTestRepo(t)
	ids := make([]int64, 0, 3)
	for _, name := range []string{"A", "B", "C"} {
		a := &models.App{Name: name, AppType: models.AppTypeCommand, StartCommand: "/bin/true", StatusType: models.CheckNone}
		_ = repo.Create(a)
		ids = append(ids, a.ID)
	}
	// 反转顺序
	rev := []int64{ids[2], ids[1], ids[0]}
	if err := repo.Reorder(rev); err != nil {
		t.Fatalf("重排失败: %v", err)
	}
	list, _ := repo.List()
	if list[0].Name != "C" || list[2].Name != "A" {
		t.Fatalf("重排结果错误: %s %s %s", list[0].Name, list[1].Name, list[2].Name)
	}
}

func TestCategoryCRUD(t *testing.T) {
	_, cats := newTestRepo(t)

	// 迁移已注入默认分类
	list, err := cats.List()
	if err != nil {
		t.Fatalf("读取分类失败: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("默认分类未注入")
	}

	c := &models.Category{Name: "自建分类", Icon: "Box", Color: "#123456"}
	if err := cats.Create(c); err != nil {
		t.Fatalf("创建分类失败: %v", err)
	}
	if c.Slug == "" {
		t.Fatal("分类 slug 未自动生成")
	}
	c.Name = "改名后"
	if err := cats.Update(c); err != nil {
		t.Fatalf("更新分类失败: %v", err)
	}
	if err := cats.Delete(c.ID); err != nil {
		t.Fatalf("删除分类失败: %v", err)
	}
	if got, _ := cats.Get(c.ID); got != nil {
		t.Fatal("删除后分类应不存在")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"CyberStrike AI": "cyberstrike-ai",
		"测试 Web":         "测试-web",
		"Vaultwarden":    "vaultwarden",
	}
	for in, want := range cases {
		got := Slugify(in)
		if got != want {
			t.Fatalf("Slugify(%q) = %q, 期望 %q", in, got, want)
		}
	}
}
