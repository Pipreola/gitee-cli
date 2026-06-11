package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gitee-cli/pkg/api"
	"gitee-cli/pkg/config"
)

// checkoutGitRunner 是一个有状态的 fake git runner，模拟真实 git 的关键语义，
// 用于覆盖 pr checkout 的分支检出/更新流程。
//
// 它显式实现了真实 git 的一个核心约束：
// 不能将 fetch 的目标 refspec 指向当前已检出的分支
// （git 会返回 "refusing to fetch into branch ... checked out"）。
// 这正是上一版实现遗漏、技术专家要求补齐的真实语义。
type checkoutGitRunner struct {
	remoteURL     string
	currentBranch string          // 当前检出分支
	branches      map[string]bool // 已存在的本地分支
	statusOutput  string          // status --porcelain 输出
	statusErr     error
	fetchHeadSet  bool // 是否已成功 fetch 到 FETCH_HEAD
	mergeFFErr    error

	calls []string // 记录所有 git 调用，便于断言
}

func (g *checkoutGitRunner) run(args ...string) (string, error) {
	g.calls = append(g.calls, strings.Join(args, " "))

	switch {
	case len(args) >= 3 && args[0] == "remote" && args[1] == "get-url" && args[2] == "origin":
		if g.remoteURL == "" {
			return "", errors.New("not a git repository")
		}
		return g.remoteURL, nil

	case len(args) >= 2 && args[0] == "status" && args[1] == "--porcelain":
		return g.statusOutput, g.statusErr

	case len(args) >= 3 && args[0] == "rev-parse" && args[1] == "--verify":
		branch := args[2]
		if g.branches[branch] {
			return "abc123", nil
		}
		return "", errors.New("fatal: Needed a single revision")

	case args[0] == "fetch":
		// 形式一: git fetch origin pull/N/head:pr-N （带本地目标引用）
		// 真实 git 在目标引用 == 当前检出分支时会拒绝。
		for _, a := range args {
			if strings.Contains(a, ":") && strings.HasPrefix(a, "pull/") {
				dst := a[strings.Index(a, ":")+1:]
				if dst == g.currentBranch {
					return "", fmt.Errorf(
						"fatal: refusing to fetch into branch 'refs/heads/%s' checked out", dst)
				}
				// 写入目标分支引用
				if g.branches == nil {
					g.branches = map[string]bool{}
				}
				g.branches[dst] = true
				return "", nil
			}
		}
		// 形式二: git fetch origin pull/N/head （无目标引用，写入 FETCH_HEAD）
		// 这是真实 git 始终允许的安全形式。
		g.fetchHeadSet = true
		return "", nil

	case args[0] == "checkout":
		// git checkout -b <branch> FETCH_HEAD
		if len(args) >= 3 && args[1] == "-b" {
			if !g.fetchHeadSet && args[len(args)-1] == "FETCH_HEAD" {
				return "", errors.New("fatal: FETCH_HEAD 不存在")
			}
			branch := args[2]
			if g.branches == nil {
				g.branches = map[string]bool{}
			}
			g.branches[branch] = true
			g.currentBranch = branch
			return "", nil
		}
		// git checkout <branch>
		if len(args) >= 2 {
			branch := args[1]
			if !g.branches[branch] {
				return "", fmt.Errorf("error: pathspec '%s' did not match", branch)
			}
			g.currentBranch = branch
			return "", nil
		}

	case len(args) >= 3 && args[0] == "reset" && args[1] == "--hard":
		if args[2] == "FETCH_HEAD" && !g.fetchHeadSet {
			return "", errors.New("fatal: FETCH_HEAD 不存在")
		}
		return "", nil

	case len(args) >= 2 && args[0] == "merge" && args[1] == "--ff-only":
		if g.mergeFFErr != nil {
			return "", g.mergeFFErr
		}
		if !g.fetchHeadSet {
			return "", errors.New("fatal: FETCH_HEAD 不存在")
		}
		return "", nil
	}

	return "", fmt.Errorf("未桩化的 git 调用: %v", args)
}

