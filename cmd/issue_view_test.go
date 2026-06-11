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

// newIssueViewGitFake 返回一个仓库 URL 已配置的 fakeGitRunner。
func newIssueViewGitFake() *fakeGitRunner {
	return &fakeGitRunner{
		remoteURL: "https://gitee.com/testowner/testrepo.git",
	}
}

func TestIssueViewCmd_Basic(t *testing.T) {
	issue := &api.Issue{
		Number:    "I456",
		Title:     "测试Issue标题",
		State:     "open",
		Body:      "这是Issue的正文内容。",
		User:      api.User{Login: "alice", Name: "Alice"},
		HTMLURL:   "https://gitee.com/testowner/testrepo/issues/I456",
		Comments:  5,
		CreatedAt: "2024-01-01T10:00:00+08:00",
		UpdatedAt: "2024-01-05T15:00:00+08:00",
	}

	var buf bytes.Buffer
	env := issueViewEnv{
		git: newIssueViewGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "fake-token", User: "testuser"}, nil
		},
		getIssue: func(ctx context.Context, host, token, owner, repo, number string) (*api.Issue, error) {
			return issue, nil
		},
		listComments: func(ctx context.Context, host, token, owner, repo, number string) ([]api.Comment, error) {
			return nil, nil
		},
		openBrowser: func(url string) error {
			return nil
		},
		out:   &buf,
		isTTY: func() bool { return false },
		now:   func() time.Time { return time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC) },
	}

	opts := issueViewOptions{}
	err := runIssueView(context.Background(), "I456", opts, env)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}

	output := buf.String()
	wantStrings := []string{"I456", "测试Issue标题", "OPEN", "alice", "这是Issue的正文内容", "评论数:", "5"}
	for _, want := range wantStrings {
		if !strings.Contains(output, want) {
			t.Errorf("输出缺少 %q\n完整输出:\n%s", want, output)
		}
	}
}

func TestIssueViewCmd_WithComments(t *testing.T) {
	issue := &api.Issue{
		Number:    "I789",
		Title:     "有评论的Issue",
		State:     "open",
		Body:      "Issue正文",
		User:      api.User{Login: "bob"},
		HTMLURL:   "https://gitee.com/testowner/testrepo/issues/I789",
		Comments:  2,
		CreatedAt: "2024-01-01T10:00:00+08:00",
		UpdatedAt: "2024-01-02T10:00:00+08:00",
	}

	comments := []api.Comment{
		{
			ID:        1,
			Body:      "第一条评论",
			User:      api.User{Login: "charlie"},
			CreatedAt: "2024-01-02T11:00:00+08:00",
		},
		{
			ID:        2,
			Body:      "第二条评论",
			User:      api.User{Login: "dave"},
			CreatedAt: "2024-01-02T12:00:00+08:00",
		},
	}

	var buf bytes.Buffer
	env := issueViewEnv{
		git: newIssueViewGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "fake-token", User: "testuser"}, nil
		},
		getIssue: func(ctx context.Context, host, token, owner, repo, number string) (*api.Issue, error) {
			return issue, nil
		},
		listComments: func(ctx context.Context, host, token, owner, repo, number string) ([]api.Comment, error) {
			return comments, nil
		},
		openBrowser: func(url string) error {
			return nil
		},
		out:   &buf,
		isTTY: func() bool { return false },
		now:   func() time.Time { return time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC) },
	}

	opts := issueViewOptions{comments: true}
	err := runIssueView(context.Background(), "I789", opts, env)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}

	output := buf.String()
	wantStrings := []string{"评论 (2)", "charlie", "第一条评论", "dave", "第二条评论"}
	for _, want := range wantStrings {
		if !strings.Contains(output, want) {
			t.Errorf("输出缺少 %q\n完整输出:\n%s", want, output)
		}
	}
}

func TestIssueViewCmd_WebMode(t *testing.T) {
	issue := &api.Issue{
		Number:    "I999",
		Title:     "Web模式测试",
		State:     "open",
		Body:      "正文",
		User:      api.User{Login: "eve"},
		HTMLURL:   "https://gitee.com/testowner/testrepo/issues/I999",
		CreatedAt: "2024-01-01T10:00:00+08:00",
		UpdatedAt: "2024-01-01T10:00:00+08:00",
	}

	var buf bytes.Buffer
	browserOpened := false
	var browserURL string

	env := issueViewEnv{
		git: newIssueViewGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "fake-token", User: "testuser"}, nil
		},
		getIssue: func(ctx context.Context, host, token, owner, repo, number string) (*api.Issue, error) {
			return issue, nil
		},
		listComments: func(ctx context.Context, host, token, owner, repo, number string) ([]api.Comment, error) {
			return nil, nil
		},
		openBrowser: func(url string) error {
			browserOpened = true
			browserURL = url
			return nil
		},
		out:   &buf,
		isTTY: func() bool { return false },
		now:   func() time.Time { return time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC) },
	}

	opts := issueViewOptions{web: true}
	err := runIssueView(context.Background(), "I999", opts, env)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}

	if !browserOpened {
		t.Errorf("浏览器应该被打开")
	}
	if browserURL != issue.HTMLURL {
		t.Errorf("浏览器打开的 URL 不正确，期望 %q，实际 %q", issue.HTMLURL, browserURL)
	}

	output := buf.String()
	if !strings.Contains(output, "正在浏览器中打开") {
		t.Errorf("输出应包含浏览器打开提示")
	}
}

