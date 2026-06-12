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

// newReviewTestEnv 构造一个默认成功路径的 prReviewEnv，调用方按需覆盖字段。
func newReviewTestEnv(out *bytes.Buffer) prReviewEnv {
	return prReviewEnv{
		git: &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		reviewPR: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.ReviewPullRequestInput) error {
			return nil
		},
		createPRComment: func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error) {
			return &api.Comment{
				ID:        789,
				Body:      input.Body,
				HTMLURL:   "https://gitee.com/owner/repo/pulls/123#note_789",
				User:      api.User{Login: "reviewer"},
				CreatedAt: "2024-01-01T00:00:00+08:00",
			}, nil
		},
		out: out,
	}
}

// TestRunPRReviewRequiresAction 验证既未指定 --approve 也未指定 --comment 时报错。
func TestRunPRReviewRequiresAction(t *testing.T) {
	out := &bytes.Buffer{}
	err := runPRReview(context.Background(), 123, prReviewOptions{}, newReviewTestEnv(out))
	if err == nil || !strings.Contains(err.Error(), "必须指定审查动作") {
		t.Errorf("期望必须指定动作错误，实际: %v", err)
	}
}

// TestRunPRReviewMutualExclusive 验证 --approve 与 --comment 互斥。
func TestRunPRReviewMutualExclusive(t *testing.T) {
	out := &bytes.Buffer{}
	opts := prReviewOptions{approve: true, comment: true, body: "x"}
	err := runPRReview(context.Background(), 123, opts, newReviewTestEnv(out))
	if err == nil || !strings.Contains(err.Error(), "不能同时指定") {
		t.Errorf("期望互斥错误，实际: %v", err)
	}
}

// TestRunPRReviewApproveOnly 验证仅审查通过（不附评论）的成功路径。
func TestRunPRReviewApproveOnly(t *testing.T) {
	out := &bytes.Buffer{}
	env := newReviewTestEnv(out)

	var reviewCalled, commentCalled bool
	var capturedNumber int64
	var capturedForce bool
	env.reviewPR = func(ctx context.Context, host, token, owner, repo string, number int64, input *api.ReviewPullRequestInput) error {
		reviewCalled = true
		capturedNumber = number
		capturedForce = input.Force
		return nil
	}
	env.createPRComment = func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error) {
		commentCalled = true
		return &api.Comment{ID: 1}, nil
	}

	if err := runPRReview(context.Background(), 123, prReviewOptions{approve: true}, env); err != nil {
		t.Fatalf("期望成功，实际错误: %v", err)
	}
	if !reviewCalled {
		t.Error("期望调用审查接口")
	}
	if commentCalled {
		t.Error("未提供 --body 时不应调用评论接口")
	}
	if capturedNumber != 123 {
		t.Errorf("PR 编号 = %d, 期望 123", capturedNumber)
	}
	if capturedForce {
		t.Error("未指定 --force 时不应强制通过")
	}
	if !strings.Contains(out.String(), "已审查通过") {
		t.Errorf("期望输出包含审查通过信息，实际: %s", out.String())
	}
}

// TestRunPRReviewApproveWithForce 验证 --force 透传到审查接口并提示。
func TestRunPRReviewApproveWithForce(t *testing.T) {
	out := &bytes.Buffer{}
	env := newReviewTestEnv(out)

	var capturedForce bool
	env.reviewPR = func(ctx context.Context, host, token, owner, repo string, number int64, input *api.ReviewPullRequestInput) error {
		capturedForce = input.Force
		return nil
	}

	if err := runPRReview(context.Background(), 5, prReviewOptions{approve: true, force: true}, env); err != nil {
		t.Fatalf("期望成功，实际错误: %v", err)
	}
	if !capturedForce {
		t.Error("期望 force 透传为 true")
	}
	if !strings.Contains(out.String(), "强制通过") {
		t.Errorf("期望输出包含强制通过提示，实际: %s", out.String())
	}
}