func (g *checkoutGitRunner) runInteractive(args ...string) error {
	return fmt.Errorf("未桩化的交互 git 调用: %v", args)
}

// hasCall 判断是否记录过某个 git 调用（精确匹配）。
func (g *checkoutGitRunner) hasCall(call string) bool {
	for _, c := range g.calls {
		if c == call {
			return true
		}
	}
	return false
}

// hasCallPrefix 判断是否存在以指定前缀开头的 git 调用。
func (g *checkoutGitRunner) hasCallPrefix(prefix string) bool {
	for _, c := range g.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// newCheckoutEnv 构造一个测试用 prCheckoutEnv。
func newCheckoutEnv(git gitRunner, out *bytes.Buffer, pr *api.PullRequest, getErr error) prCheckoutEnv {
	return prCheckoutEnv{
		git: git,
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Host: "https://gitee.com/api/v5", Token: "tok"}, nil
		},
		getPR: func(ctx context.Context, host, token, owner, repo string, number int64) (*api.PullRequest, error) {
			if getErr != nil {
				return nil, getErr
			}
			return pr, nil
		},
		out: out,
	}
}

func samplePR(number int64, state, mergedAt string) *api.PullRequest {
	return &api.PullRequest{
		Number:   number,
		State:    state,
		Title:    "示例 PR",
		HTMLURL:  fmt.Sprintf("https://gitee.com/owner/repo/pulls/%d", number),
		MergedAt: mergedAt,
		Head:     api.Branch{Label: "contributor:feature", Ref: "feature"},
		Base:     api.Branch{Ref: "main"},
	}
}

// TestRunPRCheckoutNoAuth 验证未登录时报错。
func TestRunPRCheckoutNoAuth(t *testing.T) {
	env := prCheckoutEnv{
		git:        &checkoutGitRunner{remoteURL: "https://gitee.com/owner/repo.git"},
		loadConfig: func() (*config.Config, error) { return &config.Config{Token: ""}, nil },
		out:        &bytes.Buffer{},
	}
	err := runPRCheckout(context.Background(), prCheckoutOptions{input: "123"}, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Fatalf("期望未登录错误，实际: %v", err)
	}
}

// TestRunPRCheckoutRepoError 验证无法解析仓库信息时报错。
func TestRunPRCheckoutRepoError(t *testing.T) {
	out := &bytes.Buffer{}
	git := &checkoutGitRunner{remoteURL: ""}
	env := newCheckoutEnv(git, out, samplePR(1, "open", ""), nil)
	err := runPRCheckout(context.Background(), prCheckoutOptions{input: "1"}, env)
	if err == nil || !strings.Contains(err.Error(), "获取仓库信息失败") {
		t.Fatalf("期望仓库信息错误，实际: %v", err)
	}
}

// TestRunPRCheckoutGetPRError 验证 API 获取 PR 失败时报错。
func TestRunPRCheckoutGetPRError(t *testing.T) {
	out := &bytes.Buffer{}
	git := &checkoutGitRunner{remoteURL: "https://gitee.com/owner/repo.git"}
	env := newCheckoutEnv(git, out, nil, errors.New("PR 不存在"))
	err := runPRCheckout(context.Background(), prCheckoutOptions{input: "999"}, env)
	if err == nil || !strings.Contains(err.Error(), "获取 PR 详情失败") {
		t.Fatalf("期望 PR 详情错误，实际: %v", err)
	}
}

// TestRunPRCheckoutDirtyWorktree 验证有未提交更改且非 force 时阻止检出。
func TestRunPRCheckoutDirtyWorktree(t *testing.T) {
	out := &bytes.Buffer{}
	git := &checkoutGitRunner{
		remoteURL:    "https://gitee.com/owner/repo.git",
		statusOutput: " M cmd/pr.go",
	}
	env := newCheckoutEnv(git, out, samplePR(7, "open", ""), nil)
	err := runPRCheckout(context.Background(), prCheckoutOptions{input: "7", force: false}, env)
	if err == nil || !strings.Contains(err.Error(), "未提交的更改") {
		t.Fatalf("期望未提交更改错误，实际: %v", err)
	}
}