func TestIssueViewCmd_JSONOutput(t *testing.T) {
	issue := &api.Issue{
		Number:    "I111",
		Title:     "JSON测试",
		State:     "closed",
		Body:      "正文内容",
		User:      api.User{Login: "frank"},
		HTMLURL:   "https://gitee.com/testowner/testrepo/issues/I111",
		CreatedAt: "2024-01-01T10:00:00+08:00",
		UpdatedAt: "2024-01-01T10:00:00+08:00",
		ClosedAt:  "2024-01-05T10:00:00+08:00",
	}

	var buf bytes.Buffer
	env := issueViewEnv{
		git: newIssueViewGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "fake-token", User: "testuser"}, nil
		},
		getIssue: func(ctx context.Context, host, token, owner, repo, number string) (*api.Issue, error) {
			return issue, nil
		},
		listComments: func(ctx context.Context, host, token, owner, repo, number string) ([]api.Comment, error) {
			return nil, nil
		},
		openBrowser: func(url string) error {
			return nil
		},
		out:   &buf,
		isTTY: func() bool { return false },
		now:   func() time.Time { return time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC) },
	}

	opts := issueViewOptions{jsonOut: true}
	err := runIssueView(context.Background(), "I111", opts, env)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}

	output := buf.String()
	wantStrings := []string{`"number": "I111"`, `"title": "JSON测试"`, `"state": "closed"`}
	for _, want := range wantStrings {
		if !strings.Contains(output, want) {
			t.Errorf("JSON 输出缺少 %q\n完整输出:\n%s", want, output)
		}
	}
}

func TestIssueViewCmd_EmptyNumber(t *testing.T) {
	env := issueViewEnv{
		git: newIssueViewGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "fake-token", User: "testuser"}, nil
		},
		getIssue: func(ctx context.Context, host, token, owner, repo, number string) (*api.Issue, error) {
			return nil, nil
		},
		listComments: func(ctx context.Context, host, token, owner, repo, number string) ([]api.Comment, error) {
			return nil, nil
		},
		openBrowser: func(url string) error {
			return nil
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return time.Now() },
	}

	opts := issueViewOptions{}
	err := runIssueView(context.Background(), "", opts, env)
	if err == nil {
		t.Fatalf("期望错误但没有返回")
	}
	if !strings.Contains(err.Error(), "Issue 编号不能为空") {
		t.Fatalf("期望错误消息包含 'Issue 编号不能为空'，实际: %v", err)
	}
}

func TestIssueViewCmd_APIError(t *testing.T) {
	env := issueViewEnv{
		git: newIssueViewGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "fake-token", User: "testuser"}, nil
		},
		getIssue: func(ctx context.Context, host, token, owner, repo, number string) (*api.Issue, error) {
			return nil, errors.New("API错误")
		},
		listComments: func(ctx context.Context, host, token, owner, repo, number string) ([]api.Comment, error) {
			return nil, nil
		},
		openBrowser: func(url string) error {
			return nil
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
		now:   func() time.Time { return time.Now() },
	}

	opts := issueViewOptions{}
	err := runIssueView(context.Background(), "I404", opts, env)
	if err == nil {
		t.Fatalf("期望错误但没有返回")
	}
	if !strings.Contains(err.Error(), "查询 Issue 详情失败") {
		t.Fatalf("期望错误消息包含 '查询 Issue 详情失败'，实际: %v", err)
	}
}

func TestIssueViewCmd_WithLabelsAndMilestone(t *testing.T) {
	issue := &api.Issue{
		Number: "I222",
		Title:  "带标签和里程碑的Issue",
		State:  "open",
		Body:   "正文",
		User:   api.User{Login: "grace"},
		Labels: []struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Color string `json:"color"`
		}{
			{ID: 1, Name: "bug", Color: "red"},
			{ID: 2, Name: "urgent", Color: "yellow"},
		},
		Assignee: &api.User{Login: "helen"},
		Milestone: &struct {
			ID     int64  `json:"id"`
			Title  string `json:"title"`
			Number int64  `json:"number"`
		}{
			ID:     10,
			Title:  "v1.0",
			Number: 1,
		},
		HTMLURL:   "https://gitee.com/testowner/testrepo/issues/I222",
		CreatedAt: "2024-01-01T10:00:00+08:00",
		UpdatedAt: "2024-01-01T10:00:00+08:00",
	}

	var buf bytes.Buffer
	env := issueViewEnv{
		git: newIssueViewGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "fake-token", User: "testuser"}, nil
		},
		getIssue: func(ctx context.Context, host, token, owner, repo, number string) (*api.Issue, error) {
			return issue, nil
		},
		listComments: func(ctx context.Context, host, token, owner, repo, number string) ([]api.Comment, error) {
			return nil, nil
		},
		openBrowser: func(url string) error {
			return nil
		},
		out:   &buf,
		isTTY: func() bool { return false },
		now:   func() time.Time { return time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC) },
	}

	opts := issueViewOptions{}
	err := runIssueView(context.Background(), "I222", opts, env)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}

	output := buf.String()
	wantStrings := []string{"标签: bug,urgent", "指派给: helen", "里程碑: v1.0"}
	for _, want := range wantStrings {
		if !strings.Contains(output, want) {
			t.Errorf("输出缺少 %q\n完整输出:\n%s", want, output)
		}
	}
}
