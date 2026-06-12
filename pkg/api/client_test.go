package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetCurrentUserSuccess 验证正常响应被正确解析，且令牌作为 access_token 传递。
func TestGetCurrentUserSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/user" {
			t.Errorf("请求路径 = %q, 期望 /user", got)
		}
		if got := r.URL.Query().Get("access_token"); got != "tok123" {
			t.Errorf("access_token = %q, 期望 tok123", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"login":"alice","name":"Alice"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok123")
	user, err := client.GetCurrentUser(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentUser 返回错误: %v", err)
	}
	if user.Login != "alice" || user.Name != "Alice" || user.ID != 1 {
		t.Errorf("解析结果 = %+v, 不符合预期", user)
	}
}

// TestGetCurrentUserUnauthorized 验证 401 响应被转换为带状态码与消息的 APIError。
func TestGetCurrentUserUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "bad")
	_, err := client.GetCurrentUser(context.Background())
	if err == nil {
		t.Fatal("期望返回错误，实际为 nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("错误类型 = %T, 期望 *APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, 期望 401", apiErr.StatusCode)
	}
	if apiErr.Message != "401 Unauthorized" {
		t.Errorf("Message = %q, 期望解析出 gitee 的 message 字段", apiErr.Message)
	}
}

// TestNewClientDefaultsAndTrim 验证空 baseURL 使用默认地址，并去除尾部斜杠。
func TestNewClientDefaultsAndTrim(t *testing.T) {
	if c := NewClient("", ""); c.baseURL != "https://gitee.com/api/v5" {
		t.Errorf("默认 baseURL = %q, 不符合预期", c.baseURL)
	}
	if c := NewClient("https://example.com/api/v5/", "x"); c.baseURL != "https://example.com/api/v5" {
		t.Errorf("baseURL 未去除尾部斜杠: %q", c.baseURL)
	}
}

// TestParseErrorMessage 验证错误信息解析覆盖 message / error / 原文 / 空 多种情形。
func TestParseErrorMessage(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"message字段", `{"message":"出错了"}`, "出错了"},
		{"error字段", `{"error":"invalid"}`, "invalid"},
		{"纯文本", `boom`, "boom"},
		{"空响应", ``, "无响应内容"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseErrorMessage([]byte(tc.body)); got != tc.want {
				t.Errorf("parseErrorMessage(%q) = %q, 期望 %q", tc.body, got, tc.want)
			}
		})
	}
}

// TestCreatePullRequestSuccess 验证成功创建 PR 的场景。
func TestCreatePullRequestSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("HTTP 方法 = %s, 期望 POST", r.Method)
		}
		if got := r.URL.Path; got != "/repos/owner/repo/pulls" {
			t.Errorf("请求路径 = %q, 期望 /repos/owner/repo/pulls", got)
		}
		if got := r.URL.Query().Get("access_token"); got != "test_token" {
			t.Errorf("access_token = %q, 期望 test_token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"id": 123,
			"number": 456,
			"state": "open",
			"html_url": "https://gitee.com/owner/repo/pulls/456",
			"title": "Test PR",
			"body": "Test description",
			"user": {"id":1,"login":"testuser"},
			"head": {"label":"feature","ref":"feature","sha":"abc123"},
			"base": {"label":"main","ref":"main","sha":"def456"}
		}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test_token")
	input := &CreatePullRequestInput{
		Title: "Test PR",
		Head:  "feature",
		Base:  "main",
		Body:  "Test description",
	}

	pr, err := client.CreatePullRequest(context.Background(), "owner", "repo", input)
	if err != nil {
		t.Fatalf("CreatePullRequest 返回错误: %v", err)
	}

	if pr.Number != 456 {
		t.Errorf("PR Number = %d, 期望 456", pr.Number)
	}
	if pr.Title != "Test PR" {
		t.Errorf("PR Title = %q, 期望 Test PR", pr.Title)
	}
	if pr.HTMLURL != "https://gitee.com/owner/repo/pulls/456" {
		t.Errorf("PR URL = %q, 不符合预期", pr.HTMLURL)
	}
}

// TestCreatePullRequestValidation 验证参数校验。
func TestCreatePullRequestValidation(t *testing.T) {
	client := NewClient("", "token")
	ctx := context.Background()

	tests := []struct {
		name   string
		owner  string
		repo   string
		input  *CreatePullRequestInput
		errMsg string
	}{
		{
			name:   "nil input",
			owner:  "owner",
			repo:   "repo",
			input:  nil,
			errMsg: "input 不能为空",
		},
		{
			name:   "空 owner",
			owner:  "",
			repo:   "repo",
			input:  &CreatePullRequestInput{Title: "t", Head: "h", Base: "b"},
			errMsg: "owner 和 repo 不能为空",
		},
		{
			name:   "空 repo",
			owner:  "owner",
			repo:   "",
			input:  &CreatePullRequestInput{Title: "t", Head: "h", Base: "b"},
			errMsg: "owner 和 repo 不能为空",
		},
		{
			name:   "空 title",
			owner:  "owner",
			repo:   "repo",
			input:  &CreatePullRequestInput{Title: "", Head: "h", Base: "b"},
			errMsg: "title、head、base 是必填参数",
		},
		{
			name:   "空 head",
			owner:  "owner",
			repo:   "repo",
			input:  &CreatePullRequestInput{Title: "t", Head: "", Base: "b"},
			errMsg: "title、head、base 是必填参数",
		},
		{
			name:   "空 base",
			owner:  "owner",
			repo:   "repo",
			input:  &CreatePullRequestInput{Title: "t", Head: "h", Base: ""},
			errMsg: "title、head、base 是必填参数",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.CreatePullRequest(ctx, tt.owner, tt.repo, tt.input)
			if err == nil {
				t.Fatal("期望返回错误，实际为 nil")
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("错误信息 = %q, 期望包含 %q", err.Error(), tt.errMsg)
			}
		})
	}
}

// TestCreatePullRequestConflict 验证 PR 已存在时的错误处理。
func TestCreatePullRequestConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"已存在相同源分支和目标分支的Pull Request"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	input := &CreatePullRequestInput{
		Title: "Test PR",
		Head:  "feature",
		Base:  "main",
	}

	_, err := client.CreatePullRequest(context.Background(), "owner", "repo", input)
	if err == nil {
		t.Fatal("期望返回错误，实际为 nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("错误类型 = %T, 期望 *APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("StatusCode = %d, 期望 422", apiErr.StatusCode)
	}
}