// TestRunPRCheckoutNewBranch 验证检出新分支时：
// fetch 到 FETCH_HEAD，再基于 FETCH_HEAD 创建分支。
func TestRunPRCheckoutNewBranch(t *testing.T) {
	out := &bytes.Buffer{}
	git := &checkoutGitRunner{
		remoteURL:     "https://gitee.com/owner/repo.git",
		currentBranch: "main",
		branches:      map[string]bool{"main": true},
	}
	env := newCheckoutEnv(git, out, samplePR(42, "open", ""), nil)
	if err := runPRCheckout(context.Background(), prCheckoutOptions{input: "42"}, env); err != nil {
		t.Fatalf("期望成功，实际错误: %v", err)
	}
	// 必须 fetch 到 FETCH_HEAD（无本地目标引用形式）
	if !git.hasCall("fetch origin pull/42/head") {
		t.Errorf("期望 fetch 到 FETCH_HEAD，实际调用: %v", git.calls)
	}
	// 不应出现带目标引用的危险 fetch 形式
	if git.hasCallPrefix("fetch origin pull/42/head:") {
		t.Errorf("不应 fetch 到本地分支引用，实际调用: %v", git.calls)
	}
	// 基于 FETCH_HEAD 创建分支
	if !git.hasCall("checkout -b pr-42 FETCH_HEAD") {
		t.Errorf("期望基于 FETCH_HEAD 创建分支，实际调用: %v", git.calls)
	}
	if git.currentBranch != "pr-42" {
		t.Errorf("期望切换到 pr-42，实际: %s", git.currentBranch)
	}
	if !strings.Contains(out.String(), "检出成功") {
		t.Errorf("期望成功提示，实际输出: %s", out.String())
	}
}

// TestRunPRCheckoutExistingBranchOnSameBranch 是本次修复的核心回归测试：
// 当本地已存在 pr-N 分支、且当前正检出在 pr-N 上时，
// 旧实现会用 "fetch origin pull/N/head:pr-N" 直接 fetch 到当前分支，
// 被真实 git 拒绝。新实现 fetch 到 FETCH_HEAD 后再快进更新，应当成功。
func TestRunPRCheckoutExistingBranchOnSameBranch(t *testing.T) {
	out := &bytes.Buffer{}
	git := &checkoutGitRunner{
		remoteURL:     "https://gitee.com/owner/repo.git",
		currentBranch: "pr-100", // 已经检出在目标分支上
		branches:      map[string]bool{"main": true, "pr-100": true},
	}
	env := newCheckoutEnv(git, out, samplePR(100, "open", ""), nil)
	if err := runPRCheckout(context.Background(), prCheckoutOptions{input: "100"}, env); err != nil {
		t.Fatalf("期望成功更新已存在分支，实际错误: %v", err)
	}
	// 关键：使用 FETCH_HEAD 形式，绝不能 fetch 到 pr-100 引用
	if !git.hasCall("fetch origin pull/100/head") {
		t.Errorf("期望 fetch 到 FETCH_HEAD，实际调用: %v", git.calls)
	}
	if git.hasCallPrefix("fetch origin pull/100/head:") {
		t.Errorf("不应 fetch 到 pr-100 引用（真实 git 会拒绝），实际调用: %v", git.calls)
	}
	// 非 force：应快进更新
	if !git.hasCall("merge --ff-only FETCH_HEAD") {
		t.Errorf("期望快进更新，实际调用: %v", git.calls)
	}
}

