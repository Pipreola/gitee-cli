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

// makePRView 构造 pr view 测试用 PR 数据。
func makePRView(number int64, title, state, body string, mergedAt string) *api.PullRequest {
	return &api.PullRequest{
		Number:    number,
		Title:     title,
		Body:      body,
		State:     state,
		HTMLURL:   "https://gitee.com/owner/repo/pulls/" + strings.TrimSpace(strings.ReplaceAll("X", "X", "")) + itoa64(number),
		Head:      api.Branch{Ref: "feat", Label: "owner:feat"},
		Base:      api.Branch{Ref: "main", Label: "owner:main"},
		User:      api.User{Login: "alice"},
		CreatedAt: fixedNow.Add(-2 * time.Hour).Format(time.RFC3339),
		UpdatedAt: fixedNow.Add(-1 * time.Hour).Format(time.RFC3339),
		MergedAt:  mergedAt,
		Mergeable: state == "open",
	}
}

// itoa64 是一个微型整数转字符串辅助函数，避免引入额外依赖。
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// newPRViewGitFake 返回带 origin 的 fakeGitRunner。
func newPRViewGitFake() *fakeGitRunner {
	return &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"}
}

// TestRunPRViewNoAuth 未登录时报错。
func TestRunPRViewNoAuth(t *testing.T) {
	env := prViewEnv{
		git: newPRViewGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: ""}, nil
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runPRView(context.Background(), prViewOptions{number: 1}, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunPRViewSuccess 验证默认输出包含关键信息。
func TestRunPRViewSuccess(t *testing.T) {
	pr := makePRView(123, "添加新功能", "open", "这是 PR 描述。", "")
	out := &bytes.Buffer{}
	env := prViewEnv{
		git:        newPRViewGitFake(),
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		getPR: func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error) {
			if owner != "owner" || repo != "repo" || number != 123 {
				t.Errorf("getPR 参数 = (%s,%s,%d), 期望 (owner,repo,123)", owner, repo, number)
			}
			return pr, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runPRView(context.Background(), prViewOptions{number: 123}, env); err != nil {
		t.Fatalf("runPRView 错误: %v", err)
	}
	output := out.String()
	for _, want := range []string{"添加新功能", "#123", "alice", "main ← feat", "这是 PR 描述。", "可合并"} {
		if !strings.Contains(output, want) {
			t.Errorf("输出缺少 %q\n实际:\n%s", want, output)
		}
	}
}

// TestRunPRViewMerged 验证已合并 PR 显示合并提示。
func TestRunPRViewMerged(t *testing.T) {
	pr := makePRView(8, "已合并的 PR", "closed", "", fixedNow.Add(-24*time.Hour).Format(time.RFC3339))
	out := &bytes.Buffer{}
	env := prViewEnv{
		git:        newPRViewGitFake(),
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		getPR: func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error) {
			return pr, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runPRView(context.Background(), prViewOptions{number: 8}, env); err != nil {
		t.Fatalf("runPRView 错误: %v", err)
	}
	if !strings.Contains(out.String(), "已合并") {
		t.Errorf("期望显示 '已合并'，实际:\n%s", out.String())
	}
}

// TestRunPRViewWithComments 验证 --comments 拉取并显示评论。
func TestRunPRViewWithComments(t *testing.T) {
	pr := makePRView(7, "PR 标题", "open", "描述", "")
	comments := []api.Comment{
		{Body: "👍 LGTM", User: api.User{Login: "bob"}, CreatedAt: fixedNow.Add(-30 * time.Minute).Format(time.RFC3339)},
		{Body: "请补充测试", User: api.User{Login: "carol"}, CreatedAt: fixedNow.Add(-10 * time.Minute).Format(time.RFC3339)},
	}
	out := &bytes.Buffer{}
	env := prViewEnv{
		git:        newPRViewGitFake(),
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		getPR: func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error) {
			return pr, nil
		},
		listComments: func(ctx context.Context, host, token, owner, repo string, number int64) ([]api.Comment, error) {
			return comments, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runPRView(context.Background(), prViewOptions{number: 7, comments: true}, env)
	if err != nil {
		t.Fatalf("runPRView 错误: %v", err)
	}
	output := out.String()
	for _, want := range []string{"2 条评论", "👍 LGTM", "bob", "请补充测试", "carol"} {
		if !strings.Contains(output, want) {
			t.Errorf("输出缺少 %q\n实际:\n%s", want, output)
		}
	}
}

// TestRunPRViewJSON 验证 --json 输出可解析的 JSON。
func TestRunPRViewJSON(t *testing.T) {
	pr := makePRView(99, "JSON 测试", "open", "body", "")
	out := &bytes.Buffer{}
	env := prViewEnv{
		git:        newPRViewGitFake(),
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		getPR: func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error) {
			return pr, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runPRView(context.Background(), prViewOptions{number: 99, jsonOut: true}, env); err != nil {
		t.Fatalf("runPRView 错误: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Errorf("期望 JSON 输出，实际:\n%s", out.String())
	}
	if !strings.Contains(out.String(), `"pull_request"`) {
		t.Errorf("期望包含 pull_request 字段，实际:\n%s", out.String())
	}
}

// TestRunPRViewWeb 验证 --web 调用浏览器并跳过评论拉取。
func TestRunPRViewWeb(t *testing.T) {
	pr := makePRView(5, "T", "open", "", "")
	openedURL := ""
	commentsCalled := false
	out := &bytes.Buffer{}
	env := prViewEnv{
		git:        newPRViewGitFake(),
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		getPR: func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error) {
			return pr, nil
		},
		listComments: func(ctx context.Context, host, token, owner, repo string, number int64) ([]api.Comment, error) {
			commentsCalled = true
			return nil, nil
		},
		openBrowser: func(u string) error {
			openedURL = u
			return nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runPRView(context.Background(), prViewOptions{number: 5, web: true, comments: true}, env)
	if err != nil {
		t.Fatalf("runPRView 错误: %v", err)
	}
	if openedURL != pr.HTMLURL {
		t.Errorf("openBrowser 被调用 URL = %q, 期望 %q", openedURL, pr.HTMLURL)
	}
	if commentsCalled {
		t.Error("--web 模式下不应拉取评论")
	}
}

// TestRunPRViewAPIError 验证 API 错误透传给用户。
func TestRunPRViewAPIError(t *testing.T) {
	env := prViewEnv{
		git:        newPRViewGitFake(),
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		getPR: func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error) {
			return nil, errors.New("404 Not Found")
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runPRView(context.Background(), prViewOptions{number: 999}, env)
	if err == nil || !strings.Contains(err.Error(), "404 Not Found") {
		t.Errorf("期望透传 404 错误，实际: %v", err)
	}
}