// TestRunPRReviewApproveWithBody 验证审查通过后附带评论。
func TestRunPRReviewApproveWithBody(t *testing.T) {
	out := &bytes.Buffer{}
	env := newReviewTestEnv(out)

	var reviewCalled bool
	var capturedBody string
	env.reviewPR = func(ctx context.Context, host, token, owner, repo string, number int64, input *api.ReviewPullRequestInput) error {
		reviewCalled = true
		return nil
	}
	env.createPRComment = func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error) {
		capturedBody = input.Body
		return &api.Comment{ID: 789, Body: input.Body, HTMLURL: "https://gitee.com/owner/repo/pulls/123#note_789", User: api.User{Login: "reviewer"}}, nil
	}

	opts := prReviewOptions{approve: true, body: "LGTM，逻辑清晰"}
	if err := runPRReview(context.Background(), 123, opts, env); err != nil {
		t.Fatalf("期望成功，实际错误: %v", err)
	}
	if !reviewCalled {
		t.Error("期望调用审查接口")
	}
	if capturedBody != "LGTM，逻辑清晰" {
		t.Errorf("评论内容 = %q, 期望 'LGTM，逻辑清晰'", capturedBody)
	}
	if !strings.Contains(out.String(), "已审查通过") || !strings.Contains(out.String(), "评论添加成功") {
		t.Errorf("期望输出同时包含审查通过与评论成功信息，实际: %s", out.String())
	}
}

// TestRunPRReviewCommentOnly 验证仅评论模式的成功路径，不应调用审查接口。
func TestRunPRReviewCommentOnly(t *testing.T) {
	out := &bytes.Buffer{}
	env := newReviewTestEnv(out)

	var reviewCalled bool
	var capturedBody string
	env.reviewPR = func(ctx context.Context, host, token, owner, repo string, number int64, input *api.ReviewPullRequestInput) error {
		reviewCalled = true
		return nil
	}
	env.createPRComment = func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error) {
		capturedBody = input.Body
		return &api.Comment{ID: 789, Body: input.Body, HTMLURL: "https://gitee.com/owner/repo/pulls/123#note_789", User: api.User{Login: "reviewer"}}, nil
	}

	opts := prReviewOptions{comment: true, body: "第 10 行建议补充边界校验"}
	if err := runPRReview(context.Background(), 123, opts, env); err != nil {
		t.Fatalf("期望成功，实际错误: %v", err)
	}
	if reviewCalled {
		t.Error("仅评论模式不应调用审查接口")
	}
	if capturedBody != "第 10 行建议补充边界校验" {
		t.Errorf("评论内容 = %q, 不符合预期", capturedBody)
	}
	if !strings.Contains(out.String(), "评论添加成功") {
		t.Errorf("期望输出包含评论成功信息，实际: %s", out.String())
	}
}

// TestRunPRReviewCommentRequiresBody 验证 --comment 缺少内容时报错。
func TestRunPRReviewCommentRequiresBody(t *testing.T) {
	out := &bytes.Buffer{}
	err := runPRReview(context.Background(), 123, prReviewOptions{comment: true}, newReviewTestEnv(out))
	if err == nil || !strings.Contains(err.Error(), "必须通过 --body") {
		t.Errorf("期望缺少内容错误，实际: %v", err)
	}
}

// TestRunPRReviewCommentRejectsForce 验证 --force 不能与 --comment 同时使用。
func TestRunPRReviewCommentRejectsForce(t *testing.T) {
	out := &bytes.Buffer{}
	opts := prReviewOptions{comment: true, body: "x", force: true}
	err := runPRReview(context.Background(), 123, opts, newReviewTestEnv(out))
	if err == nil || !strings.Contains(err.Error(), "--force 仅对 --approve 生效") {
		t.Errorf("期望 force 与 comment 冲突错误，实际: %v", err)
	}
}