// TestRunPRCheckoutExistingBranchForce 验证 force 模式下硬重置到 PR 最新提交。
func TestRunPRCheckoutExistingBranchForce(t *testing.T) {
	out := &bytes.Buffer{}
	git := &checkoutGitRunner{
		remoteURL:     "https://gitee.com/owner/repo.git",
		currentBranch: "main",
		branches:      map[string]bool{"main": true, "pr-5": true},
	}
	env := newCheckoutEnv(git, out, samplePR(5, "open", ""), nil)
	if err := runPRCheckout(context.Background(), prCheckoutOptions{input: "5", force: true}, env); err != nil {
		t.Fatalf("期望 force 成功，实际错误: %v", err)
	}
	if !git.hasCall("checkout pr-5") {
		t.Errorf("期望切换到 pr-5，实际调用: %v", git.calls)
	}
	if !git.hasCall("reset --hard FETCH_HEAD") {
		t.Errorf("期望 force 模式硬重置，实际调用: %v", git.calls)
	}
}

// TestRunPRCheckoutExistingBranchNonFastForward 验证非快进更新失败时给出明确指引。
func TestRunPRCheckoutExistingBranchNonFastForward(t *testing.T) {
	out := &bytes.Buffer{}
	git := &checkoutGitRunner{
		remoteURL:     "https://gitee.com/owner/repo.git",
		currentBranch: "main",
		branches:      map[string]bool{"main": true, "pr-9": true},
		mergeFFErr:    errors.New("fatal: Not possible to fast-forward"),
	}
	env := newCheckoutEnv(git, out, samplePR(9, "open", ""), nil)
	err := runPRCheckout(context.Background(), prCheckoutOptions{input: "9"}, env)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("期望提示使用 --force，实际: %v", err)
	}
}

// TestRunPRCheckoutClosedPR 验证已关闭 PR 给出警告但仍可检出。
func TestRunPRCheckoutClosedPR(t *testing.T) {
	out := &bytes.Buffer{}
	git := &checkoutGitRunner{
		remoteURL:     "https://gitee.com/owner/repo.git",
		currentBranch: "main",
		branches:      map[string]bool{"main": true},
	}
	env := newCheckoutEnv(git, out, samplePR(3, "closed", ""), nil)
	if err := runPRCheckout(context.Background(), prCheckoutOptions{input: "3"}, env); err != nil {
		t.Fatalf("期望成功，实际错误: %v", err)
	}
	if !strings.Contains(out.String(), "已关闭") {
		t.Errorf("期望已关闭警告，实际输出: %s", out.String())
	}
}

// TestRunPRCheckoutMergedPR 验证已合并 PR 给出合并警告。
func TestRunPRCheckoutMergedPR(t *testing.T) {
	out := &bytes.Buffer{}
	git := &checkoutGitRunner{
		remoteURL:     "https://gitee.com/owner/repo.git",
		currentBranch: "main",
		branches:      map[string]bool{"main": true},
	}
	env := newCheckoutEnv(git, out, samplePR(4, "closed", "2026-06-01T00:00:00Z"), nil)
	if err := runPRCheckout(context.Background(), prCheckoutOptions{input: "4"}, env); err != nil {
		t.Fatalf("期望成功，实际错误: %v", err)
	}
	if !strings.Contains(out.String(), "已合并") {
		t.Errorf("期望已合并警告，实际输出: %s", out.String())
	}
}

// TestParsePRInput 验证输入解析：编号、URL、分支名（不支持）、非法输入。
func TestParsePRInput(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"纯数字", "123", 123, false},
		{"带空格数字", "  45 ", 45, false},
		{"https URL", "https://gitee.com/owner/repo/pulls/678", 678, false},
		{"http URL", "http://gitee.com/owner/repo/pulls/9", 9, false},
		{"空输入", "", 0, true},
		{"分支名不支持", "feature-branch", 0, true},
		{"非法 URL", "https://gitee.com/owner/repo/issues/1", 0, true},
		{"零编号", "0", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parsePRInput(c.input, "owner", "repo")
			if c.wantErr {
				if err == nil {
					t.Errorf("期望错误，实际成功 got=%d", got)
				}
				return
			}
			if err != nil {
				t.Errorf("期望成功，实际错误: %v", err)
			}
			if got != c.want {
				t.Errorf("期望 %d，实际 %d", c.want, got)
			}
		})
	}
}
