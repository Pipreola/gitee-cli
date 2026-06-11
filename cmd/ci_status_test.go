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

// newCIStatusGitFake 返回一个仓库 URL 和当前分支已配置的 fakeGitRunner。
func newCIStatusGitFake() *fakeGitRunner {
	return &fakeGitRunner{
		remoteURL:     "https://gitee.com/testowner/testrepo.git",
		currentBranch: "main",
	}
}

// makeCombinedStatus 构造测试用的聚合 CI 状态。
func makeCombinedStatus(state string, statuses []api.CIStatus) *api.CombinedStatus {
	return &api.CombinedStatus{
		State:      state,
		TotalCount: len(statuses),
		Statuses:   statuses,
	}
}

// makeCIStatus 构造单个 CI 状态记录。
func makeCIStatus(state, context, desc, targetURL string) api.CIStatus {
	return api.CIStatus{
		ID:          1,
		State:       state,
		Context:     context,
		Description: desc,
		TargetURL:   targetURL,
		CreatedAt:   fixedNow.Add(-30 * time.Minute).Format(time.RFC3339),
		UpdatedAt:   fixedNow.Add(-5 * time.Minute).Format(time.RFC3339),
	}
}

// TestRunCIStatusNoAuth 验证未登录时报错。
func TestRunCIStatusNoAuth(t *testing.T) {
	env := ciStatusEnv{
		git: newCIStatusGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: ""}, nil
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runCIStatus(context.Background(), ciStatusOptions{}, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunCIStatusGetRepoError 验证获取仓库信息失败时的错误处理。
func TestRunCIStatusGetRepoError(t *testing.T) {
	env := ciStatusEnv{
		git: &fakeGitRunner{remoteURL: ""}, // 触发错误
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok", Host: "https://gitee.com/api/v5"}, nil
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runCIStatus(context.Background(), ciStatusOptions{}, env)
	if err == nil || !strings.Contains(err.Error(), "获取仓库信息失败") {
		t.Errorf("期望仓库信息错误，实际: %v", err)
	}
}

// TestRunCIStatusNoCurrentBranch 验证没有当前分支时报错。
func TestRunCIStatusNoCurrentBranch(t *testing.T) {
	env := ciStatusEnv{
		git: &fakeGitRunner{
			remoteURL:     "https://gitee.com/owner/repo.git",
			currentBranch: "", // 触发错误
		},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok", Host: "https://gitee.com/api/v5"}, nil
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runCIStatus(context.Background(), ciStatusOptions{}, env)
	if err == nil || !strings.Contains(err.Error(), "获取当前分支失败") {
		t.Errorf("期望分支错误，实际: %v", err)
	}
}

// TestRunCIStatusAPIError 验证 API 错误时的错误处理。
func TestRunCIStatusAPIError(t *testing.T) {
	env := ciStatusEnv{
		git:        newCIStatusGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok", Host: "https://gitee.com/api/v5"}, nil
		},
		getCombinedStatus: func(ctx context.Context, host, token, owner, repo, ref string) (*api.CombinedStatus, error) {
			return nil, errors.New("网络超时")
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runCIStatus(context.Background(), ciStatusOptions{}, env)
	if err == nil || !strings.Contains(err.Error(), "查询 CI 状态失败") {
		t.Errorf("期望 API 错误，实际: %v", err)
	}
}

// TestRunCIStatusNotFound 验证 404 错误给出友好提示。
func TestRunCIStatusNotFound(t *testing.T) {
	env := ciStatusEnv{
		git:        newCIStatusGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok", Host: "https://gitee.com/api/v5"}, nil
		},
		getCombinedStatus: func(ctx context.Context, host, token, owner, repo, ref string) (*api.CombinedStatus, error) {
			return nil, &api.APIError{StatusCode: 404, Message: "not found"}
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runCIStatus(context.Background(), ciStatusOptions{}, env)
	if err == nil || !strings.Contains(err.Error(), "未找到") {
		t.Errorf("期望 404 友好错误，实际: %v", err)
	}
}

// TestRunCIStatusSuccess 验证成功输出包含关键信息。
func TestRunCIStatusSuccess(t *testing.T) {
	combined := makeCombinedStatus("success", []api.CIStatus{
		makeCIStatus("success", "jenkins", "Build passed", "https://jenkins.example.com/job/1"),
		makeCIStatus("success", "sonar", "Quality gate passed", ""),
	})

	out := &bytes.Buffer{}
	env := ciStatusEnv{
		git:        newCIStatusGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok", Host: "https://gitee.com/api/v5"}, nil
		},
		getCombinedStatus: func(ctx context.Context, host, token, owner, repo, ref string) (*api.CombinedStatus, error) {
			return combined, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runCIStatus(context.Background(), ciStatusOptions{}, env)
	if err != nil {
		t.Fatalf("runCIStatus 返回错误: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "testowner/testrepo") {
		t.Errorf("期望输出包含仓库信息，实际: %s", output)
	}
	if !strings.Contains(output, "success") {
		t.Errorf("期望输出包含 success，实际: %s", output)
	}
	if !strings.Contains(output, "jenkins") {
		t.Errorf("期望输出包含 jenkins context，实际: %s", output)
	}
	if !strings.Contains(output, "sonar") {
		t.Errorf("期望输出包含 sonar context，实际: %s", output)
	}
}

// TestRunCIStatusWithBranchFlag 验证 --branch 参数会正确传递。
func TestRunCIStatusWithBranchFlag(t *testing.T) {
	var capturedRef string
	out := &bytes.Buffer{}
	env := ciStatusEnv{
		git:        newCIStatusGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok", Host: "https://gitee.com/api/v5"}, nil
		},
		getCombinedStatus: func(ctx context.Context, host, token, owner, repo, ref string) (*api.CombinedStatus, error) {
			capturedRef = ref
			return makeCombinedStatus("pending", []api.CIStatus{}), nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runCIStatus(context.Background(), ciStatusOptions{branch: "develop"}, env)
	if err != nil {
		t.Fatalf("runCIStatus 返回错误: %v", err)
	}
	if capturedRef != "develop" {
		t.Errorf("capturedRef = %q, 期望 develop", capturedRef)
	}
}

// TestRunCIStatusWithRefFlag 验证 --ref 参数优先级高于 --branch。
func TestRunCIStatusWithRefFlag(t *testing.T) {
	var capturedRef string
	out := &bytes.Buffer{}
	env := ciStatusEnv{
		git:        newCIStatusGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok", Host: "https://gitee.com/api/v5"}, nil
		},
		getCombinedStatus: func(ctx context.Context, host, token, owner, repo, ref string) (*api.CombinedStatus, error) {
			capturedRef = ref
			return makeCombinedStatus("success", []api.CIStatus{}), nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runCIStatus(context.Background(), ciStatusOptions{ref: "abc1234", branch: "develop"}, env)
	if err != nil {
		t.Fatalf("runCIStatus 返回错误: %v", err)
	}
	if capturedRef != "abc1234" {
		t.Errorf("capturedRef = %q, 期望 abc1234（--ref 优先级更高）", capturedRef)
	}
}

// TestRunCIStatusEmptyStatuses 验证无 CI 记录时给出提示。
func TestRunCIStatusEmptyStatuses(t *testing.T) {
	out := &bytes.Buffer{}
	env := ciStatusEnv{
		git:        newCIStatusGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok", Host: "https://gitee.com/api/v5"}, nil
		},
		getCombinedStatus: func(ctx context.Context, host, token, owner, repo, ref string) (*api.CombinedStatus, error) {
			return makeCombinedStatus("", []api.CIStatus{}), nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runCIStatus(context.Background(), ciStatusOptions{}, env)
	if err != nil {
		t.Fatalf("runCIStatus 返回错误: %v", err)
	}
	if !strings.Contains(out.String(), "暂无 CI 状态记录") {
		t.Errorf("期望无记录提示，实际: %s", out.String())
	}
}

// TestRunCIStatusJSONOutput 验证 --json 输出有效 JSON。
func TestRunCIStatusJSONOutput(t *testing.T) {
	combined := makeCombinedStatus("success", []api.CIStatus{
		makeCIStatus("success", "jenkins", "Build passed", ""),
	})

	out := &bytes.Buffer{}
	env := ciStatusEnv{
		git:        newCIStatusGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok", Host: "https://gitee.com/api/v5"}, nil
		},
		getCombinedStatus: func(ctx context.Context, host, token, owner, repo, ref string) (*api.CombinedStatus, error) {
			return combined, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runCIStatus(context.Background(), ciStatusOptions{jsonOut: true}, env)
	if err != nil {
		t.Fatalf("runCIStatus 返回错误: %v", err)
	}
	if !strings.Contains(out.String(), `"state"`) || !strings.Contains(out.String(), `"statuses"`) {
		t.Errorf("期望有效 JSON 输出，实际: %s", out.String())
	}
}

// TestRunCIStatusWebFlag 验证 --web 模式打开浏览器。
func TestRunCIStatusWebFlag(t *testing.T) {
	var capturedURL string
	out := &bytes.Buffer{}
	env := ciStatusEnv{
		git:        newCIStatusGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok", Host: "https://gitee.com/api/v5"}, nil
		},
		openBrowser: func(url string) error {
			capturedURL = url
			return nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	err := runCIStatus(context.Background(), ciStatusOptions{web: true}, env)
	if err != nil {
		t.Fatalf("runCIStatus 返回错误: %v", err)
	}
	if !strings.Contains(capturedURL, "testowner") || !strings.Contains(capturedURL, "testrepo") {
		t.Errorf("capturedURL = %q, 期望包含 owner/repo", capturedURL)
	}
}

// TestFormatCIState 验证状态格式化逻辑（无颜色模式）。
func TestFormatCIState(t *testing.T) {
	cases := []struct {
		state string
		want  string
	}{
		{"success", "✔ success"},
		{"failed", "✖ failed"},
		{"failure", "✖ failed"},
		{"error", "✖ error"},
		{"pending", "● pending"},
		{"running", "● running"},
		{"canceled", "○ canceled"},
		{"cancelled", "○ canceled"},
		{"", "○ unknown"},
	}
	for _, tc := range cases {
		got := formatCIState(tc.state, false)
		if got != tc.want {
			t.Errorf("formatCIState(%q) = %q, 期望 %q", tc.state, got, tc.want)
		}
	}
}

// TestBuildCIPageURL 验证 CI 页面 URL 构造。
func TestBuildCIPageURL(t *testing.T) {
	cases := []struct {
		host  string
		owner string
		repo  string
		ref   string
		want  string
	}{
		{
			host:  "https://gitee.com/api/v5",
			owner: "alice",
			repo:  "myrepo",
			ref:   "main",
			want:  "https://gitee.com/alice/myrepo/commits/main",
		},
		{
			host:  "https://gitee.com",
			owner: "bob",
			repo:  "his-repo",
			ref:   "abc1234",
			want:  "https://gitee.com/bob/his-repo/commits/abc1234",
		},
	}
	for _, tc := range cases {
		got := buildCIPageURL(tc.host, tc.owner, tc.repo, tc.ref)
		if got != tc.want {
			t.Errorf("buildCIPageURL(%q, %q, %q, %q) = %q, 期望 %q",
				tc.host, tc.owner, tc.repo, tc.ref, got, tc.want)
		}
	}
}

// TestIsNotFoundError 验证 404 错误检测。
func TestIsNotFoundError(t *testing.T) {
	if !isNotFoundError(&api.APIError{StatusCode: 404, Message: "not found"}) {
		t.Error("期望 404 被识别为 not found 错误")
	}
	if isNotFoundError(&api.APIError{StatusCode: 500, Message: "internal error"}) {
		t.Error("期望 500 不被识别为 not found 错误")
	}
	if isNotFoundError(errors.New("普通错误")) {
		t.Error("期望普通错误不被识别为 not found 错误")
	}
}
