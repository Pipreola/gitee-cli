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

// --- repo create ---

// TestRunRepoCreateNoName 验证缺少仓库名称时报错。
func TestRunRepoCreateNoName(t *testing.T) {
	env := repoCreateEnv{
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		out:        &bytes.Buffer{},
	}
	err := runRepoCreate(context.Background(), repoCreateOptions{}, env)
	if err == nil || !strings.Contains(err.Error(), "仓库名称") {
		t.Errorf("期望仓库名称为空错误，实际: %v", err)
	}
}

// TestRunRepoCreateNoAuth 验证未登录时报错。
func TestRunRepoCreateNoAuth(t *testing.T) {
	env := repoCreateEnv{
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: ""}, nil },
		out:        &bytes.Buffer{},
	}
	err := runRepoCreate(context.Background(), repoCreateOptions{name: "repo"}, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunRepoCreateSuccess 验证创建成功路径，并校验传递的参数。
func TestRunRepoCreateSuccess(t *testing.T) {
	out := &bytes.Buffer{}
	gotOrg := ""
	var gotInput *api.CreateRepositoryInput
	env := repoCreateEnv{
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		createRepo: func(ctx context.Context, host, token, org string, input *api.CreateRepositoryInput) (*api.Repository, error) {
			gotOrg = org
			gotInput = input
			return &api.Repository{FullName: "me/repo", Name: "repo", Private: true, HTMLURL: "https://gitee.com/me/repo", Description: "d"}, nil
		},
		out: out,
	}
	opts := repoCreateOptions{name: "repo", description: "d", private: true}
	if err := runRepoCreate(context.Background(), opts, env); err != nil {
		t.Fatalf("runRepoCreate 错误: %v", err)
	}
	if gotOrg != "" {
		t.Errorf("org = %q, 期望空", gotOrg)
	}
	if gotInput.Name != "repo" || !gotInput.Private || gotInput.Description != "d" {
		t.Errorf("input = %+v, 不符合预期", gotInput)
	}
	for _, want := range []string{"创建成功", "me/repo", "private", "https://gitee.com/me/repo"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("输出缺少 %q\n实际:\n%s", want, out.String())
		}
	}
}

// TestRunRepoCreateOrg 验证 --org 透传到 API 层。
func TestRunRepoCreateOrg(t *testing.T) {
	gotOrg := ""
	env := repoCreateEnv{
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		createRepo: func(ctx context.Context, host, token, org string, input *api.CreateRepositoryInput) (*api.Repository, error) {
			gotOrg = org
			return &api.Repository{FullName: "my-org/repo", Name: "repo"}, nil
		},
		out: &bytes.Buffer{},
	}
	if err := runRepoCreate(context.Background(), repoCreateOptions{name: "repo", org: "my-org"}, env); err != nil {
		t.Fatalf("runRepoCreate 错误: %v", err)
	}
	if gotOrg != "my-org" {
		t.Errorf("org = %q, 期望 my-org", gotOrg)
	}
}

// TestRunRepoCreateJSON 验证 --json 输出。
func TestRunRepoCreateJSON(t *testing.T) {
	out := &bytes.Buffer{}
	env := repoCreateEnv{
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		createRepo: func(ctx context.Context, host, token, org string, input *api.CreateRepositoryInput) (*api.Repository, error) {
			return &api.Repository{FullName: "me/repo", Name: "repo"}, nil
		},
		out: out,
	}
	if err := runRepoCreate(context.Background(), repoCreateOptions{name: "repo", jsonOut: true}, env); err != nil {
		t.Fatalf("runRepoCreate 错误: %v", err)
	}
	s := strings.TrimSpace(out.String())
	if !strings.HasPrefix(s, "{") || !strings.Contains(s, `"full_name": "me/repo"`) {
		t.Errorf("期望 JSON 输出含 full_name，实际:\n%s", out.String())
	}
}

// TestRunRepoCreateAPIError 验证 API 错误被透传。
func TestRunRepoCreateAPIError(t *testing.T) {
	env := repoCreateEnv{
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		createRepo: func(ctx context.Context, host, token, org string, input *api.CreateRepositoryInput) (*api.Repository, error) {
			return nil, errors.New("422 Unprocessable")
		},
		out: &bytes.Buffer{},
	}
	err := runRepoCreate(context.Background(), repoCreateOptions{name: "repo"}, env)
	if err == nil || !strings.Contains(err.Error(), "422 Unprocessable") {
		t.Errorf("期望透传 422 错误，实际: %v", err)
	}
}

// --- repo fork ---

