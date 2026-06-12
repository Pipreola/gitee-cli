package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"gitee-cli/pkg/config"
)

const sampleDiff = `diff --git a/main.go b/main.go
index abc1234..def5678 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main

 import "fmt"
+import "os"

 func main() {
-    fmt.Println("hello")
+    fmt.Fprintln(os.Stdout, "hello")
 }
diff --git a/util.go b/util.go
new file mode 100644
--- /dev/null
+++ b/util.go
@@ -0,0 +1,3 @@
+package main
+
+func helper() {}
`

func newDiffGitFake() *fakeGitRunner {
	return &fakeGitRunner{remoteURL: "https://gitee.com/owner/repo.git"}
}

func TestRunPRDiffNoAuth(t *testing.T) {
	env := prDiffEnv{
		git: newDiffGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: ""}, nil
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
	}
	err := runPRDiff(context.Background(), prDiffOptions{number: 1}, env)
	if err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Errorf("期望未登录错误，实际: %v", err)
	}
}

func TestRunPRDiffConfigError(t *testing.T) {
	env := prDiffEnv{
		git: newDiffGitFake(),
		loadConfig: func() (*config.Config, error) {
			return nil, errors.New("config broken")
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
	}
	err := runPRDiff(context.Background(), prDiffOptions{number: 1}, env)
	if err == nil || !strings.Contains(err.Error(), "加载配置失败") {
		t.Errorf("期望配置错误，实际: %v", err)
	}
}

func TestRunPRDiffRepoError(t *testing.T) {
	env := prDiffEnv{
		git: &fakeGitRunner{remoteURL: ""},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok"}, nil
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
	}
	err := runPRDiff(context.Background(), prDiffOptions{number: 1}, env)
	if err == nil || !strings.Contains(err.Error(), "获取仓库信息失败") {
		t.Errorf("期望仓库错误，实际: %v", err)
	}
}

func TestRunPRDiffAPIError(t *testing.T) {
	env := prDiffEnv{
		git: newDiffGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok"}, nil
		},
		getDiff: func(ctx context.Context, host, token, owner, repo string, number int64) (string, error) {
			return "", errors.New("404 not found")
		},
		out:   &bytes.Buffer{},
		isTTY: func() bool { return false },
	}
	err := runPRDiff(context.Background(), prDiffOptions{number: 99}, env)
	if err == nil || !strings.Contains(err.Error(), "获取 PR diff 失败") {
		t.Errorf("期望 API 错误，实际: %v", err)
	}
}

func TestRunPRDiffFullOutput(t *testing.T) {
	out := &bytes.Buffer{}
	env := prDiffEnv{
		git: newDiffGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok"}, nil
		},
		getDiff: func(ctx context.Context, host, token, owner, repo string, number int64) (string, error) {
			if owner != "owner" || repo != "repo" || number != 42 {
				t.Errorf("getDiff 参数 = (%s,%s,%d), 期望 (owner,repo,42)", owner, repo, number)
			}
			return sampleDiff, nil
		},
		out:   out,
		isTTY: func() bool { return false },
	}
	err := runPRDiff(context.Background(), prDiffOptions{number: 42, color: "never"}, env)
	if err != nil {
		t.Fatalf("runPRDiff 错误: %v", err)
	}
	if out.String() != sampleDiff {
		t.Errorf("输出与预期不符\n期望:\n%s\n实际:\n%s", sampleDiff, out.String())
	}
}

func TestRunPRDiffNameOnly(t *testing.T) {
	out := &bytes.Buffer{}
	env := prDiffEnv{
		git: newDiffGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok"}, nil
		},
		getDiff: func(ctx context.Context, host, token, owner, repo string, number int64) (string, error) {
			return sampleDiff, nil
		},
		out:   out,
		isTTY: func() bool { return false },
	}
	err := runPRDiff(context.Background(), prDiffOptions{number: 1, nameOnly: true}, env)
	if err != nil {
		t.Fatalf("runPRDiff 错误: %v", err)
	}
	lines := strings.TrimSpace(out.String())
	parts := strings.Split(lines, "\n")
	if len(parts) != 2 {
		t.Fatalf("期望 2 个文件名，实际 %d: %q", len(parts), lines)
	}
	if parts[0] != "main.go" || parts[1] != "util.go" {
		t.Errorf("文件名 = %v, 期望 [main.go, util.go]", parts)
	}
}