// TestListPullRequestsSuccess 验证列表查询成功路径，断言 query 参数被正确透传。
func TestListPullRequestsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/pulls" {
			t.Errorf("路径 = %q, 期望 /repos/o/r/pulls", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("state") != "merged" {
			t.Errorf("state = %q", q.Get("state"))
		}
		if q.Get("direction") != "asc" {
			t.Errorf("direction = %q", q.Get("direction"))
		}
		if q.Get("sort") != "updated" {
			t.Errorf("sort = %q", q.Get("sort"))
		}
		if q.Get("base") != "main" {
			t.Errorf("base = %q", q.Get("base"))
		}
		if q.Get("head") != "ns:feat" {
			t.Errorf("head = %q", q.Get("head"))
		}
		if q.Get("labels") != "bug,urgent" {
			t.Errorf("labels = %q", q.Get("labels"))
		}
		if q.Get("page") != "2" {
			t.Errorf("page = %q", q.Get("page"))
		}
		if q.Get("per_page") != "10" {
			t.Errorf("per_page = %q", q.Get("per_page"))
		}
		if q.Get("milestone_number") != "3" {
			t.Errorf("milestone_number = %q", q.Get("milestone_number"))
		}
		if q.Get("access_token") != "tok" {
			t.Errorf("access_token = %q", q.Get("access_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":1,"number":10,"state":"merged","title":"first","html_url":"https://gitee.com/o/r/pulls/10",
			 "user":{"id":1,"login":"alice"},"head":{"ref":"feat","label":"o:feat","sha":"a"},"base":{"ref":"main","label":"o:main","sha":"b"}},
			{"id":2,"number":11,"state":"merged","title":"second","html_url":"https://gitee.com/o/r/pulls/11",
			 "user":{"id":2,"login":"bob"},"head":{"ref":"fix","label":"o:fix","sha":"c"},"base":{"ref":"main","label":"o:main","sha":"d"}}
		]`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	prs, err := client.ListPullRequests(context.Background(), "o", "r", &ListPullRequestsInput{
		State:           "merged",
		Direction:       "asc",
		Sort:            "updated",
		Base:            "main",
		Head:            "ns:feat",
		Labels:          "bug,urgent",
		MilestoneNumber: 3,
		Page:            2,
		PerPage:         10,
	})
	if err != nil {
		t.Fatalf("ListPullRequests 返回错误: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("len(prs) = %d, 期望 2", len(prs))
	}
	if prs[0].Number != 10 || prs[1].Number != 11 {
		t.Errorf("PR 编号 = %d/%d, 期望 10/11", prs[0].Number, prs[1].Number)
	}
}

// TestListPullRequestsValidation 验证 owner/repo 必填校验。
func TestListPullRequestsValidation(t *testing.T) {
	client := NewClient("", "tok")
	if _, err := client.ListPullRequests(context.Background(), "", "r", nil); err == nil ||
		!strings.Contains(err.Error(), "owner 和 repo 不能为空") {
		t.Errorf("期望 owner 校验错误，实际: %v", err)
	}
	if _, err := client.ListPullRequests(context.Background(), "o", "", nil); err == nil ||
		!strings.Contains(err.Error(), "owner 和 repo 不能为空") {
		t.Errorf("期望 repo 校验错误，实际: %v", err)
	}
}

// TestListPullRequestsNilInput 验证 nil input 走默认值（无 query 参数除 access_token 外）。
func TestListPullRequestsNilInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 只应该有 access_token 一个 query 参数
		q := r.URL.Query()
		for key := range q {
			if key != "access_token" {
				t.Errorf("nil input 不应携带 query %q", key)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	prs, err := client.ListPullRequests(context.Background(), "o", "r", nil)
	if err != nil {
		t.Fatalf("ListPullRequests 返回错误: %v", err)
	}
	if len(prs) != 0 {
		t.Errorf("len(prs) = %d, 期望 0", len(prs))
	}
}

// TestListPullRequestsAPIError 验证 API 错误状态码被解析为 APIError。
func TestListPullRequestsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"仓库不存在"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	_, err := client.ListPullRequests(context.Background(), "o", "missing", nil)
	if err == nil {
		t.Fatal("期望错误，实际为 nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("错误类型 = %T, 期望 *APIError", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, 期望 404", apiErr.StatusCode)
	}
}

// TestGetPullRequestSuccess 验证按编号获取 PR 详情。
func TestGetPullRequestSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/pulls/123" {
			t.Errorf("路径 = %q, 期望 /repos/o/r/pulls/123", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, 期望 GET", r.Method)
		}
		if r.URL.Query().Get("access_token") != "tok" {
			t.Errorf("access_token = %q", r.URL.Query().Get("access_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"number":123,"state":"open","title":"demo",
			"html_url":"https://gitee.com/o/r/pulls/123",
			"head":{"ref":"feat","label":"contributor:feat","sha":"a"},
			"base":{"ref":"main","label":"o:main","sha":"b"}}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pr, err := client.GetPullRequest(context.Background(), "o", "r", 123)
	if err != nil {
		t.Fatalf("GetPullRequest 返回错误: %v", err)
	}
	if pr.Number != 123 {
		t.Errorf("Number = %d, 期望 123", pr.Number)
	}
	if pr.Head.Label != "contributor:feat" {
		t.Errorf("Head.Label = %q", pr.Head.Label)
	}
	if pr.Base.Ref != "main" {
		t.Errorf("Base.Ref = %q", pr.Base.Ref)
	}
}

// TestGetPullRequestValidation 验证参数校验。
func TestGetPullRequestValidation(t *testing.T) {
	client := NewClient("", "tok")
	if _, err := client.GetPullRequest(context.Background(), "", "r", 1); err == nil ||
		!strings.Contains(err.Error(), "owner 和 repo 不能为空") {
		t.Errorf("期望 owner 校验错误，实际: %v", err)
	}
	if _, err := client.GetPullRequest(context.Background(), "o", "r", 0); err == nil ||
		!strings.Contains(err.Error(), "PR 编号必须大于 0") {
		t.Errorf("期望编号校验错误，实际: %v", err)
	}
}

// TestGetPullRequestAPIError 验证 API 错误向上传播。
func TestGetPullRequestAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	if _, err := client.GetPullRequest(context.Background(), "o", "r", 404); err == nil {
		t.Error("期望 API 错误，实际为 nil")
	}
}

// TestGetRepositorySuccess 验证仓库详情解析与路径构造。
func TestGetRepositorySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/repos/owner/repo" {
			t.Errorf("请求路径 = %q, 期望 /repos/owner/repo", got)
		}
		if got := r.URL.Query().Get("access_token"); got != "tok" {
			t.Errorf("access_token = %q, 期望 tok", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"full_name":"owner/repo","name":"repo","description":"演示仓库","private":false,"language":"Go","stargazers_count":12,"forks_count":3,"watchers_count":5,"open_issues_count":2,"default_branch":"main","html_url":"https://gitee.com/owner/repo"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	r, err := client.GetRepository(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("GetRepository 返回错误: %v", err)
	}
	if r.FullName != "owner/repo" || r.Language != "Go" || r.StargazersCount != 12 || r.OpenIssuesCount != 2 {
		t.Errorf("解析结果 = %+v, 不符合预期", r)
	}
}

// TestGetRepositoryValidation 验证空 owner/repo 的本地校验。
func TestGetRepositoryValidation(t *testing.T) {
	client := NewClient("https://example.com", "tok")
	if _, err := client.GetRepository(context.Background(), "", "repo"); err == nil {
		t.Error("期望 owner 为空时报错")
	}
	if _, err := client.GetRepository(context.Background(), "owner", ""); err == nil {
		t.Error("期望 repo 为空时报错")
	}
}

// TestListPullRequestCommentsSuccess 验证 PR 评论列表解析与路径构造。
func TestListPullRequestCommentsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/repos/owner/repo/pulls/42/comments" {
			t.Errorf("请求路径 = %q, 期望 /repos/owner/repo/pulls/42/comments", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"body":"LGTM","user":{"login":"bob"},"created_at":"2026-06-10T10:00:00+08:00"}]`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	comments, err := client.ListPullRequestComments(context.Background(), "owner", "repo", 42)
	if err != nil {
		t.Fatalf("ListPullRequestComments 返回错误: %v", err)
	}
	if len(comments) != 1 || comments[0].Body != "LGTM" || comments[0].User.Login != "bob" {
		t.Errorf("解析结果 = %+v, 不符合预期", comments)
	}
}

// TestListPullRequestCommentsValidation 验证编号与 owner/repo 的本地校验。
func TestListPullRequestCommentsValidation(t *testing.T) {
	client := NewClient("https://example.com", "tok")
	if _, err := client.ListPullRequestComments(context.Background(), "owner", "repo", 0); err == nil {
		t.Error("期望编号为 0 时报错")
	}
	if _, err := client.ListPullRequestComments(context.Background(), "", "repo", 1); err == nil {
		t.Error("期望 owner 为空时报错")
	}
}

// TestListCIStatusesSuccess 验证正常响应解析与查询参数传递。
func TestListCIStatusesSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/repos/o/r/commits/abc123/statuses" {
			t.Errorf("路径 = %q, 期望 /repos/o/r/commits/abc123/statuses", got)
		}
		q := r.URL.Query()
		if q.Get("page") != "2" {
			t.Errorf("page = %q", q.Get("page"))
		}
		if q.Get("per_page") != "5" {
			t.Errorf("per_page = %q", q.Get("per_page"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"state":"success","context":"jenkins","description":"ok"}]`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	statuses, err := client.ListCIStatuses(context.Background(), "o", "r", "abc123", &ListCIStatusesInput{Page: 2, PerPage: 5})
	if err != nil {
		t.Fatalf("ListCIStatuses 返回错误: %v", err)
	}
	if len(statuses) != 1 || statuses[0].State != "success" {
		t.Errorf("解析结果 = %+v, 不符合预期", statuses)
	}
}

// TestListCIStatusesRefEscaping 验证含 `/` 的分支名被转义为单个 path segment，
// 不会被服务端拆成多级路径（回归测试: feature/CRH-10-ci-status）。
func TestListCIStatusesRefEscaping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// EscapedPath 保留转义形式，ref 中的 / 应编码为 %2F。
		wantEscaped := "/repos/o/r/commits/feature%2FCRH-10-ci-status/statuses"
		if got := r.URL.EscapedPath(); got != wantEscaped {
			t.Errorf("EscapedPath = %q, 期望 %q", got, wantEscaped)
		}
		// 解码后 ref 应还原为完整分支名，而非被截断。
		if got := r.URL.Path; got != "/repos/o/r/commits/feature/CRH-10-ci-status/statuses" {
			t.Errorf("Path = %q, ref 被错误拆分", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	if _, err := client.ListCIStatuses(context.Background(), "o", "r", "feature/CRH-10-ci-status", nil); err != nil {
		t.Fatalf("ListCIStatuses 返回错误: %v", err)
	}
}

// TestListCIStatusesValidation 验证 owner/repo/ref 的本地校验。
func TestListCIStatusesValidation(t *testing.T) {
	client := NewClient("https://example.com", "tok")
	if _, err := client.ListCIStatuses(context.Background(), "", "r", "ref", nil); err == nil {
		t.Error("期望 owner 为空时报错")
	}
	if _, err := client.ListCIStatuses(context.Background(), "o", "r", "", nil); err == nil {
		t.Error("期望 ref 为空时报错")
	}
}

// TestGetCombinedStatusSuccess 验证聚合状态解析。
func TestGetCombinedStatusSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/repos/o/r/commits/main/status" {
			t.Errorf("路径 = %q, 期望 /repos/o/r/commits/main/status", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"success","total_count":2,"statuses":[{"id":1,"state":"success"},{"id":2,"state":"success"}]}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	combined, err := client.GetCombinedStatus(context.Background(), "o", "r", "main")
	if err != nil {
		t.Fatalf("GetCombinedStatus 返回错误: %v", err)
	}
	if combined.State != "success" || combined.TotalCount != 2 || len(combined.Statuses) != 2 {
		t.Errorf("解析结果 = %+v, 不符合预期", combined)
	}
}

// TestGetCombinedStatusRefEscaping 验证含 `/` 的 ref 被转义为单个 path segment。
func TestGetCombinedStatusRefEscaping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantEscaped := "/repos/o/r/commits/feature%2Ffoo/status"
		if got := r.URL.EscapedPath(); got != wantEscaped {
			t.Errorf("EscapedPath = %q, 期望 %q", got, wantEscaped)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"pending","total_count":0,"statuses":[]}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	if _, err := client.GetCombinedStatus(context.Background(), "o", "r", "feature/foo"); err != nil {
		t.Fatalf("GetCombinedStatus 返回错误: %v", err)
	}
}

// TestGetPullRequestDiffSuccess 验证获取 PR diff 文本。
func TestGetPullRequestDiffSuccess(t *testing.T) {
	wantDiff := "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1 +1 @@\n-old\n+new\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/repos/owner/repo/pulls/7.diff" {
			t.Errorf("路径 = %q, 期望 /repos/owner/repo/pulls/7.diff", got)
		}
		if r.URL.Query().Get("access_token") != "tok" {
			t.Errorf("access_token = %q", r.URL.Query().Get("access_token"))
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(wantDiff))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	diff, err := client.GetPullRequestDiff(context.Background(), "owner", "repo", 7)
	if err != nil {
		t.Fatalf("GetPullRequestDiff 返回错误: %v", err)
	}
	if diff != wantDiff {
		t.Errorf("diff = %q, 期望 %q", diff, wantDiff)
	}
}

// TestGetPullRequestDiffValidation 验证参数校验。
func TestGetPullRequestDiffValidation(t *testing.T) {
	client := NewClient("", "tok")
	if _, err := client.GetPullRequestDiff(context.Background(), "", "r", 1); err == nil ||
		!strings.Contains(err.Error(), "owner 和 repo 不能为空") {
		t.Errorf("期望 owner 校验错误，实际: %v", err)
	}
	if _, err := client.GetPullRequestDiff(context.Background(), "o", "r", 0); err == nil ||
		!strings.Contains(err.Error(), "PR 编号必须大于 0") {
		t.Errorf("期望编号校验错误，实际: %v", err)
	}
}

// TestGetPullRequestDiffAPIError 验证 404 错误返回 APIError。
func TestGetPullRequestDiffAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	_, err := client.GetPullRequestDiff(context.Background(), "o", "r", 999)
	if err == nil {
		t.Fatal("期望 API 错误，实际为 nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("错误类型 = %T, 期望 *APIError", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, 期望 404", apiErr.StatusCode)
	}
}

// TestGetCombinedStatusValidation 验证 owner/repo/ref 的本地校验。
func TestGetCombinedStatusValidation(t *testing.T) {
	client := NewClient("https://example.com", "tok")
	if _, err := client.GetCombinedStatus(context.Background(), "o", "", "ref"); err == nil {
		t.Error("期望 repo 为空时报错")
	}
	if _, err := client.GetCombinedStatus(context.Background(), "o", "r", ""); err == nil {
		t.Error("期望 ref 为空时报错")
	}
}

// TestMergePullRequestFormDataContract 验证 PUT /pulls/{number}/merge 严格遵循
// Gitee v5 Swagger（v5.4.92）定义的 formData contract：
//   - HTTP 方法 PUT，路径含 owner / repo / number
//   - Content-Type 为 application/x-www-form-urlencoded（不是 JSON）
//   - 字段命名：merge_method / title / description / prune_source_branch
//   - access_token 也以 form 字段提交，不放在 query
//   - 自定义合并信息映射到 description（不是 message）
func TestMergePullRequestFormDataContract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. HTTP 方法与路径
		if r.Method != http.MethodPut {
			t.Errorf("Method = %q, 期望 PUT", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/pulls/123/merge" {
			t.Errorf("Path = %q, 期望 /repos/owner/repo/pulls/123/merge", r.URL.Path)
		}
		// 2. Content-Type 必须是 form-urlencoded
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
			t.Errorf("Content-Type = %q, 期望 application/x-www-form-urlencoded", ct)
		}
		// 3. access_token 必须在 form body 中，不应残留在 query
		if got := r.URL.Query().Get("access_token"); got != "" {
			t.Errorf("query access_token = %q, 期望不在 query 中", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm 失败: %v", err)
		}
		// 4. form 字段命名严格符合 Gitee v5 contract
		if got := r.PostForm.Get("access_token"); got != "tok123" {
			t.Errorf("form access_token = %q, 期望 tok123", got)
		}
		if got := r.PostForm.Get("merge_method"); got != "squash" {
			t.Errorf("form merge_method = %q, 期望 squash", got)
		}
		if got := r.PostForm.Get("title"); got != "release v1" {
			t.Errorf("form title = %q, 期望 release v1", got)
		}
		// description 是 Gitee 的字段名，不是 message
		if got := r.PostForm.Get("description"); got != "merge body" {
			t.Errorf("form description = %q, 期望 merge body", got)
		}
		// prune_source_branch=true 必须传字符串 "true"
		if got := r.PostForm.Get("prune_source_branch"); got != "true" {
			t.Errorf("form prune_source_branch = %q, 期望 true", got)
		}
		// 不应该出现 message 字段（这是错误的字段名）
		if r.PostForm.Has("message") {
			t.Errorf("form 出现非法字段 message: %q, 应使用 description", r.PostForm.Get("message"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok123")
	err := client.MergePullRequest(context.Background(), "owner", "repo", 123, &MergePullRequestInput{
		MergeMethod:       "squash",
		Title:             "release v1",
		Description:       "merge body",
		PruneSourceBranch: true,
	})
	if err != nil {
		t.Fatalf("MergePullRequest 返回错误: %v", err)
	}
}

// TestMergePullRequestOmitsEmptyFields 验证空字段不被发送，
// 使服务端能应用默认值（merge_method 默认 merge）。
func TestMergePullRequestOmitsEmptyFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm 失败: %v", err)
		}
		// 未设置的字段应不出现在表单中
		for _, key := range []string{"title", "description", "prune_source_branch"} {
			if r.PostForm.Has(key) {
				t.Errorf("空字段 %s 不应被发送，实际值: %q", key, r.PostForm.Get(key))
			}
		}
		// merge_method 仍应被发送
		if got := r.PostForm.Get("merge_method"); got != "merge" {
			t.Errorf("merge_method = %q, 期望 merge", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	err := client.MergePullRequest(context.Background(), "o", "r", 1, &MergePullRequestInput{
		MergeMethod: "merge",
	})
	if err != nil {
		t.Fatalf("MergePullRequest 返回错误: %v", err)
	}
}

// TestMergePullRequestErrorResponse 验证非 2xx 响应被转为带状态码与消息的 APIError。
func TestMergePullRequestErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"message":"Pull Request is not mergeable"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	err := client.MergePullRequest(context.Background(), "o", "r", 1, &MergePullRequestInput{MergeMethod: "merge"})
	if err == nil {
		t.Fatal("期望返回错误，实际为 nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("错误类型 = %T, 期望 *APIError", err)
	}
	if apiErr.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("StatusCode = %d, 期望 405", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Message, "not mergeable") {
		t.Errorf("Message = %q, 期望包含 not mergeable", apiErr.Message)
	}
}

// TestMergePullRequestValidation 验证 owner/repo/number 的本地校验。
func TestMergePullRequestValidation(t *testing.T) {
	client := NewClient("https://example.com", "tok")
	if err := client.MergePullRequest(context.Background(), "", "r", 1, &MergePullRequestInput{}); err == nil {
		t.Error("期望 owner 为空时报错")
	}
	if err := client.MergePullRequest(context.Background(), "o", "", 1, &MergePullRequestInput{}); err == nil {
		t.Error("期望 repo 为空时报错")
	}
	if err := client.MergePullRequest(context.Background(), "o", "r", 0, &MergePullRequestInput{}); err == nil {
		t.Error("期望 number 为 0 时报错")
	}
}

// TestMergePullRequestPathEscaping 验证 owner/repo 中的特殊字符被正确转义。
func TestMergePullRequestPathEscaping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantEscaped := "/repos/my%2Forg/my%20repo/pulls/9/merge"
		if got := r.URL.EscapedPath(); got != wantEscaped {
			t.Errorf("EscapedPath = %q, 期望 %q", got, wantEscaped)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	if err := client.MergePullRequest(context.Background(), "my/org", "my repo", 9, &MergePullRequestInput{MergeMethod: "merge"}); err != nil {
		t.Fatalf("MergePullRequest 返回错误: %v", err)
	}
}

// TestReviewPullRequestFormDataContract 验证审查接口以 form-urlencoded 提交，
// 且 access_token / force 字段命名与 HTTP 方法、路径严格符合 Gitee v5 contract。
func TestReviewPullRequestFormDataContract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %q, 期望 POST", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/pulls/123/review" {
			t.Errorf("Path = %q, 期望 /repos/owner/repo/pulls/123/review", r.URL.Path)
		}
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
			t.Errorf("Content-Type = %q, 期望 application/x-www-form-urlencoded", ct)
		}
		// access_token 必须在 form body 中，不应残留在 query
		if got := r.URL.Query().Get("access_token"); got != "" {
			t.Errorf("query access_token = %q, 期望不在 query 中", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm 失败: %v", err)
		}
		if got := r.PostForm.Get("access_token"); got != "tok123" {
			t.Errorf("form access_token = %q, 期望 tok123", got)
		}
		if got := r.PostForm.Get("force"); got != "true" {
			t.Errorf("form force = %q, 期望 true", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok123")
	if err := client.ReviewPullRequest(context.Background(), "owner", "repo", 123, &ReviewPullRequestInput{Force: true}); err != nil {
		t.Fatalf("ReviewPullRequest 返回错误: %v", err)
	}
}

// TestReviewPullRequestOmitsForceWhenFalse 验证 force 为 false 时不发送该字段，
// 让服务端按分支保护规则默认处理。
func TestReviewPullRequestOmitsForceWhenFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm 失败: %v", err)
		}
		if r.PostForm.Has("force") {
			t.Errorf("force=false 时不应发送 force 字段，实际值: %q", r.PostForm.Get("force"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	if err := client.ReviewPullRequest(context.Background(), "o", "r", 1, &ReviewPullRequestInput{Force: false}); err != nil {
		t.Fatalf("ReviewPullRequest 返回错误: %v", err)
	}
}

// TestReviewPullRequestValidation 验证 owner/repo/number 的本地校验。
func TestReviewPullRequestValidation(t *testing.T) {
	client := NewClient("https://example.com", "tok")
	ctx := context.Background()
	if err := client.ReviewPullRequest(ctx, "", "r", 1, &ReviewPullRequestInput{}); err == nil {
		t.Error("期望 owner 为空时报错")
	}
	if err := client.ReviewPullRequest(ctx, "o", "", 1, &ReviewPullRequestInput{}); err == nil {
		t.Error("期望 repo 为空时报错")
	}
	if err := client.ReviewPullRequest(ctx, "o", "r", 0, &ReviewPullRequestInput{}); err == nil {
		t.Error("期望 number 为 0 时报错")
	}
}

// TestReviewPullRequestErrorResponse 验证非 2xx 响应被转为带状态码与消息的 APIError。
func TestReviewPullRequestErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"无权审查该 Pull Request"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	err := client.ReviewPullRequest(context.Background(), "o", "r", 1, &ReviewPullRequestInput{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("错误类型 = %T, 期望 *APIError", err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, 期望 403", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Message, "无权审查") {
		t.Errorf("Message = %q, 期望包含 无权审查", apiErr.Message)
	}
}

// TestReviewPullRequestPathEscaping 验证 owner/repo 中的特殊字符被正确转义。
func TestReviewPullRequestPathEscaping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantEscaped := "/repos/my%2Forg/my%20repo/pulls/9/review"
		if got := r.URL.EscapedPath(); got != wantEscaped {
			t.Errorf("EscapedPath = %q, 期望 %q", got, wantEscaped)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	if err := client.ReviewPullRequest(context.Background(), "my/org", "my repo", 9, &ReviewPullRequestInput{}); err != nil {
		t.Fatalf("ReviewPullRequest 返回错误: %v", err)
	}
}

// TestCreatePullRequestCommentSuccess 验证创建 PR 评论的请求格式与响应解析。
func TestCreatePullRequestCommentSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("HTTP 方法 = %s, 期望 POST", r.Method)
		}
		if got := r.URL.Path; got != "/repos/owner/repo/pulls/123/comments" {
			t.Errorf("请求路径 = %q, 期望 /repos/owner/repo/pulls/123/comments", got)
		}
		if got := r.URL.Query().Get("access_token"); got != "test_token" {
			t.Errorf("access_token = %q, 期望 test_token", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, 期望 application/json", got)
		}
		raw, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("请求体不是合法 JSON: %v", err)
		}
		if payload["body"] != "LGTM" {
			t.Errorf("请求体 body = %v, 期望 LGTM", payload["body"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"id": 789,
			"body": "LGTM",
			"html_url": "https://gitee.com/owner/repo/pulls/123#note_789",
			"user": {"id":1,"login":"testuser"},
			"created_at": "2024-01-01T00:00:00+08:00",
			"updated_at": "2024-01-01T00:00:00+08:00"
		}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test_token")
	comment, err := client.CreatePullRequestComment(context.Background(), "owner", "repo", 123, &CreatePullRequestCommentInput{Body: "LGTM"})
	if err != nil {
		t.Fatalf("CreatePullRequestComment 返回错误: %v", err)
	}
	if comment.ID != 789 {
		t.Errorf("评论 ID = %d, 期望 789", comment.ID)
	}
	if comment.HTMLURL != "https://gitee.com/owner/repo/pulls/123#note_789" {
		t.Errorf("评论 URL = %q, 不符合预期", comment.HTMLURL)
	}
}

// TestCreatePullRequestCommentValidation 验证 PR 评论的本地参数校验。
func TestCreatePullRequestCommentValidation(t *testing.T) {
	client := NewClient("https://example.com", "tok")
	ctx := context.Background()

	if _, err := client.CreatePullRequestComment(ctx, "o", "r", 1, nil); err == nil {
		t.Error("期望 input 为 nil 时报错")
	}
	if _, err := client.CreatePullRequestComment(ctx, "", "r", 1, &CreatePullRequestCommentInput{Body: "x"}); err == nil {
		t.Error("期望 owner 为空时报错")
	}
	if _, err := client.CreatePullRequestComment(ctx, "o", "r", 0, &CreatePullRequestCommentInput{Body: "x"}); err == nil {
		t.Error("期望编号为 0 时报错")
	}
	if _, err := client.CreatePullRequestComment(ctx, "o", "r", 1, &CreatePullRequestCommentInput{Body: ""}); err == nil {
		t.Error("期望评论内容为空时报错")
	}
}

// TestCreatePullRequestCommentAPIError 验证 API 返回非 2xx 时的错误处理。
func TestCreatePullRequestCommentAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	_, err := client.CreatePullRequestComment(context.Background(), "o", "r", 1, &CreatePullRequestCommentInput{Body: "x"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("错误类型 = %T, 期望 *APIError", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, 期望 404", apiErr.StatusCode)
	}
}

// TestCreateIssueCommentSuccess 验证创建 Issue 评论的请求格式与响应解析。
func TestCreateIssueCommentSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("HTTP 方法 = %s, 期望 POST", r.Method)
		}
		if got := r.URL.Path; got != "/repos/owner/repo/issues/I1ABC/comments" {
			t.Errorf("请求路径 = %q, 期望 /repos/owner/repo/issues/I1ABC/comments", got)
		}
		if got := r.URL.Query().Get("access_token"); got != "test_token" {
			t.Errorf("access_token = %q, 期望 test_token", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, 期望 application/json", got)
		}
		raw, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("请求体不是合法 JSON: %v", err)
		}
		if payload["body"] != "已修复" {
			t.Errorf("请求体 body = %v, 期望 已修复", payload["body"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"id": 10003,
			"body": "已修复",
			"html_url": "https://gitee.com/owner/repo/issues/I1ABC#note_10003",
			"user": {"id":2,"login":"bob"},
			"created_at": "2024-01-06T10:00:00+08:00",
			"updated_at": "2024-01-06T10:00:00+08:00"
		}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test_token")
	comment, err := client.CreateIssueComment(context.Background(), "owner", "repo", "I1ABC", &CreateIssueCommentInput{Body: "已修复"})
	if err != nil {
		t.Fatalf("CreateIssueComment 返回错误: %v", err)
	}
	if comment.ID != 10003 {
		t.Errorf("评论 ID = %d, 期望 10003", comment.ID)
	}
	if comment.HTMLURL != "https://gitee.com/owner/repo/issues/I1ABC#note_10003" {
		t.Errorf("评论 URL = %q, 不符合预期", comment.HTMLURL)
	}
}

// TestCreateIssueCommentValidation 验证 Issue 评论的本地参数校验。
func TestCreateIssueCommentValidation(t *testing.T) {
	client := NewClient("https://example.com", "tok")
	ctx := context.Background()

	if _, err := client.CreateIssueComment(ctx, "o", "r", "I1", nil); err == nil {
		t.Error("期望 input 为 nil 时报错")
	}
	if _, err := client.CreateIssueComment(ctx, "", "r", "I1", &CreateIssueCommentInput{Body: "x"}); err == nil {
		t.Error("期望 owner 为空时报错")
	}
	if _, err := client.CreateIssueComment(ctx, "o", "r", "", &CreateIssueCommentInput{Body: "x"}); err == nil {
		t.Error("期望编号为空时报错")
	}
	if _, err := client.CreateIssueComment(ctx, "o", "r", "I1", &CreateIssueCommentInput{Body: ""}); err == nil {
		t.Error("期望评论内容为空时报错")
	}
}

// TestCreateIssueSuccess 验证创建 Issue 的成功路径。
func TestCreateIssueSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/issues" {
			t.Errorf("路径 = %q, 期望 /repos/owner/repo/issues", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("方法 = %q, 期望 POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, 期望 application/json", got)
		}
		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":123,"number":"I1ABC","state":"open","title":"测试Issue","html_url":"https://gitee.com/owner/repo/issues/I1ABC"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	input := &CreateIssueInput{
		Title:     "测试Issue",
		Body:      "这是描述",
		Labels:    "bug,urgent",
		Assignees: "user1",
	}

	issue, err := client.CreateIssue(context.Background(), "owner", "repo", input)
	if err != nil {
		t.Fatalf("CreateIssue 返回错误: %v", err)
	}
	if issue.Number != "I1ABC" || issue.Title != "测试Issue" {
		t.Errorf("issue = %+v, 不符合预期", issue)
	}
}

// TestCreateIssueValidation 验证 CreateIssue 的输入参数校验。
func TestCreateIssueValidation(t *testing.T) {
	client := NewClient("https://example.com", "tok")
	ctx := context.Background()

	tests := []struct {
		name   string
		owner  string
		repo   string
		input  *CreateIssueInput
		errMsg string
	}{
		{
			name:   "input 为 nil",
			owner:  "o",
			repo:   "r",
			input:  nil,
			errMsg: "input 不能为空",
		},
		{
			name:   "owner 为空",
			owner:  "",
			repo:   "r",
			input:  &CreateIssueInput{Title: "test"},
			errMsg: "owner 和 repo 不能为空",
		},
		{
			name:   "repo 为空",
			owner:  "o",
			repo:   "",
			input:  &CreateIssueInput{Title: "test"},
			errMsg: "owner 和 repo 不能为空",
		},
		{
			name:   "title 为空",
			owner:  "o",
			repo:   "r",
			input:  &CreateIssueInput{Title: ""},
			errMsg: "title 是必填参数",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.CreateIssue(ctx, tt.owner, tt.repo, tt.input)
			if err == nil {
				t.Fatal("期望返回错误，实际为 nil")
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("错误信息 = %q, 期望包含 %q", err.Error(), tt.errMsg)
			}
		})
	}
}

// TestCreateIssueBadRequest 验证 400 错误响应的处理。
func TestCreateIssueBadRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"title 不能为空"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	input := &CreateIssueInput{Title: "test"}

	_, err := client.CreateIssue(context.Background(), "owner", "repo", input)
	if err == nil {
		t.Fatal("期望返回错误，实际为 nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("错误类型 = %T, 期望 *APIError", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, 期望 400", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Message, "title 不能为空") {
		t.Errorf("Message = %q, 期望包含 'title 不能为空'", apiErr.Message)
	}
}

// TestUpdatePullRequestStateSuccess 验证 PR 状态更新成功。
func TestUpdatePullRequestStateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("HTTP 方法 = %s, 期望 PATCH", r.Method)
		}
		if got := r.URL.Path; got != "/repos/owner/repo/pulls/123" {
			t.Errorf("请求路径 = %q, 期望 /repos/owner/repo/pulls/123", got)
		}
		if got := r.URL.Query().Get("access_token"); got != "tok" {
			t.Errorf("access_token = %q, 期望 tok", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": 1,
			"number": 123,
			"state": "closed",
			"html_url": "https://gitee.com/owner/repo/pulls/123",
			"title": "Test PR",
			"user": {"id":1,"login":"testuser"},
			"head": {"label":"feature","ref":"feature","sha":"abc123"},
			"base": {"label":"main","ref":"main","sha":"def456"}
		}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pr, err := client.UpdatePullRequestState(context.Background(), "owner", "repo", 123, "closed")
	if err != nil {
		t.Fatalf("UpdatePullRequestState 返回错误: %v", err)
	}
	if pr.Number != 123 {
		t.Errorf("PR Number = %d, 期望 123", pr.Number)
	}
	if pr.State != "closed" {
		t.Errorf("PR State = %q, 期望 closed", pr.State)
	}
}

// TestUpdatePullRequestStateValidation 验证参数校验。
func TestUpdatePullRequestStateValidation(t *testing.T) {
	client := NewClient("", "tok")
	ctx := context.Background()

	if _, err := client.UpdatePullRequestState(ctx, "", "repo", 1, "closed"); err == nil ||
		!strings.Contains(err.Error(), "owner 和 repo 不能为空") {
		t.Errorf("期望 owner 校验错误，实际: %v", err)
	}
	if _, err := client.UpdatePullRequestState(ctx, "owner", "", 1, "closed"); err == nil ||
		!strings.Contains(err.Error(), "owner 和 repo 不能为空") {
		t.Errorf("期望 repo 校验错误，实际: %v", err)
	}
	if _, err := client.UpdatePullRequestState(ctx, "owner", "repo", 0, "closed"); err == nil ||
		!strings.Contains(err.Error(), "PR 编号必须大于 0") {
		t.Errorf("期望编号校验错误，实际: %v", err)
	}
	if _, err := client.UpdatePullRequestState(ctx, "owner", "repo", 1, "invalid"); err == nil ||
		!strings.Contains(err.Error(), "state 必须为 open 或 closed") {
		t.Errorf("期望 state 校验错误，实际: %v", err)
	}
}

// TestUpdateIssueStateSuccess 验证 Issue 状态更新成功。
func TestUpdateIssueStateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("HTTP 方法 = %s, 期望 PATCH", r.Method)
		}
		if got := r.URL.Path; got != "/repos/owner/repo/issues/I123" {
			t.Errorf("请求路径 = %q, 期望 /repos/owner/repo/issues/I123", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": 1,
			"number": "I123",
			"state": "closed",
			"html_url": "https://gitee.com/owner/repo/issues/I123",
			"title": "Test Issue",
			"body": "Test body",
			"user": {"id":1,"login":"testuser"},
			"created_at": "2024-01-01T10:00:00+08:00",
			"updated_at": "2024-01-02T10:00:00+08:00"
		}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	issue, err := client.UpdateIssueState(context.Background(), "owner", "repo", "I123", "closed")
	if err != nil {
		t.Fatalf("UpdateIssueState 返回错误: %v", err)
	}
	if issue.Number != "I123" {
		t.Errorf("Issue Number = %q, 期望 I123", issue.Number)
	}
	if issue.State != "closed" {
		t.Errorf("Issue State = %q, 期望 closed", issue.State)
	}
}

// TestUpdateIssueStateValidation 验证参数校验。
func TestUpdateIssueStateValidation(t *testing.T) {
	client := NewClient("", "tok")
	ctx := context.Background()

	if _, err := client.UpdateIssueState(ctx, "", "repo", "I1", "closed"); err == nil ||
		!strings.Contains(err.Error(), "owner 和 repo 不能为空") {
		t.Errorf("期望 owner 校验错误，实际: %v", err)
	}
	if _, err := client.UpdateIssueState(ctx, "owner", "", "I1", "closed"); err == nil ||
		!strings.Contains(err.Error(), "owner 和 repo 不能为空") {
		t.Errorf("期望 repo 校验错误，实际: %v", err)
	}
	if _, err := client.UpdateIssueState(ctx, "owner", "repo", "", "closed"); err == nil ||
		!strings.Contains(err.Error(), "Issue 编号不能为空") {
		t.Errorf("期望编号校验错误，实际: %v", err)
	}
	if _, err := client.UpdateIssueState(ctx, "owner", "repo", "I1", "invalid"); err == nil ||
		!strings.Contains(err.Error(), "state 必须为 open 或 closed") {
		t.Errorf("期望 state 校验错误，实际: %v", err)
	}
}

// TestEditPullRequestPartialUpdate 验证 PATCH 仅提交显式指定的字段。
func TestEditPullRequestPartialUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Method = %q, 期望 PATCH", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/pulls/123" {
			t.Errorf("Path = %q, 期望 /repos/owner/repo/pulls/123", r.URL.Path)
		}
		if got := r.URL.Query().Get("access_token"); got != "tok" {
			t.Errorf("access_token = %q, 期望 tok", got)
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("解析请求体失败: %v", err)
		}
		// 仅 title 与 labels 被指定，其余字段不得出现
		if got := payload["title"]; got != "新标题" {
			t.Errorf("title = %v, 期望 新标题", got)
		}
		if got := payload["labels"]; got != "bug,urgent" {
			t.Errorf("labels = %v, 期望 bug,urgent", got)
		}
		if _, ok := payload["body"]; ok {
			t.Errorf("body 不应出现在请求体中: %v", payload["body"])
		}
		if _, ok := payload["assignees"]; ok {
			t.Errorf("assignees 不应出现在请求体中: %v", payload["assignees"])
		}
		if _, ok := payload["milestone_number"]; ok {
			t.Errorf("milestone_number 不应出现在请求体中: %v", payload["milestone_number"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"number":123,"title":"新标题","html_url":"https://gitee.com/owner/repo/pulls/123","state":"open"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	title := "新标题"
	labels := "bug,urgent"
	pr, err := client.EditPullRequest(context.Background(), "owner", "repo", 123, &EditPullRequestInput{
		Title:  &title,
		Labels: &labels,
	})
	if err != nil {
		t.Fatalf("EditPullRequest 返回错误: %v", err)
	}
	if pr.Title != "新标题" {
		t.Errorf("返回 Title = %q, 期望 新标题", pr.Title)
	}
}

// TestEditPullRequestClearField 验证传空字符串可显式清空字段（区别于不修改）。
func TestEditPullRequestClearField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("解析请求体失败: %v", err)
		}
		// labels 显式设为空字符串，必须出现在请求体中
		v, ok := payload["labels"]
		if !ok {
			t.Errorf("labels 应出现在请求体中（清空意图）")
		}
		if v != "" {
			t.Errorf("labels = %v, 期望空字符串", v)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"number":123,"title":"t","html_url":"u","state":"open"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	empty := ""
	_, err := client.EditPullRequest(context.Background(), "owner", "repo", 123, &EditPullRequestInput{
		Labels: &empty,
	})
	if err != nil {
		t.Fatalf("EditPullRequest 返回错误: %v", err)
	}
}

// TestEditPullRequestValidation 验证参数校验。
func TestEditPullRequestValidation(t *testing.T) {
	client := NewClient("", "tok")
	ctx := context.Background()
	title := "t"

	if _, err := client.EditPullRequest(ctx, "", "repo", 1, &EditPullRequestInput{Title: &title}); err == nil ||
		!strings.Contains(err.Error(), "owner 和 repo 不能为空") {
		t.Errorf("期望 owner 校验错误，实际: %v", err)
	}
	if _, err := client.EditPullRequest(ctx, "owner", "repo", 0, &EditPullRequestInput{Title: &title}); err == nil ||
		!strings.Contains(err.Error(), "PR 编号必须大于 0") {
		t.Errorf("期望编号校验错误，实际: %v", err)
	}
	if _, err := client.EditPullRequest(ctx, "owner", "repo", 1, &EditPullRequestInput{}); err == nil ||
		!strings.Contains(err.Error(), "至少需要指定一个待修改字段") {
		t.Errorf("期望空字段校验错误，实际: %v", err)
	}
	if _, err := client.EditPullRequest(ctx, "owner", "repo", 1, nil); err == nil ||
		!strings.Contains(err.Error(), "至少需要指定一个待修改字段") {
		t.Errorf("期望 nil input 校验错误，实际: %v", err)
	}
}

// TestEditPullRequestAPIError 验证 API 错误向上传播。
func TestEditPullRequestAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"无权限"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	title := "t"
	_, err := client.EditPullRequest(context.Background(), "owner", "repo", 1, &EditPullRequestInput{Title: &title})
	if err == nil {
		t.Fatal("期望错误但没有返回")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("期望 403 APIError，实际: %v", err)
	}
}

// TestEditIssuePartialUpdate 验证 Issue PATCH 仅提交显式字段，且字段名符合 Gitee 约定（assignee/milestone）。
func TestEditIssuePartialUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Method = %q, 期望 PATCH", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/issues/I123" {
			t.Errorf("Path = %q, 期望 /repos/owner/repo/issues/I123", r.URL.Path)
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("解析请求体失败: %v", err)
		}
		// repo 字段必须随体提交（与状态更新接口一致）
		if got := payload["repo"]; got != "repo" {
			t.Errorf("repo = %v, 期望 repo", got)
		}
		if got := payload["title"]; got != "改标题" {
			t.Errorf("title = %v, 期望 改标题", got)
		}
		// 指派人字段名为 assignee（单数），不是 assignees
		if got := payload["assignee"]; got != "user1" {
			t.Errorf("assignee = %v, 期望 user1", got)
		}
		if _, ok := payload["assignees"]; ok {
			t.Errorf("不应出现 assignees 字段（应使用 assignee）")
		}
		if got, ok := payload["milestone"]; !ok || got != float64(5) {
			t.Errorf("milestone = %v, 期望 5", got)
		}
		if _, ok := payload["body"]; ok {
			t.Errorf("body 不应出现在请求体中")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"number":"I123","title":"改标题","html_url":"https://gitee.com/owner/repo/issues/I123","state":"open"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	title := "改标题"
	assignee := "user1"
	var ms int64 = 5
	issue, err := client.EditIssue(context.Background(), "owner", "repo", "I123", &EditIssueInput{
		Title:           &title,
		Assignee:        &assignee,
		MilestoneNumber: &ms,
	})
	if err != nil {
		t.Fatalf("EditIssue 返回错误: %v", err)
	}
	if issue.Number != "I123" || issue.Title != "改标题" {
		t.Errorf("返回 Issue 不符: %+v", issue)
	}
}

// TestEditIssueValidation 验证 Issue 编辑参数校验。
func TestEditIssueValidation(t *testing.T) {
	client := NewClient("", "tok")
	ctx := context.Background()
	title := "t"

	if _, err := client.EditIssue(ctx, "owner", "", "I1", &EditIssueInput{Title: &title}); err == nil ||
		!strings.Contains(err.Error(), "owner 和 repo 不能为空") {
		t.Errorf("期望 repo 校验错误，实际: %v", err)
	}
	if _, err := client.EditIssue(ctx, "owner", "repo", "", &EditIssueInput{Title: &title}); err == nil ||
		!strings.Contains(err.Error(), "Issue 编号不能为空") {
		t.Errorf("期望编号校验错误，实际: %v", err)
	}
	if _, err := client.EditIssue(ctx, "owner", "repo", "I1", &EditIssueInput{}); err == nil ||
		!strings.Contains(err.Error(), "至少需要指定一个待修改字段") {
		t.Errorf("期望空字段校验错误，实际: %v", err)
	}
	if _, err := client.EditIssue(ctx, "owner", "repo", "I1", nil); err == nil ||
		!strings.Contains(err.Error(), "至少需要指定一个待修改字段") {
		t.Errorf("期望 nil input 校验错误，实际: %v", err)
	}
}
