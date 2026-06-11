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

func TestRunPRMerge(t *testing.T) {
	tests := []struct {
		name       string
		opts       prMergeOptions
		mockConfig *config.Config
		mockPR     *api.PullRequest
		mockErr    error
		mergeErr   error
		wantErr    string
	}{
		{
			name: "成功合并 PR（默认方式）",
			opts: prMergeOptions{
				input:  "123",
				method: "merge",
			},
			mockConfig: &config.Config{Host: "https://gitee.com/api/v5", Token: "test-token"},
			mockPR: &api.PullRequest{
				Number:    123,
				State:     "open",
				Mergeable: true,
				Title:     "Test PR",
				HTMLURL:   "https://gitee.com/owner/repo/pulls/123",
				Head:      api.Branch{Ref: "feature"},
			},
			wantErr: "",
		},
		{
			name: "成功合并 PR（squash 方式）",
			opts: prMergeOptions{
				input:  "456",
				method: "squash",
			},
			mockConfig: &config.Config{Host: "https://gitee.com/api/v5", Token: "test-token"},
			mockPR: &api.PullRequest{
				Number:    456,
				State:     "open",
				Mergeable: true,
				Title:     "Feature X",
				HTMLURL:   "https://gitee.com/owner/repo/pulls/456",
				Head:      api.Branch{Ref: "feature-x"},
			},
			wantErr: "",
		},
		{
			name: "成功合并 PR（rebase 方式 + 删除分支）",
			opts: prMergeOptions{
				input:        "789",
				method:       "rebase",
				deleteBranch: true,
			},
			mockConfig: &config.Config{Host: "https://gitee.com/api/v5", Token: "test-token"},
			mockPR: &api.PullRequest{
				Number:    789,
				State:     "open",
				Mergeable: true,
				Title:     "Refactor Y",
				HTMLURL:   "https://gitee.com/owner/repo/pulls/789",
				Head:      api.Branch{Ref: "refactor-y"},
			},
			wantErr: "",
		},
		{
			name: "未登录",
			opts: prMergeOptions{
				input:  "123",
				method: "merge",
			},
			mockConfig: &config.Config{Host: "https://gitee.com/api/v5", Token: ""},
			wantErr:    "未登录",
		},
		{
			name: "PR 状态不是 open",
			opts: prMergeOptions{
				input:  "123",
				method: "merge",
			},
			mockConfig: &config.Config{Host: "https://gitee.com/api/v5", Token: "test-token"},
			mockPR: &api.PullRequest{
				Number:    123,
				State:     "closed",
				Mergeable: false,
			},
			wantErr: "状态为 closed，无法合并",
		},
		{
			name: "PR 不可合并（有冲突）",
			opts: prMergeOptions{
				input:  "123",
				method: "merge",
			},
			mockConfig: &config.Config{Host: "https://gitee.com/api/v5", Token: "test-token"},
			mockPR: &api.PullRequest{
				Number:    123,
				State:     "open",
				Mergeable: false,
			},
			wantErr: "当前不可合并",
		},
		{
			name: "无效的合并方式",
			opts: prMergeOptions{
				input:  "123",
				method: "invalid",
			},
			mockConfig: &config.Config{Host: "https://gitee.com/api/v5", Token: "test-token"},
			mockPR: &api.PullRequest{
				Number:    123,
				State:     "open",
				Mergeable: true,
			},
			wantErr: "无效的合并方式",
		},
		{
			name: "获取 PR 失败",
			opts: prMergeOptions{
				input:  "999",
				method: "merge",
			},
			mockConfig: &config.Config{Host: "https://gitee.com/api/v5", Token: "test-token"},
			mockErr:    errors.New("PR not found"),
			wantErr:    "获取 PR 详情失败",
		},
		{
			name: "合并失败",
			opts: prMergeOptions{
				input:  "123",
				method: "merge",
			},
			mockConfig: &config.Config{Host: "https://gitee.com/api/v5", Token: "test-token"},
			mockPR: &api.PullRequest{
				Number:    123,
				State:     "open",
				Mergeable: true,
				Title:     "Test PR",
				Head:      api.Branch{Ref: "feature"},
			},
			mergeErr: errors.New("merge conflict"),
			wantErr:  "合并 PR 失败",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			env := prMergeEnv{
				git: &fakeGitRunner{
					remoteURL: "https://gitee.com/owner/repo.git",
				},
				loadConfig: func() (*config.Config, error) {
					return tt.mockConfig, nil
				},
				getPR: func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error) {
					if tt.mockErr != nil {
						return nil, tt.mockErr
					}
					return tt.mockPR, nil
				},
				mergePR: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.MergePullRequestInput) error {
					return tt.mergeErr
				},
				out: &out,
			}

			err := runPRMerge(context.Background(), tt.opts, env)

			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("期望错误包含 %q，但没有错误", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("期望错误包含 %q，实际错误: %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("不期望错误，但收到: %v", err)
				}
				// 验证成功输出
				if !strings.Contains(out.String(), "合并成功") {
					t.Errorf("期望输出包含 '合并成功'，实际输出: %s", out.String())
				}
			}
		})
	}
}

func TestParsePRInputForMerge(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{
			name:    "纯数字",
			input:   "123",
			want:    123,
			wantErr: false,
		},
		{
			name:    "URL 格式",
			input:   "https://gitee.com/owner/repo/pulls/456",
			want:    456,
			wantErr: false,
		},
		{
			name:    "无效输入",
			input:   "abc",
			wantErr: true,
		},
		{
			name:    "空输入",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePRInput(tt.input, "owner", "repo")
			if tt.wantErr {
				if err == nil {
					t.Errorf("期望错误，但没有错误")
				}
			} else {
				if err != nil {
					t.Errorf("不期望错误，但收到: %v", err)
				}
				if got != tt.want {
					t.Errorf("期望 %d，实际 %d", tt.want, got)
				}
			}
		})
	}
}