func TestRunPRDiffColorAlways(t *testing.T) {
	out := &bytes.Buffer{}
	env := prDiffEnv{
		git: newDiffGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok"}, nil
		},
		getDiff: func(ctx context.Context, host, token, owner, repo string, number int64) (string, error) {
			return sampleDiff, nil
		},
		out:   out,
		isTTY: func() bool { return false },
	}
	err := runPRDiff(context.Background(), prDiffOptions{number: 1, color: "always"}, env)
	if err != nil {
		t.Fatalf("runPRDiff 错误: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "\033[32m+") {
		t.Errorf("期望绿色加行，实际缺少 ANSI green")
	}
	if !strings.Contains(output, "\033[31m-") {
		t.Errorf("期望红色减行，实际缺少 ANSI red")
	}
	if !strings.Contains(output, "\033[35m@@") {
		t.Errorf("期望紫色 hunk header，实际缺少 ANSI magenta")
	}
}

func TestRunPRDiffColorNeverNoANSI(t *testing.T) {
	out := &bytes.Buffer{}
	env := prDiffEnv{
		git: newDiffGitFake(),
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Token: "tok"}, nil
		},
		getDiff: func(ctx context.Context, host, token, owner, repo string, number int64) (string, error) {
			return sampleDiff, nil
		},
		out:   out,
		isTTY: func() bool { return true },
	}
	err := runPRDiff(context.Background(), prDiffOptions{number: 1, color: "never"}, env)
	if err != nil {
		t.Fatalf("runPRDiff 错误: %v", err)
	}
	if strings.Contains(out.String(), "\033[") {
		t.Errorf("color=never 不应包含 ANSI 转义码")
	}
}

func TestExtractFileNamesDeletedFile(t *testing.T) {
	diff := `diff --git a/deleted.go b/deleted.go
deleted file mode 100644
--- a/deleted.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package main
-
-func old() {}
`
	names := extractFileNames(diff)
	if len(names) != 1 || names[0] != "deleted.go" {
		t.Errorf("期望 [deleted.go], 实际 %v", names)
	}
}

func TestExtractFileNamesMixedModifyAndDelete(t *testing.T) {
	diff := `diff --git a/modified.go b/modified.go
index abc1234..def5678 100644
--- a/modified.go
+++ b/modified.go
@@ -1,2 +1,3 @@
 package main
+import "fmt"
diff --git a/deleted.go b/deleted.go
deleted file mode 100644
--- a/deleted.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package old
-
-func remove() {}
diff --git a/added.go b/added.go
new file mode 100644
--- /dev/null
+++ b/added.go
@@ -0,0 +1,3 @@
+package new
+
+func create() {}
`
	names := extractFileNames(diff)
	if len(names) != 3 {
		t.Fatalf("期望 3 个文件，实际 %d: %v", len(names), names)
	}
	want := []string{"modified.go", "deleted.go", "added.go"}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("names[%d] = %q, 期望 %q", i, names[i], w)
		}
	}
}

func TestResolveColor(t *testing.T) {
	tests := []struct {
		mode  string
		tty   bool
		want  bool
	}{
		{"always", false, true},
		{"always", true, true},
		{"never", false, false},
		{"never", true, false},
		{"auto", true, true},
		{"auto", false, false},
	}
	for _, tt := range tests {
		got := resolveColor(tt.mode, tt.tty)
		if got != tt.want {
			t.Errorf("resolveColor(%q, %v) = %v, 期望 %v", tt.mode, tt.tty, got, tt.want)
		}
	}
}
