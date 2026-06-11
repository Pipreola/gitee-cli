package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// fixedNow 是测试中固定的"当前时间"，便于断言相对时间输出。
var fixedNow = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

// newPRListGitFake 返回一个仓库 URL 已配置的 fakeGitRunner。
func newPRListGitFake() *fakeGitRunner {
	return &fakeGitRunner{
		remoteURL: "https://gitee.com/owner/repo.git",
	}
}

// makePR 构造测试用 PR 数据。
func makePR(number int64, title, state, headRef, baseRef, login string, createdAt time.Time) api.PullRequest {
	return api.PullRequest{
		Number:    number,
		Title:     title,
		State:     state,
		HTMLURL:   fmt.Sprintf("https://gitee.com/owner/repo/pulls/%d", number),
		Head:      api.Branch{Ref: headRef, Label: "owner:" + headRef, Sha: fmt.Sprintf("h%d", number)},
		Base:      api.Branch{Ref: baseRef, Label: "owner:" + baseRef, Sha: fmt.Sprintf("b%d", number)},
		User:      api.User{Login: login, ID: number},
		CreatedAt: createdAt.Format(time.RFC3339),
		UpdatedAt: createdAt.Format(time.RFC3339),
	}
}

// TestRunPRListNoAuth 验证未登录时报错。
func TestRunPRListNoAuth(t *testing.T) {
	opts := prListOptions{state: "open", direction: "desc", sort: "created", limit: 30}
	env := prListEnv{
		git: newPRListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: ""}, nil
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runPRList(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunPRListGetRepoError 验证仓库解析失败的错误处理。
func TestRunPRListGetRepoError(t *testing.T) {
	opts := prListOptions{state: "open", direction: "desc", sort: "created", limit: 30}
	env := prListEnv{
		git: &fakeGitRunner{remoteURL: ""},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runPRList(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "获取仓库信息失败") {
		t.Errorf("期望仓库信息错误，实际: %v", err)
	}
}

// TestValidatePRListOptions 验证参数校验逻辑。
func TestValidatePRListOptions(t *testing.T) {
	cases := []struct {
		name    string
		opts    prListOptions
		wantErr string
	}{
		{"valid open", prListOptions{state: "open", direction: "desc", sort: "created", limit: 30}, ""},
		{"valid all", prListOptions{state: "all", direction: "asc", sort: "updated", limit: 100}, ""},
		{"invalid state", prListOptions{state: "weird", direction: "desc", sort: "created", limit: 30}, "无效的 --state"},
		{"invalid direction", prListOptions{state: "open", direction: "sideways", sort: "created", limit: 30}, "无效的 --direction"},
		{"invalid sort", prListOptions{state: "open", direction: "desc", sort: "magic", limit: 30}, "无效的 --sort"},
		{"limit too low", prListOptions{state: "open", direction: "desc", sort: "created", limit: 0}, "无效的 --limit"},
		{"limit too high", prListOptions{state: "open", direction: "desc", sort: "created", limit: 101}, "无效的 --limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := tc.opts
			err := validatePRListOptions(&opts)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("期望无错误，实际: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("期望错误包含 %q，实际: %v", tc.wantErr, err)
			}
		})
	}
}

// TestRunPRListSuccess 验证成功列出 PR 并输出表格。
func TestRunPRListSuccess(t *testing.T) {
	prs := []api.PullRequest{
		makePR(101, "添加新功能", "open", "feat", "main", "alice", fixedNow.Add(-2*time.Hour)),
		makePR(102, "修复 bug", "merged", "fix", "main", "bob", fixedNow.Add(-3*24*time.Hour)),
	}
	out := &bytes.Buffer{}
	opts := prListOptions{state: "open", direction: "desc", sort: "created", limit: 30}
	env := prListEnv{
		git: newPRListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		listPRs: func(ctx context.Context, host, token, owner, repo string, input *api.ListPullRequestsInput) ([]api.PullRequest, error) {
			if owner != "owner" || repo != "repo" {
				t.Errorf("owner/repo = %q/%q, 期望 owner/repo", owner, repo)
			}
			if input.State != "open" {
				t.Errorf("input.State = %q, 期望 open", input.State)
			}
			if input.PerPage != 30 {
				t.Errorf("input.PerPage = %d, 期望 30", input.PerPage)
			}
			return prs, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runPRList(context.Background(), opts, env); err != nil {
		t.Fatalf("runPRList 返回错误: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Showing 2 pull requests in owner/repo") {
		t.Errorf("期望标题行，实际: %s", output)
	}
	if !strings.Contains(output, "#101") || !strings.Contains(output, "#102") {
		t.Errorf("期望输出 PR 编号，实际: %s", output)
	}
	if !strings.Contains(output, "OPEN") || !strings.Contains(output, "MERGED") {
		t.Errorf("期望输出 PR 状态，实际: %s", output)
	}
	if !strings.Contains(output, "2 小时前") {
		t.Errorf("期望输出相对时间，实际: %s", output)
	}
	if strings.Contains(output, "AUTHOR") {
		t.Errorf("非 verbose 模式不应输出 AUTHOR 列，实际: %s", output)
	}
}

// TestRunPRListEmpty 验证无 PR 时的友好提示。
func TestRunPRListEmpty(t *testing.T) {
	out := &bytes.Buffer{}
	opts := prListOptions{state: "open", direction: "desc", sort: "created", limit: 30}
	env := prListEnv{
		git: newPRListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		listPRs: func(ctx context.Context, host, token, owner, repo string, input *api.ListPullRequestsInput) ([]api.PullRequest, error) {
			return []api.PullRequest{}, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runPRList(context.Background(), opts, env); err != nil {
		t.Fatalf("runPRList 返回错误: %v", err)
	}
	if !strings.Contains(out.String(), "没有匹配的 Pull Request") {
		t.Errorf("期望空列表提示，实际: %s", out.String())
	}
}

// TestRunPRListVerbose 验证 verbose 模式输出额外列。
func TestRunPRListVerbose(t *testing.T) {
	prs := []api.PullRequest{
		makePR(1, "Hello", "open", "feat", "main", "alice", fixedNow.Add(-time.Hour)),
	}
	out := &bytes.Buffer{}
	opts := prListOptions{state: "open", direction: "desc", sort: "created", limit: 30, verbose: true}
	env := prListEnv{
		git: newPRListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		listPRs: func(ctx context.Context, host, token, owner, repo string, input *api.ListPullRequestsInput) ([]api.PullRequest, error) {
			return prs, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runPRList(context.Background(), opts, env); err != nil {
		t.Fatalf("runPRList 返回错误: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "AUTHOR") || !strings.Contains(output, "UPDATED") {
		t.Errorf("verbose 模式应输出 AUTHOR/UPDATED 列，实际: %s", output)
	}
	if !strings.Contains(output, "alice") {
		t.Errorf("verbose 模式应输出作者名，实际: %s", output)
	}
}

// TestRunPRListJSON 验证 JSON 输出格式。
func TestRunPRListJSON(t *testing.T) {
	prs := []api.PullRequest{
		makePR(1, "Hello", "open", "feat", "main", "alice", fixedNow.Add(-time.Hour)),
		makePR(2, "World", "merged", "fix", "main", "bob", fixedNow.Add(-time.Hour)),
	}
	out := &bytes.Buffer{}
	opts := prListOptions{state: "all", direction: "desc", sort: "created", limit: 30, jsonOut: true}
	env := prListEnv{
		git: newPRListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		listPRs: func(ctx context.Context, host, token, owner, repo string, input *api.ListPullRequestsInput) ([]api.PullRequest, error) {
			return prs, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runPRList(context.Background(), opts, env); err != nil {
		t.Fatalf("runPRList 返回错误: %v", err)
	}
	var decoded []api.PullRequest
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON 解析失败: %v\n输出: %s", err, out.String())
	}
	if len(decoded) != 2 {
		t.Errorf("解析出 %d 条 PR，期望 2", len(decoded))
	}
}

// TestRunPRListAuthorFilter 验证客户端按作者过滤。
func TestRunPRListAuthorFilter(t *testing.T) {
	prs := []api.PullRequest{
		makePR(1, "A", "open", "f1", "main", "alice", fixedNow),
		makePR(2, "B", "open", "f2", "main", "bob", fixedNow),
		makePR(3, "C", "open", "f3", "main", "Alice", fixedNow),
	}
	out := &bytes.Buffer{}
	opts := prListOptions{state: "open", direction: "desc", sort: "created", limit: 30, author: "alice"}
	env := prListEnv{
		git: newPRListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		listPRs: func(ctx context.Context, host, token, owner, repo string, input *api.ListPullRequestsInput) ([]api.PullRequest, error) {
			return prs, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runPRList(context.Background(), opts, env); err != nil {
		t.Fatalf("runPRList 返回错误: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "#1") || !strings.Contains(output, "#3") {
		t.Errorf("期望保留 alice 的 PR #1 #3，实际: %s", output)
	}
	if strings.Contains(output, "#2") {
		t.Errorf("期望过滤掉 bob 的 PR #2，实际: %s", output)
	}
}

// TestRunPRListAtMeAuthor 验证 @me 替换为已登录用户。
func TestRunPRListAtMeAuthor(t *testing.T) {
	prs := []api.PullRequest{
		makePR(1, "mine", "open", "f1", "main", "self", fixedNow),
		makePR(2, "theirs", "open", "f2", "main", "other", fixedNow),
	}
	out := &bytes.Buffer{}
	opts := prListOptions{state: "open", direction: "desc", sort: "created", limit: 30, author: "@me"}
	env := prListEnv{
		git: newPRListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok", User: "self"}, nil
		},
		listPRs: func(ctx context.Context, host, token, owner, repo string, input *api.ListPullRequestsInput) ([]api.PullRequest, error) {
			return prs, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runPRList(context.Background(), opts, env); err != nil {
		t.Fatalf("runPRList 返回错误: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "#1") || strings.Contains(output, "#2") {
		t.Errorf("@me 应仅保留 self 的 PR，实际: %s", output)
	}
}

// TestRunPRListAtMeNoUser 验证 @me 但配置缺少 user 时报错。
func TestRunPRListAtMeNoUser(t *testing.T) {
	opts := prListOptions{state: "open", direction: "desc", sort: "created", limit: 30, author: "@me"}
	env := prListEnv{
		git: newPRListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok", User: ""}, nil
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runPRList(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "@me") {
		t.Errorf("期望 @me 解析错误，实际: %v", err)
	}
}

// TestRunPRListAPIError 验证 API 错误的传递。
func TestRunPRListAPIError(t *testing.T) {
	opts := prListOptions{state: "open", direction: "desc", sort: "created", limit: 30}
	env := prListEnv{
		git: newPRListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		listPRs: func(ctx context.Context, host, token, owner, repo string, input *api.ListPullRequestsInput) ([]api.PullRequest, error) {
			return nil, errors.New("boom")
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runPRList(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "查询 PR 列表失败") {
		t.Errorf("期望 API 错误传递，实际: %v", err)
	}
}

// TestRunPRListColorOnTTY 验证 TTY 时启用颜色，no-color 标志强制关闭。
func TestRunPRListColorOnTTY(t *testing.T) {
	prs := []api.PullRequest{
		makePR(1, "x", "open", "feat", "main", "alice", fixedNow),
	}
	cases := []struct {
		name     string
		isTTY    bool
		noColor  bool
		wantANSI bool
	}{
		{"tty enables color", true, false, true},
		{"non-tty no color", false, false, false},
		{"tty with no-color flag", true, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			opts := prListOptions{state: "open", direction: "desc", sort: "created", limit: 30, noColor: tc.noColor}
			env := prListEnv{
				git: newPRListGitFake(),
				loadConfig: func() (*config.Config, error) {
					return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
				},
				listPRs: func(ctx context.Context, host, token, owner, repo string, input *api.ListPullRequestsInput) ([]api.PullRequest, error) {
					return prs, nil
				},
				out:   out,
				isTTY: func() bool { return tc.isTTY },
				now:   func() time.Time { return fixedNow },
			}
			if err := runPRList(context.Background(), opts, env); err != nil {
				t.Fatalf("runPRList 返回错误: %v", err)
			}
			hasANSI := strings.Contains(out.String(), "\033[")
			if hasANSI != tc.wantANSI {
				t.Errorf("hasANSI = %v, 期望 %v\n输出: %q", hasANSI, tc.wantANSI, out.String())
			}
		})
	}
}

// TestRunPRListLimitTrim 验证客户端按 author 过滤后再裁剪到 limit。
func TestRunPRListLimitTrim(t *testing.T) {
	var prs []api.PullRequest
	for i := int64(1); i <= 5; i++ {
		prs = append(prs, makePR(i, fmt.Sprintf("PR%d", i), "open", fmt.Sprintf("f%d", i), "main", "alice", fixedNow))
	}
	out := &bytes.Buffer{}
	opts := prListOptions{state: "open", direction: "desc", sort: "created", limit: 2, author: "alice"}
	env := prListEnv{
		git: newPRListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		listPRs: func(ctx context.Context, host, token, owner, repo string, input *api.ListPullRequestsInput) ([]api.PullRequest, error) {
			if input.PerPage != 2 {
				t.Errorf("PerPage = %d, 期望 2", input.PerPage)
			}
			return prs, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runPRList(context.Background(), opts, env); err != nil {
		t.Fatalf("runPRList 返回错误: %v", err)
	}
	if !strings.Contains(out.String(), "Showing 2 pull requests") {
		t.Errorf("期望显示 2 条，实际: %s", out.String())
	}
}

// TestRelativeTime 验证相对时间格式化。
func TestRelativeTime(t *testing.T) {
	now := fixedNow
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"just now", now.Add(-30 * time.Second), "刚刚"},
		{"minutes", now.Add(-5 * time.Minute), "5 分钟前"},
		{"hours", now.Add(-3 * time.Hour), "3 小时前"},
		{"days", now.Add(-2 * 24 * time.Hour), "2 天前"},
		{"months", now.Add(-60 * 24 * time.Hour), "2 个月前"},
		{"years", now.Add(-2 * 365 * 24 * time.Hour), "2 年前"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := relativeTime(tc.t.Format(time.RFC3339), now)
			if got != tc.want {
				t.Errorf("relativeTime = %q, 期望 %q", got, tc.want)
			}
		})
	}
}

// TestRelativeTimeInvalid 验证非 RFC3339 字符串原样返回。
func TestRelativeTimeInvalid(t *testing.T) {
	if got := relativeTime("not-a-time", fixedNow); got != "not-a-time" {
		t.Errorf("relativeTime 应原样返回无效输入，实际: %q", got)
	}
	if got := relativeTime("", fixedNow); got != "-" {
		t.Errorf("relativeTime 空字符串应返回 -，实际: %q", got)
	}
}

// TestTruncate 验证字符串截断。
func TestTruncate(t *testing.T) {
	cases := []struct {
		in, want string
		max      int
	}{
		{"hello", "hello", 10},
		{"hello world", "hello wor…", 10},
		{"中文测试很长很长很长", "中文测试…", 5},
		{"x", "x", 1},
	}
	for _, tc := range cases {
		got := truncate(tc.in, tc.max)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, 期望 %q", tc.in, tc.max, got, tc.want)
		}
	}
}

// TestColorizeState 验证状态着色。
func TestColorizeState(t *testing.T) {
	cases := []struct {
		state string
		color string
	}{
		{"open", colorGreen},
		{"merged", colorMagenta},
		{"closed", colorRed},
		{"progressing", colorYellow},
		{"unknown", colorGray},
	}
	for _, tc := range cases {
		got := colorizeState(tc.state, true)
		if !strings.Contains(got, tc.color) {
			t.Errorf("colorizeState(%q) 应包含颜色 %q，实际: %q", tc.state, tc.color, got)
		}
	}
	if got := colorizeState("open", false); strings.Contains(got, "\033[") {
		t.Errorf("noColor 模式不应有 ANSI 码，实际: %q", got)
	}
}

// TestRunPRListIntegration 用 httptest 验证 ListPullRequests 真实通过 HTTP 客户端工作。
func TestRunPRListIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/pulls" {
			t.Errorf("路径 = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("state") != "merged" {
			t.Errorf("state = %q, 期望 merged", q.Get("state"))
		}
		if q.Get("per_page") != "5" {
			t.Errorf("per_page = %q, 期望 5", q.Get("per_page"))
		}
		if q.Get("access_token") != "tok" {
			t.Errorf("access_token = %q, 期望 tok", q.Get("access_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":1,"number":42,"state":"merged","title":"Done","html_url":"https://gitee.com/owner/repo/pulls/42",
			"user":{"id":1,"login":"alice"},"head":{"ref":"feat","label":"o:feat","sha":"x"},
			"base":{"ref":"main","label":"o:main","sha":"y"},"created_at":"2026-06-09T12:00:00Z","updated_at":"2026-06-10T11:00:00Z"}
		]`))
	}))
	defer srv.Close()

	out := &bytes.Buffer{}
	opts := prListOptions{state: "merged", direction: "desc", sort: "created", limit: 5}
	env := prListEnv{
		git: newPRListGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: srv.URL, Token: "tok"}, nil
		},
		listPRs: func(ctx context.Context, host, token, owner, repo string, input *api.ListPullRequestsInput) ([]api.PullRequest, error) {
			client := api.NewClient(host, token)
			return client.ListPullRequests(ctx, owner, repo, input)
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runPRList(context.Background(), opts, env); err != nil {
		t.Fatalf("runPRList 返回错误: %v", err)
	}
	if !strings.Contains(out.String(), "#42") || !strings.Contains(out.String(), "MERGED") {
		t.Errorf("期望输出 PR #42 MERGED，实际: %s", out.String())
	}
}
