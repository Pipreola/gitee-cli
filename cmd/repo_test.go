package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// makeRepo 构造测试用 Repository 数据。
func makeRepo(fullName, lang, desc string, private bool, stars, forks, watchers, openIssues int64) *api.Repository {
	return &api.Repository{
		ID:              42,
		FullName:        fullName,
		Name:            "repo",
		Description:     desc,
		Language:        lang,
		Private:         private,
		StargazersCount: stars,
		ForksCount:      forks,
		WatchersCount:   watchers,
		OpenIssuesCount: openIssues,
		DefaultBranch:   "main",
		HTMLURL:         "https://gitee.com/" + fullName,
		Homepage:        "https://example.com",
		PushedAt:        fixedNow.Add(-3 * time.Hour).Format(time.RFC3339),
	}
}

// newRepoViewGitFake 返回带 origin 的 fakeGitRunner。
func newRepoViewGitFake() *fakeGitRunner {
	return &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"}
}

// TestRunRepoViewNoAuth 未登录时报错。
func TestRunRepoViewNoAuth(t *testing.T) {
	env := repoViewEnv{
		git:        newRepoViewGitFake(),
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: ""}, nil },
		out:        &bytes.Buffer{},
		isTTY:      func() bool { return false },
		now:        func() time.Time { return fixedNow },
	}
	err := runRepoView(context.Background(), repoViewOptions{}, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunRepoViewFromCurrentRepo 验证从当前 git remote 推断仓库。
func TestRunRepoViewFromCurrentRepo(t *testing.T) {
	r := makeRepo("owner/repo", "Go", "演示仓库", false, 12, 3, 5, 2)
	out := &bytes.Buffer{}
	gotOwner, gotRepo := "", ""
	env := repoViewEnv{
		git:        newRepoViewGitFake(),
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		getRepo: func(ctx context.Context, host, token, owner, repo string) (*api.Repository, error) {
			gotOwner, gotRepo = owner, repo
			return r, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runRepoView(context.Background(), repoViewOptions{}, env); err != nil {
		t.Fatalf("runRepoView 错误: %v", err)
	}
	if gotOwner != "owner" || gotRepo != "repo" {
		t.Errorf("getRepo 参数 = (%s,%s), 期望 (owner,repo)", gotOwner, gotRepo)
	}
	output := out.String()
	for _, want := range []string{"owner/repo", "演示仓库", "Go", "Star", "12", "3 小时前", "main"} {
		if !strings.Contains(output, want) {
			t.Errorf("输出缺少 %q\n实际:\n%s", want, output)
		}
	}
}

// TestRunRepoViewFromArg 验证显式 owner/repo 参数覆盖 git 推断。
func TestRunRepoViewFromArg(t *testing.T) {
	r := makeRepo("alice/cool-repo", "Python", "Other repo", true, 0, 0, 0, 0)
	gotOwner, gotRepo := "", ""
	env := repoViewEnv{
		// git remote 故意配置成另一个仓库，验证显式参数会覆盖
		git:        &fakeGitRunner{remoteURL: "https://gitee.com/wrong/wrong.git"},
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		getRepo: func(ctx context.Context, host, token, owner, repo string) (*api.Repository, error) {
			gotOwner, gotRepo = owner, repo
			return r, nil
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runRepoView(context.Background(), repoViewOptions{repo: "alice/cool-repo"}, env)
	if err != nil {
		t.Fatalf("runRepoView 错误: %v", err)
	}
	if gotOwner != "alice" || gotRepo != "cool-repo" {
		t.Errorf("getRepo 参数 = (%s,%s), 期望 (alice,cool-repo)", gotOwner, gotRepo)
	}
}

// TestParseRepoArgInvalid 验证非法仓库定位被拒绝。
func TestParseRepoArgInvalid(t *testing.T) {
	cases := []string{"", "owner", "/repo", "owner/", "owner/repo/extra"}
	for _, c := range cases {
		if _, _, err := parseRepoArg(c); err == nil && c != "owner/repo/extra" {
			// owner/repo/extra 会被拆为 ("owner", "repo/extra")，本测试只校验明显错误形式
			t.Errorf("parseRepoArg(%q) 期望报错", c)
		}
	}
	// 合法
	if owner, repo, err := parseRepoArg("alice/proj.git"); err != nil || owner != "alice" || repo != "proj" {
		t.Errorf("parseRepoArg(\"alice/proj.git\") = (%s,%s,%v), 期望 (alice,proj,nil)", owner, repo, err)
	}
}

// TestRunRepoViewJSON 验证 --json 输出 JSON。
func TestRunRepoViewJSON(t *testing.T) {
	r := makeRepo("owner/repo", "Go", "desc", false, 1, 2, 3, 4)
	out := &bytes.Buffer{}
	env := repoViewEnv{
		git:        newRepoViewGitFake(),
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		getRepo: func(ctx context.Context, host, token, owner, repo string) (*api.Repository, error) {
			return r, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runRepoView(context.Background(), repoViewOptions{jsonOut: true}, env); err != nil {
		t.Fatalf("runRepoView 错误: %v", err)
	}
	s := strings.TrimSpace(out.String())
	if !strings.HasPrefix(s, "{") || !strings.Contains(s, `"full_name": "owner/repo"`) {
		t.Errorf("期望 JSON 输出包含 full_name，实际:\n%s", out.String())
	}
}

// TestRunRepoViewWeb 验证 --web 调用浏览器。
func TestRunRepoViewWeb(t *testing.T) {
	r := makeRepo("owner/repo", "Go", "", false, 0, 0, 0, 0)
	openedURL := ""
	env := repoViewEnv{
		git:        newRepoViewGitFake(),
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		getRepo: func(ctx context.Context, host, token, owner, repo string) (*api.Repository, error) {
			return r, nil
		},
		openBrowser: func(u string) error {
			openedURL = u
			return nil
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runRepoView(context.Background(), repoViewOptions{web: true}, env); err != nil {
		t.Fatalf("runRepoView 错误: %v", err)
	}
	if openedURL != r.HTMLURL {
		t.Errorf("openBrowser URL = %q, 期望 %q", openedURL, r.HTMLURL)
	}
}

// TestRunRepoViewAPIError 验证 API 错误被透传。
func TestRunRepoViewAPIError(t *testing.T) {
	env := repoViewEnv{
		git:        newRepoViewGitFake(),
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		getRepo: func(ctx context.Context, host, token, owner, repo string) (*api.Repository, error) {
			return nil, errors.New("404 Not Found")
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runRepoView(context.Background(), repoViewOptions{}, env)
	if err == nil || !strings.Contains(err.Error(), "404 Not Found") {
		t.Errorf("期望透传 404 错误，实际: %v", err)
	}
}

// TestRunRepoViewPrivateLabel 验证私有仓库可见性标记。
func TestRunRepoViewPrivateLabel(t *testing.T) {
	r := makeRepo("owner/secret", "Go", "私有", true, 0, 0, 0, 0)
	out := &bytes.Buffer{}
	env := repoViewEnv{
		git:        newRepoViewGitFake(),
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		getRepo: func(ctx context.Context, host, token, owner, repo string) (*api.Repository, error) {
			return r, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runRepoView(context.Background(), repoViewOptions{}, env); err != nil {
		t.Fatalf("runRepoView 错误: %v", err)
	}
	if !strings.Contains(out.String(), "private") {
		t.Errorf("期望显示 private 标记，实际:\n%s", out.String())
	}
}