// TestRunRepoForkNoAuth 验证未登录时报错。
func TestRunRepoForkNoAuth(t *testing.T) {
	env := repoForkEnv{
		git:        &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: ""}, nil },
		out:        &bytes.Buffer{},
	}
	err := runRepoFork(context.Background(), repoForkOptions{}, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunRepoForkFromCurrentRepo 验证从 git remote 推断源仓库。
func TestRunRepoForkFromCurrentRepo(t *testing.T) {
	out := &bytes.Buffer{}
	gotOwner, gotRepo := "", ""
	env := repoForkEnv{
		git:        &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		forkRepo: func(ctx context.Context, host, token, owner, repo string, input *api.ForkRepositoryInput) (*api.Repository, error) {
			gotOwner, gotRepo = owner, repo
			return &api.Repository{FullName: "me/repo", Name: "repo", Fork: true, HTMLURL: "https://gitee.com/me/repo"}, nil
		},
		out: out,
	}
	if err := runRepoFork(context.Background(), repoForkOptions{}, env); err != nil {
		t.Fatalf("runRepoFork 错误: %v", err)
	}
	if gotOwner != "owner" || gotRepo != "repo" {
		t.Errorf("fork 参数 = (%s,%s), 期望 (owner,repo)", gotOwner, gotRepo)
	}
	for _, want := range []string{"Fork", "me/repo", "https://gitee.com/me/repo"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("输出缺少 %q\n实际:\n%s", want, out.String())
		}
	}
}

// TestRunRepoForkFromArg 验证显式 owner/repo 参数覆盖 git 推断。
func TestRunRepoForkFromArg(t *testing.T) {
	gotOwner, gotRepo := "", ""
	env := repoForkEnv{
		git:        &fakeGitRunner{remoteURL: "https://gitee.com/wrong/wrong.git"},
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		forkRepo: func(ctx context.Context, host, token, owner, repo string, input *api.ForkRepositoryInput) (*api.Repository, error) {
			gotOwner, gotRepo = owner, repo
			return &api.Repository{FullName: "me/cool", Name: "cool"}, nil
		},
		out: &bytes.Buffer{},
	}
	if err := runRepoFork(context.Background(), repoForkOptions{repo: "alice/cool"}, env); err != nil {
		t.Fatalf("runRepoFork 错误: %v", err)
	}
	if gotOwner != "alice" || gotRepo != "cool" {
		t.Errorf("fork 参数 = (%s,%s), 期望 (alice,cool)", gotOwner, gotRepo)
	}
}

// TestRunRepoForkWithOrg 验证 --org 构造非 nil input。
func TestRunRepoForkWithOrg(t *testing.T) {
	var gotInput *api.ForkRepositoryInput
	env := repoForkEnv{
		git:        &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		forkRepo: func(ctx context.Context, host, token, owner, repo string, input *api.ForkRepositoryInput) (*api.Repository, error) {
			gotInput = input
			return &api.Repository{FullName: "my-org/repo", Name: "repo"}, nil
		},
		out: &bytes.Buffer{},
	}
	if err := runRepoFork(context.Background(), repoForkOptions{repo: "owner/repo", org: "my-org"}, env); err != nil {
		t.Fatalf("runRepoFork 错误: %v", err)
	}
	if gotInput == nil || gotInput.Organization != "my-org" {
		t.Errorf("input = %+v, 期望 Organization=my-org", gotInput)
	}
}

// TestRunRepoForkNoOptInput 验证未指定 org/name 时 input 为 nil。
func TestRunRepoForkNoOptInput(t *testing.T) {
	sawNil := false
	env := repoForkEnv{
		git:        &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		forkRepo: func(ctx context.Context, host, token, owner, repo string, input *api.ForkRepositoryInput) (*api.Repository, error) {
			sawNil = input == nil
			return &api.Repository{FullName: "me/repo", Name: "repo"}, nil
		},
		out: &bytes.Buffer{},
	}
	if err := runRepoFork(context.Background(), repoForkOptions{repo: "owner/repo"}, env); err != nil {
		t.Fatalf("runRepoFork 错误: %v", err)
	}
	if !sawNil {
		t.Error("未指定 org/name 时期望 input 为 nil")
	}
}

// TestRunRepoForkAPIError 验证 API 错误被透传。
func TestRunRepoForkAPIError(t *testing.T) {
	env := repoForkEnv{
		git:        &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		forkRepo: func(ctx context.Context, host, token, owner, repo string, input *api.ForkRepositoryInput) (*api.Repository, error) {
			return nil, errors.New("403 Forbidden")
		},
		out: &bytes.Buffer{},
	}
	err := runRepoFork(context.Background(), repoForkOptions{repo: "owner/repo"}, env)
	if err == nil || !strings.Contains(err.Error(), "403 Forbidden") {
		t.Errorf("期望透传 403 错误，实际: %v", err)
	}
}

// --- repo list ---

