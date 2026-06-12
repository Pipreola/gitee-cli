package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCreateRepositoryUser 验证个人仓库创建命中 POST /user/repos，并正确传递 body。
func TestCreateRepositoryUser(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, 期望 POST", r.Method)
		}
		if r.URL.Path != "/user/repos" {
			t.Errorf("path = %q, 期望 /user/repos", r.URL.Path)
		}
		if got := r.URL.Query().Get("access_token"); got != "tok" {
			t.Errorf("access_token = %q, 期望 tok", got)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1,"full_name":"alice/my-repo","name":"my-repo","private":true,"html_url":"https://gitee.com/alice/my-repo"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	r, err := client.CreateRepository(context.Background(), "", &CreateRepositoryInput{
		Name:        "my-repo",
		Description: "desc",
		Private:     true,
	})
	if err != nil {
		t.Fatalf("CreateRepository 错误: %v", err)
	}
	if r.FullName != "alice/my-repo" || !r.Private {
		t.Errorf("解析结果 = %+v, 不符合预期", r)
	}
	if gotBody["name"] != "my-repo" || gotBody["description"] != "desc" || gotBody["private"] != true {
		t.Errorf("请求体 = %+v, 不符合预期", gotBody)
	}
}

// TestCreateRepositoryOrg 验证组织仓库创建命中 POST /orgs/{org}/repos。
func TestCreateRepositoryOrg(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orgs/my-org/repos" {
			t.Errorf("path = %q, 期望 /orgs/my-org/repos", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":2,"full_name":"my-org/repo","name":"repo"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	r, err := client.CreateRepository(context.Background(), "my-org", &CreateRepositoryInput{Name: "repo"})
	if err != nil {
		t.Fatalf("CreateRepository 错误: %v", err)
	}
	if r.FullName != "my-org/repo" {
		t.Errorf("FullName = %q, 期望 my-org/repo", r.FullName)
	}
}

// TestCreateRepositoryValidation 验证缺少 name 或 input 时报错。
func TestCreateRepositoryValidation(t *testing.T) {
	client := NewClient("http://example.com", "tok")
	if _, err := client.CreateRepository(context.Background(), "", nil); err == nil {
		t.Error("input 为 nil 时期望报错")
	}
	if _, err := client.CreateRepository(context.Background(), "", &CreateRepositoryInput{}); err == nil {
		t.Error("name 为空时期望报错")
	}
}

// TestForkRepository 验证 fork 命中 POST /repos/:owner/:repo/forks。
func TestForkRepository(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, 期望 POST", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/forks" {
			t.Errorf("path = %q, 期望 /repos/owner/repo/forks", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) > 0 {
			_ = json.Unmarshal(body, &gotBody)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":3,"full_name":"me/repo","name":"repo","fork":true,"html_url":"https://gitee.com/me/repo"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	r, err := client.ForkRepository(context.Background(), "owner", "repo", &ForkRepositoryInput{Organization: "my-org"})
	if err != nil {
		t.Fatalf("ForkRepository 错误: %v", err)
	}
	if !r.Fork || r.FullName != "me/repo" {
		t.Errorf("解析结果 = %+v, 不符合预期", r)
	}
	if gotBody["organization"] != "my-org" {
		t.Errorf("请求体 = %+v, 期望含 organization=my-org", gotBody)
	}
}

// TestForkRepositoryNilInput 验证 input 为 nil 时不发送 body 也能成功。
func TestForkRepositoryNilInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) != 0 {
			t.Errorf("期望空 body，实际 = %q", string(body))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":4,"full_name":"me/repo","name":"repo","fork":true}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	if _, err := client.ForkRepository(context.Background(), "owner", "repo", nil); err != nil {
		t.Fatalf("ForkRepository 错误: %v", err)
	}
}

// TestForkRepositoryValidation 验证空 owner/repo 报错。
func TestForkRepositoryValidation(t *testing.T) {
	client := NewClient("http://example.com", "tok")
	if _, err := client.ForkRepository(context.Background(), "", "repo", nil); err == nil {
		t.Error("owner 为空时期望报错")
	}
	if _, err := client.ForkRepository(context.Background(), "owner", "", nil); err == nil {
		t.Error("repo 为空时期望报错")
	}
}

// TestListRepositoriesUser 验证个人仓库列表命中 GET /user/repos 并传递查询参数。
func TestListRepositoriesUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" {
			t.Errorf("path = %q, 期望 /user/repos", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("visibility") != "private" {
			t.Errorf("visibility = %q, 期望 private", q.Get("visibility"))
		}
		if q.Get("sort") != "updated" {
			t.Errorf("sort = %q, 期望 updated", q.Get("sort"))
		}
		if q.Get("per_page") != "10" {
			t.Errorf("per_page = %q, 期望 10", q.Get("per_page"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"full_name":"me/a","name":"a"},{"id":2,"full_name":"me/b","name":"b"}]`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	repos, err := client.ListRepositories(context.Background(), "", &ListRepositoriesInput{
		Visibility: "private",
		Sort:       "updated",
		PerPage:    10,
	})
	if err != nil {
		t.Fatalf("ListRepositories 错误: %v", err)
	}
	if len(repos) != 2 || repos[0].FullName != "me/a" {
		t.Errorf("解析结果 = %+v, 不符合预期", repos)
	}
}

// TestListRepositoriesOrg 验证组织仓库列表命中 GET /orgs/{org}/repos，
// 且 visibility/affiliation 参数不会被发送（组织接口不支持）。
func TestListRepositoriesOrg(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orgs/my-org/repos" {
			t.Errorf("path = %q, 期望 /orgs/my-org/repos", r.URL.Path)
		}
		if r.URL.Query().Get("visibility") != "" {
			t.Errorf("组织接口不应发送 visibility 参数，实际 = %q", r.URL.Query().Get("visibility"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"full_name":"my-org/a","name":"a"}]`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	repos, err := client.ListRepositories(context.Background(), "my-org", &ListRepositoriesInput{Visibility: "private"})
	if err != nil {
		t.Fatalf("ListRepositories 错误: %v", err)
	}
	if len(repos) != 1 || repos[0].FullName != "my-org/a" {
		t.Errorf("解析结果 = %+v, 不符合预期", repos)
	}
}
