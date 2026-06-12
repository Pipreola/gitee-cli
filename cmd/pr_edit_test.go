package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// TestRunPREditNoAuth 验证未登录时报错。
func TestRunPREditNoAuth(t *testing.T) {
	env := prEditEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: ""}, nil
		},
		out: &bytes.Buffer{},
	}
	opts := prEditOptions{number: 123, titleChanged: true, title: "t"}
	err := runPREdit(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunPREditNoFieldsSpecified 验证未指定任何字段时报错。
func TestRunPREditNoFieldsSpecified(t *testing.T) {
	env := prEditEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		out: &bytes.Buffer{},
	}
	opts := prEditOptions{number: 123} // 没有任何 *Changed 标志
	err := runPREdit(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "至少需要指定一个待修改字段") {
		t.Errorf("期望字段校验错误，实际: %v", err)
	}
}

// TestRunPREditTitleOnly 验证仅修改标题的成功场景。
func TestRunPREditTitleOnly(t *testing.T) {
	out := &bytes.Buffer{}
	env := prEditEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		editPR: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.EditPullRequestInput) (*api.PullRequest, error) {
			if number != 123 {
				t.Errorf("number = %d, 期望 123", number)
			}
			if input.Title == nil || *input.Title != "新标题" {
				t.Errorf("Title = %v, 期望指针指向 '新标题'", input.Title)
			}
			if input.Body != nil {
				t.Errorf("Body 应为 nil（未指定），实际: %v", input.Body)
			}
			return &api.PullRequest{
				Number:  123,
				Title:   "新标题",
				HTMLURL: "https://gitee.com/owner/repo/pulls/123",
			}, nil
		},
		out: out,
	}
	opts := prEditOptions{number: 123, titleChanged: true, title: "新标题"}
	err := runPREdit(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runPREdit 返回错误: %v", err)
	}
	output := out.String()
	wantStrings := []string{"PR #123", "已更新", "新标题"}
	for _, want := range wantStrings {
		if !strings.Contains(output, want) {
			t.Errorf("输出缺少 %q\n完整输出:\n%s", want, output)
		}
	}
}

// TestRunPREditMultipleFields 验证同时修改多个字段。
func TestRunPREditMultipleFields(t *testing.T) {
	out := &bytes.Buffer{}
	env := prEditEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		editPR: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.EditPullRequestInput) (*api.PullRequest, error) {
			if input.Title == nil || *input.Title != "t" {
				t.Errorf("Title = %v, 期望 t", input.Title)
			}
			if input.Body == nil || *input.Body != "b" {
				t.Errorf("Body = %v, 期望 b", input.Body)
			}
			if input.Labels == nil || *input.Labels != "bug" {
				t.Errorf("Labels = %v, 期望 bug", input.Labels)
			}
			if input.Assignees != nil {
				t.Errorf("Assignees 应为 nil（未指定），实际: %v", input.Assignees)
			}
			return &api.PullRequest{Number: 456, Title: "t", HTMLURL: "u"}, nil
		},
		out: out,
	}
	opts := prEditOptions{
		number:        456,
		titleChanged:  true,
		title:         "t",
		bodyChanged:   true,
		body:          "b",
		labelsChanged: true,
		labels:        "bug",
	}
	err := runPREdit(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runPREdit 返回错误: %v", err)
	}
}

// TestRunPREditClearLabels 验证传空字符串可清空标签（区别于不修改）。
func TestRunPREditClearLabels(t *testing.T) {
	out := &bytes.Buffer{}
	env := prEditEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		editPR: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.EditPullRequestInput) (*api.PullRequest, error) {
			if input.Labels == nil {
				t.Errorf("Labels 应为非 nil 指针（清空意图）")
			}
			if input.Labels != nil && *input.Labels != "" {
				t.Errorf("Labels = %q, 期望空字符串", *input.Labels)
			}
			return &api.PullRequest{Number: 1, Title: "t", HTMLURL: "u"}, nil
		},
		out: out,
	}
	opts := prEditOptions{number: 1, labelsChanged: true, labels: ""}
	err := runPREdit(context.Background(), opts, env)
	if err != nil {
		t.Fatalf("runPREdit 返回错误: %v", err)
	}
}

// TestRunPREditAPIError 验证 API 错误向上传播。
func TestRunPREditAPIError(t *testing.T) {
	env := prEditEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		editPR: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.EditPullRequestInput) (*api.PullRequest, error) {
			return nil, errors.New("权限不足")
		},
		out: &bytes.Buffer{},
	}
	opts := prEditOptions{number: 1, titleChanged: true, title: "t"}
	err := runPREdit(context.Background(), opts, env)
	if err == nil {
		t.Fatal("期望错误但没有返回")
	}
	if !strings.Contains(err.Error(), "编辑 PR 失败") {
		t.Errorf("期望错误消息包含 '编辑 PR 失败'，实际: %v", err)
	}
}

// TestRunPREditGetRepoError 验证获取仓库信息失败时的错误处理。
func TestRunPREditGetRepoError(t *testing.T) {
	env := prEditEnv{
		git: &fakeGitRunner{remoteURL: ""}, // 触发获取仓库错误
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		out: &bytes.Buffer{},
	}
	opts := prEditOptions{number: 1, titleChanged: true, title: "t"}
	err := runPREdit(context.Background(), opts, env)
	if err == nil || !strings.Contains(err.Error(), "获取仓库信息失败") {
		t.Errorf("期望仓库信息错误，实际: %v", err)
	}
}