// TestRunPRReviewBodyAndBodyFileExclusive 验证 --body 与 --body-file 互斥。
func TestRunPRReviewBodyAndBodyFileExclusive(t *testing.T) {
	out := &bytes.Buffer{}
	opts := prReviewOptions{approve: true, body: "a", bodyFile: "b.txt"}
	err := runPRReview(context.Background(), 123, opts, newReviewTestEnv(out))
	if err == nil || !strings.Contains(err.Error(), "不能同时指定") {
		t.Errorf("期望 body 互斥错误，实际: %v", err)
	}
}

// TestRunPRReviewBodyFile 验证从文件读取评论内容。
func TestRunPRReviewBodyFile(t *testing.T) {
	out := &bytes.Buffer{}
	env := newReviewTestEnv(out)

	var capturedBody string
	env.readFile = func(filename string) ([]byte, error) {
		if filename != "review.txt" {
			t.Errorf("文件名 = %q, 期望 review.txt", filename)
		}
		return []byte("  来自文件的评审意见  \n"), nil
	}
	env.createPRComment = func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error) {
		capturedBody = input.Body
		return &api.Comment{ID: 1, Body: input.Body, User: api.User{Login: "reviewer"}}, nil
	}

	opts := prReviewOptions{comment: true, bodyFile: "review.txt"}
	if err := runPRReview(context.Background(), 123, opts, env); err != nil {
		t.Fatalf("期望成功，实际错误: %v", err)
	}
	if capturedBody != "来自文件的评审意见" {
		t.Errorf("评论内容 = %q, 期望已 trim 的 '来自文件的评审意见'", capturedBody)
	}
}

// TestRunPRReviewNoAuth 验证未登录时报错。
func TestRunPRReviewNoAuth(t *testing.T) {
	out := &bytes.Buffer{}
	env := newReviewTestEnv(out)
	env.loadConfig = func() (*config.Config, error) {
		return &config.Config{Host: "https://gitee.com/api/v5", Token: ""}, nil
	}
	err := runPRReview(context.Background(), 123, prReviewOptions{approve: true}, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

// TestRunPRReviewGetRepoError 验证获取仓库信息失败时的错误处理。
func TestRunPRReviewGetRepoError(t *testing.T) {
	out := &bytes.Buffer{}
	env := newReviewTestEnv(out)
	env.git = &fakeGitRunner{remoteURL: ""} // 返回错误
	err := runPRReview(context.Background(), 123, prReviewOptions{approve: true}, env)
	if err == nil || !strings.Contains(err.Error(), "获取仓库信息失败") {
		t.Errorf("期望仓库信息错误，实际: %v", err)
	}
}

// TestRunPRReviewAPIError 验证审查接口返回错误时被向上包装。
func TestRunPRReviewAPIError(t *testing.T) {
	out := &bytes.Buffer{}
	env := newReviewTestEnv(out)
	env.reviewPR = func(ctx context.Context, host, token, owner, repo string, number int64, input *api.ReviewPullRequestInput) error {
		return errors.New("boom")
	}
	err := runPRReview(context.Background(), 123, prReviewOptions{approve: true}, env)
	if err == nil || !strings.Contains(err.Error(), "审查通过失败") {
		t.Errorf("期望审查失败错误，实际: %v", err)
	}
}

// TestRunPRReviewApproveSucceedsButCommentFails 验证审查已通过但附加评论失败时
// 返回明确的错误信息，避免误导用户以为评论也成功。
func TestRunPRReviewApproveSucceedsButCommentFails(t *testing.T) {
	out := &bytes.Buffer{}
	env := newReviewTestEnv(out)
	env.createPRComment = func(ctx context.Context, host, token, owner, repo string, number int64, input *api.CreatePullRequestCommentInput) (*api.Comment, error) {
		return nil, errors.New("network down")
	}
	err := runPRReview(context.Background(), 123, prReviewOptions{approve: true, body: "LGTM"}, env)
	if err == nil || !strings.Contains(err.Error(), "审查已通过，但附加评论失败") {
		t.Errorf("期望附加评论失败错误，实际: %v", err)
	}
}