// TestRunRepoListNoAuth 验证未登录时报错。
func TestRunRepoListNoAuth(t *testing.T) {
	env := repoListEnv{
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: ""}, nil },
		out:        &bytes.Buffer{},
		isTTY:      func() bool { return false },
		now:        func() time.Time { return fixedNow },
	}
	err := runRepoList(context.Background(), repoListOptions{limit: 30}, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunRepoListUser 验证列出当前用户仓库并渲染表格。
func TestRunRepoListUser(t *testing.T) {
	out := &bytes.Buffer{}
	gotOrg := "unset"
	env := repoListEnv{
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		listRepos: func(ctx context.Context, host, token, org string, input *api.ListRepositoriesInput) ([]api.Repository, error) {
			gotOrg = org
			return []api.Repository{
				{FullName: "me/a", Name: "a", Language: "Go", PushedAt: fixedNow.Add(-2 * time.Hour).Format(time.RFC3339)},
				{FullName: "me/b", Name: "b", Private: true},
			}, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runRepoList(context.Background(), repoListOptions{limit: 30}, env); err != nil {
		t.Fatalf("runRepoList 错误: %v", err)
	}
	if gotOrg != "" {
		t.Errorf("org = %q, 期望空", gotOrg)
	}
	for _, want := range []string{"me/a", "me/b", "Go", "public", "private", "2 小时前"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("输出缺少 %q\n实际:\n%s", want, out.String())
		}
	}
}

// TestRunRepoListOrg 验证 --org 透传，并显示组织作用域。
func TestRunRepoListOrg(t *testing.T) {
	out := &bytes.Buffer{}
	gotOrg := ""
	env := repoListEnv{
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		listRepos: func(ctx context.Context, host, token, org string, input *api.ListRepositoriesInput) ([]api.Repository, error) {
			gotOrg = org
			return []api.Repository{{FullName: "my-org/a", Name: "a"}}, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runRepoList(context.Background(), repoListOptions{org: "my-org", limit: 30}, env); err != nil {
		t.Fatalf("runRepoList 错误: %v", err)
	}
	if gotOrg != "my-org" {
		t.Errorf("org = %q, 期望 my-org", gotOrg)
	}
	if !strings.Contains(out.String(), "my-org") {
		t.Errorf("输出应包含组织名，实际:\n%s", out.String())
	}
}

// TestRunRepoListEmpty 验证空列表提示。
func TestRunRepoListEmpty(t *testing.T) {
	out := &bytes.Buffer{}
	env := repoListEnv{
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		listRepos: func(ctx context.Context, host, token, org string, input *api.ListRepositoriesInput) ([]api.Repository, error) {
			return []api.Repository{}, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runRepoList(context.Background(), repoListOptions{limit: 30}, env); err != nil {
		t.Fatalf("runRepoList 错误: %v", err)
	}
	if !strings.Contains(out.String(), "没有仓库") {
		t.Errorf("期望空列表提示，实际:\n%s", out.String())
	}
}

// TestRunRepoListJSON 验证 --json 输出。
func TestRunRepoListJSON(t *testing.T) {
	out := &bytes.Buffer{}
	env := repoListEnv{
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		listRepos: func(ctx context.Context, host, token, org string, input *api.ListRepositoriesInput) ([]api.Repository, error) {
			return []api.Repository{{FullName: "me/a", Name: "a"}}, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runRepoList(context.Background(), repoListOptions{limit: 30, jsonOut: true}, env); err != nil {
		t.Fatalf("runRepoList 错误: %v", err)
	}
	s := strings.TrimSpace(out.String())
	if !strings.HasPrefix(s, "[") || !strings.Contains(s, `"full_name": "me/a"`) {
		t.Errorf("期望 JSON 数组含 full_name，实际:\n%s", out.String())
	}
}

// TestRunRepoListLimitClamp 验证客户端兜底裁剪超出 limit 的结果。
func TestRunRepoListLimitClamp(t *testing.T) {
	out := &bytes.Buffer{}
	env := repoListEnv{
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: "tok"}, nil },
		listRepos: func(ctx context.Context, host, token, org string, input *api.ListRepositoriesInput) ([]api.Repository, error) {
			return []api.Repository{
				{FullName: "me/a", Name: "a"},
				{FullName: "me/b", Name: "b"},
				{FullName: "me/c", Name: "c"},
			}, nil
		},
		out:   out,
		isTTY: func() bool { return false },
		now:   func() time.Time { return fixedNow },
	}
	if err := runRepoList(context.Background(), repoListOptions{limit: 2}, env); err != nil {
		t.Fatalf("runRepoList 错误: %v", err)
	}
	if strings.Contains(out.String(), "me/c") {
		t.Errorf("limit=2 时不应包含第三个仓库，实际:\n%s", out.String())
	}
}

// TestValidateRepoListOptions 验证参数校验。
func TestValidateRepoListOptions(t *testing.T) {
	cases := []struct {
		name    string
		opts    repoListOptions
		wantErr bool
	}{
		{"默认合法", repoListOptions{limit: 30}, false},
		{"visibility 合法", repoListOptions{limit: 30, visibility: "private"}, false},
		{"visibility 非法", repoListOptions{limit: 30, visibility: "bogus"}, true},
		{"sort 合法", repoListOptions{limit: 30, sort: "pushed"}, false},
		{"sort 非法", repoListOptions{limit: 30, sort: "bogus"}, true},
		{"direction 非法", repoListOptions{limit: 30, direction: "up"}, true},
		{"limit 过小", repoListOptions{limit: 0}, true},
		{"limit 过大", repoListOptions{limit: 101}, true},
	}
	for _, c := range cases {
		opts := c.opts
		err := validateRepoListOptions(&opts)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: validateRepoListOptions 返回 %v, wantErr=%v", c.name, err, c.wantErr)
		}
	}
}
