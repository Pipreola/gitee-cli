// Package api 封装 Gitee OpenAPI v5 的 HTTP 客户端。
// 提供认证、统一错误处理与基础请求能力，供上层命令复用。
//
// 参考: https://gitee.com/api/v5/swagger
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultTimeout 是 HTTP 请求的默认超时时间。
const defaultTimeout = 30 * time.Second

// escapePathSegment 对 URL path segment 进行转义，确保 owner/repo/ref 中的
// 特殊字符（尤其是 `/`，如分支名 feature/foo）不会被拆分成多个 path segment。
func escapePathSegment(s string) string {
	return url.PathEscape(s)
}

// Client 是 Gitee API 客户端。
type Client struct {
	// baseURL 是 API 基础地址，例如 https://gitee.com/api/v5。
	baseURL string
	// token 是访问令牌，作为查询参数 access_token 附加到请求中（Gitee v5 约定）。
	token string
	// httpClient 是底层 HTTP 客户端，可注入以便测试。
	httpClient *http.Client
}

// Option 是 Client 的可选配置函数。
type Option func(*Client)

// WithHTTPClient 注入自定义 http.Client，主要用于测试。
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// WithTimeout 设置 HTTP 请求超时时间。
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// NewClient 创建一个 Gitee API 客户端。
// baseURL 为空时使用默认地址；token 可为空（仅访问公开接口）。
func NewClient(baseURL, token string, opts ...Option) *Client {
	if baseURL == "" {
		baseURL = "https://gitee.com/api/v5"
	}
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: defaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// APIError 表示 Gitee API 返回的错误响应。
type APIError struct {
	// StatusCode 是 HTTP 状态码。
	StatusCode int
	// Message 是错误信息，尽量从响应体中解析。
	Message string
}

// Error 实现 error 接口。
func (e *APIError) Error() string {
	return fmt.Sprintf("gitee api 错误 (HTTP %d): %s", e.StatusCode, e.Message)
}

// do 执行一次请求，处理认证与错误，并将成功响应体反序列化到 out（out 可为 nil）。
func (c *Client) do(ctx context.Context, method, path string, query url.Values, out interface{}) error {
	return c.doWithBody(ctx, method, path, query, nil, out)
}

// doWithBody 执行一次带 body 的请求（用于 POST / PATCH），处理认证与错误。
func (c *Client) doWithBody(ctx context.Context, method, path string, query url.Values, body io.Reader, out interface{}) error {
	if query == nil {
		query = url.Values{}
	}
	// Gitee v5 认证: 通过 access_token 查询参数传递令牌。
	if c.token != "" {
		query.Set("access_token", c.token)
	}

	fullURL := c.baseURL + path
	if encoded := query.Encode(); encoded != "" {
		fullURL += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    parseErrorMessage(respBody),
		}
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}
	return nil
}

// parseErrorMessage 尝试从 Gitee 错误响应体中提取人类可读的错误信息。
// Gitee 错误体常见形如 {"message": "..."} 或 {"error": "..."}。
func parseErrorMessage(body []byte) string {
	if len(body) == 0 {
		return "无响应内容"
	}
	var parsed struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.Message != "" {
			return parsed.Message
		}
		if parsed.Error != "" {
			return parsed.Error
		}
	}
	return strings.TrimSpace(string(body))
}

// User 表示 Gitee 用户的精简信息。
type User struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

// GetCurrentUser 调用 GET /user 获取当前认证用户信息。
// 该接口用于验证令牌有效性，是最小可用的连通性测试接口。
func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	var u User
	if err := c.do(ctx, http.MethodGet, "/user", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// PullRequest 表示 Gitee Pull Request 的完整信息。
type PullRequest struct {
	ID        int64  `json:"id"`
	Number    int64  `json:"number"`
	State     string `json:"state"`
	HTMLURL   string `json:"html_url"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	User      User   `json:"user"`
	Head      Branch `json:"head"`
	Base      Branch `json:"base"`
	Mergeable bool   `json:"mergeable"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	MergedAt  string `json:"merged_at,omitempty"`
	ClosedAt  string `json:"closed_at,omitempty"`
}

// Branch 表示分支信息。
type Branch struct {
	Label string `json:"label"`
	Ref   string `json:"ref"`
	Sha   string `json:"sha"`
}

// CreatePullRequestInput 是创建 Pull Request 的输入参数。
type CreatePullRequestInput struct {
	// Title 是 PR 的标题（必填）。
	Title string `json:"title"`
	// Head 是源分支名称（必填），格式：namespace:branch_name 或 branch_name。
	Head string `json:"head"`
	// Base 是目标分支名称（必填），通常是 master 或 main。
	Base string `json:"base"`
	// Body 是 PR 的描述内容（可选）。
	Body string `json:"body,omitempty"`
	// Draft 标记是否创建为草稿 PR（可选）。
	Draft bool `json:"draft,omitempty"`
	// Labels 是标签列表（可选），逗号分隔的字符串。
	Labels string `json:"labels,omitempty"`
	// MilestoneNumber 是里程碑编号（可选）。
	MilestoneNumber int64 `json:"milestone_number,omitempty"`
	// Assignees 是指派的审阅者（可选），逗号分隔的用户名。
	Assignees string `json:"assignees,omitempty"`
	// Testers 是指派的测试者（可选），逗号分隔的用户名。
	Testers string `json:"testers,omitempty"`
}

// ListPullRequestsInput 是查询 Pull Request 列表的输入参数。
// 所有字段均为可选；空值由客户端跳过，让服务端使用默认值。
//
// 参考: GET /api/v5/repos/{owner}/{repo}/pulls
type ListPullRequestsInput struct {
	// State 是 PR 状态：open（默认）/ closed / merged / all。
	State string
	// Head 是源分支过滤器，格式为 namespace:branch。
	Head string
	// Base 是目标分支过滤器，例如 main。
	Base string
	// Sort 是排序字段：created（默认）/ updated / popularity / long-running。
	Sort string
	// Direction 是排序方向：asc / desc（默认）。
	Direction string
	// MilestoneNumber 是里程碑编号过滤器。
	MilestoneNumber int64
	// Labels 是标签过滤器，逗号分隔。
	Labels string
	// Page 是页码（从 1 开始，默认 1）。
	Page int
	// PerPage 是每页数量（1-100，默认 20）。
	PerPage int
}

// ListPullRequests 调用 GET /repos/:owner/:repo/pulls 获取 PR 列表。
//
// 注意：Gitee v5 接口本身不支持按作者过滤，调用方如需 --author 过滤
// 应在拿到结果后基于 PullRequest.User.Login 自行筛选。
func (c *Client) ListPullRequests(ctx context.Context, owner, repo string, input *ListPullRequestsInput) ([]PullRequest, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}

	query := url.Values{}
	if input != nil {
		if input.State != "" {
			query.Set("state", input.State)
		}
		if input.Head != "" {
			query.Set("head", input.Head)
		}
		if input.Base != "" {
			query.Set("base", input.Base)
		}
		if input.Sort != "" {
			query.Set("sort", input.Sort)
		}
		if input.Direction != "" {
			query.Set("direction", input.Direction)
		}
		if input.MilestoneNumber > 0 {
			query.Set("milestone_number", fmt.Sprintf("%d", input.MilestoneNumber))
		}
		if input.Labels != "" {
			query.Set("labels", input.Labels)
		}
		if input.Page > 0 {
			query.Set("page", fmt.Sprintf("%d", input.Page))
		}
		if input.PerPage > 0 {
			query.Set("per_page", fmt.Sprintf("%d", input.PerPage))
		}
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls", escapePathSegment(owner), escapePathSegment(repo))

	var prs []PullRequest
	if err := c.do(ctx, http.MethodGet, path, query, &prs); err != nil {
		return nil, err
	}
	return prs, nil
}

// CreatePullRequest 调用 POST /repos/:owner/:repo/pulls 创建 Pull Request。
func (c *Client) CreatePullRequest(ctx context.Context, owner, repo string, input *CreatePullRequestInput) (*PullRequest, error) {
	if input == nil {
		return nil, fmt.Errorf("input 不能为空")
	}
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if input.Title == "" || input.Head == "" || input.Base == "" {
		return nil, fmt.Errorf("title、head、base 是必填参数")
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls", escapePathSegment(owner), escapePathSegment(repo))

	// 构造请求体
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	var pr PullRequest
	if err := c.doWithBody(ctx, http.MethodPost, path, nil, strings.NewReader(string(body)), &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// GetPullRequest 获取指定编号的 Pull Request 详情。
func (c *Client) GetPullRequest(ctx context.Context, owner, repo string, number int64) (*PullRequest, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if number <= 0 {
		return nil, fmt.Errorf("PR 编号必须大于 0")
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", escapePathSegment(owner), escapePathSegment(repo), number)
	var pr PullRequest
	if err := c.do(ctx, http.MethodGet, path, nil, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// UpdatePullRequestState 调用 PATCH /repos/:owner/:repo/pulls/:number 更新 PR 状态。
// state 可以是 "open" 或 "closed"。
func (c *Client) UpdatePullRequestState(ctx context.Context, owner, repo string, number int64, state string) (*PullRequest, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if number <= 0 {
		return nil, fmt.Errorf("PR 编号必须大于 0")
	}
	if state != "open" && state != "closed" {
		return nil, fmt.Errorf("state 必须为 open 或 closed")
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", escapePathSegment(owner), escapePathSegment(repo), number)

	body, err := json.Marshal(map[string]string{"state": state})
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	var pr PullRequest
	if err := c.doWithBody(ctx, http.MethodPatch, path, nil, strings.NewReader(string(body)), &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// EditPullRequestInput 是编辑 Pull Request 的输入参数。
//
// 所有字段均为指针类型，遵循 PATCH 部分更新语义：
//   - nil 表示该字段保持不变（不提交到请求体）；
//   - 非 nil（即使指向空字符串/0）表示显式更新为该值。
//
// 这样调用方可区分「不修改」与「清空」两种意图。
type EditPullRequestInput struct {
	// Title 是 PR 标题。
	Title *string
	// Body 是 PR 描述内容。
	Body *string
	// Labels 是标签列表，逗号分隔的字符串。
	Labels *string
	// Assignees 是指派的审阅者，逗号分隔的用户名。
	Assignees *string
	// MilestoneNumber 是里程碑编号。
	MilestoneNumber *int64
}

// isEmpty 判断是否没有任何待更新字段。
func (in *EditPullRequestInput) isEmpty() bool {
	if in == nil {
		return true
	}
	return in.Title == nil && in.Body == nil && in.Labels == nil &&
		in.Assignees == nil && in.MilestoneNumber == nil
}

// EditPullRequest 调用 PATCH /repos/:owner/:repo/pulls/:number 编辑 PR 的标题/正文/标签/指派人/里程碑。
// 仅提交非 nil 字段，符合部分更新语义。
func (c *Client) EditPullRequest(ctx context.Context, owner, repo string, number int64, input *EditPullRequestInput) (*PullRequest, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if number <= 0 {
		return nil, fmt.Errorf("PR 编号必须大于 0")
	}
	if input.isEmpty() {
		return nil, fmt.Errorf("至少需要指定一个待修改字段")
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", escapePathSegment(owner), escapePathSegment(repo), number)

	payload := map[string]interface{}{}
	if input.Title != nil {
		payload["title"] = *input.Title
	}
	if input.Body != nil {
		payload["body"] = *input.Body
	}
	if input.Labels != nil {
		payload["labels"] = *input.Labels
	}
	if input.Assignees != nil {
		payload["assignees"] = *input.Assignees
	}
	if input.MilestoneNumber != nil {
		payload["milestone_number"] = *input.MilestoneNumber
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	var pr PullRequest
	if err := c.doWithBody(ctx, http.MethodPatch, path, nil, strings.NewReader(string(body)), &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// Repository 表示 Gitee 仓库的信息（精简自 GET /repos/:owner/:repo）。
type Repository struct {
	ID              int64  `json:"id"`
	FullName        string `json:"full_name"`
	HumanName       string `json:"human_name"`
	Name            string `json:"name"`
	Owner           User   `json:"owner"`
	Description     string `json:"description"`
	Private         bool   `json:"private"`
	Fork            bool   `json:"fork"`
	HTMLURL         string `json:"html_url"`
	Homepage        string `json:"homepage"`
	Language        string `json:"language"`
	StargazersCount int64  `json:"stargazers_count"`
	ForksCount      int64  `json:"forks_count"`
	WatchersCount   int64  `json:"watchers_count"`
	OpenIssuesCount int64  `json:"open_issues_count"`
	DefaultBranch   string `json:"default_branch"`
	License         string `json:"license"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	PushedAt        string `json:"pushed_at"`
}

// GetRepository 调用 GET /repos/:owner/:repo 获取仓库详情。
func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}

	path := fmt.Sprintf("/repos/%s/%s", escapePathSegment(owner), escapePathSegment(repo))
	var r Repository
	if err := c.do(ctx, http.MethodGet, path, nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateRepositoryInput 是创建仓库的输入参数。
// 对应 Gitee v5 的 POST /user/repos（个人仓库）与 POST /orgs/{org}/repos（组织仓库）。
type CreateRepositoryInput struct {
	// Name 是仓库名称（必填）。
	Name string `json:"name"`
	// Description 是仓库描述（可选）。
	Description string `json:"description,omitempty"`
	// Homepage 是仓库主页地址（可选）。
	Homepage string `json:"homepage,omitempty"`
	// Private 标记是否为私有仓库（可选，默认公开）。
	Private bool `json:"private,omitempty"`
	// AutoInit 标记是否自动初始化仓库（生成 README）（可选）。
	AutoInit bool `json:"auto_init,omitempty"`
}

// CreateRepository 创建一个仓库。
// org 为空时在当前认证用户名下创建（POST /user/repos）；
// org 非空时在指定组织下创建（POST /orgs/{org}/repos）。
func (c *Client) CreateRepository(ctx context.Context, org string, input *CreateRepositoryInput) (*Repository, error) {
	if input == nil {
		return nil, fmt.Errorf("input 不能为空")
	}
	if input.Name == "" {
		return nil, fmt.Errorf("仓库名称（name）是必填参数")
	}

	var path string
	if org != "" {
		path = fmt.Sprintf("/orgs/%s/repos", escapePathSegment(org))
	} else {
		path = "/user/repos"
	}

	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	var r Repository
	if err := c.doWithBody(ctx, http.MethodPost, path, nil, strings.NewReader(string(body)), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ForkRepositoryInput 是 fork 仓库的可选输入参数。
type ForkRepositoryInput struct {
	// Organization 是 fork 到的目标组织（可选）。为空时 fork 到当前认证用户名下。
	Organization string `json:"organization,omitempty"`
	// Name 是 fork 后的新仓库名称（可选）。为空时沿用源仓库名称。
	Name string `json:"name,omitempty"`
	// Path 是 fork 后的新仓库路径（可选）。
	Path string `json:"path,omitempty"`
}

// ForkRepository 调用 POST /repos/:owner/:repo/forks fork 一个仓库。
// input 可为 nil，表示 fork 到当前认证用户名下并沿用源仓库名称。
func (c *Client) ForkRepository(ctx context.Context, owner, repo string, input *ForkRepositoryInput) (*Repository, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}

	path := fmt.Sprintf("/repos/%s/%s/forks", escapePathSegment(owner), escapePathSegment(repo))

	var reader io.Reader
	if input != nil {
		body, err := json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("序列化请求体失败: %w", err)
		}
		reader = strings.NewReader(string(body))
	}

	var r Repository
	if err := c.doWithBody(ctx, http.MethodPost, path, nil, reader, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRepositoriesInput 是查询仓库列表的输入参数。
// 所有字段均为可选；空值由客户端跳过，让服务端使用默认值。
//
// 参考: GET /api/v5/user/repos 与 GET /api/v5/orgs/{org}/repos
type ListRepositoriesInput struct {
	// Visibility 是仓库可见性过滤：public / private / all（默认 all）。仅个人仓库接口支持。
	Visibility string
	// Affiliation 是用户与仓库的关系过滤：owner / collaborator / organization_member。仅个人仓库接口支持。
	Affiliation string
	// Type 是仓库类型过滤：all / owner / member 等。仅个人仓库接口支持。
	Type string
	// Sort 是排序字段：created（默认）/ updated / pushed / full_name。
	Sort string
	// Direction 是排序方向：asc / desc。
	Direction string
	// Page 是页码（从 1 开始，默认 1）。
	Page int
	// PerPage 是每页数量（1-100，默认 20）。
	PerPage int
}

// ListRepositories 获取仓库列表。
// org 为空时列出当前认证用户的仓库（GET /user/repos）；
// org 非空时列出指定组织的仓库（GET /orgs/{org}/repos）。
func (c *Client) ListRepositories(ctx context.Context, org string, input *ListRepositoriesInput) ([]Repository, error) {
	query := url.Values{}
	if input != nil {
		// Visibility / Affiliation / Type 仅个人仓库接口有意义，组织接口会忽略。
		if org == "" {
			if input.Visibility != "" {
				query.Set("visibility", input.Visibility)
			}
			if input.Affiliation != "" {
				query.Set("affiliation", input.Affiliation)
			}
		}
		if input.Type != "" {
			query.Set("type", input.Type)
		}
		if input.Sort != "" {
			query.Set("sort", input.Sort)
		}
		if input.Direction != "" {
			query.Set("direction", input.Direction)
		}
		if input.Page > 0 {
			query.Set("page", fmt.Sprintf("%d", input.Page))
		}
		if input.PerPage > 0 {
			query.Set("per_page", fmt.Sprintf("%d", input.PerPage))
		}
	}

	var path string
	if org != "" {
		path = fmt.Sprintf("/orgs/%s/repos", escapePathSegment(org))
	} else {
		path = "/user/repos"
	}

	var repos []Repository
	if err := c.do(ctx, http.MethodGet, path, query, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// Comment 表示 Gitee 的评论（PR 评论与 Issue 评论结构一致）。
type Comment struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	User      User   `json:"user"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ListPullRequestComments 调用 GET /repos/:owner/:repo/pulls/:number/comments 获取 PR 评论列表。
func (c *Client) ListPullRequestComments(ctx context.Context, owner, repo string, number int64) ([]Comment, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if number <= 0 {
		return nil, fmt.Errorf("PR 编号必须大于 0")
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", escapePathSegment(owner), escapePathSegment(repo), number)
	var comments []Comment
	if err := c.do(ctx, http.MethodGet, path, nil, &comments); err != nil {
		return nil, err
	}
	return comments, nil
}

// Issue 表示 Gitee Issue 的完整信息。
// 注意：Gitee Issue 的 number 字段是字符串类型（可能包含字母前缀），与 PR 不同。
type Issue struct {
	ID      int64  `json:"id"`
	Number  string `json:"number"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	User    User   `json:"user"`
	Labels  []struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	} `json:"labels"`
	Assignee  *User `json:"assignee"`
	Milestone *struct {
		ID     int64  `json:"id"`
		Title  string `json:"title"`
		Number int64  `json:"number"`
	} `json:"milestone"`
	Comments  int64  `json:"comments"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	ClosedAt  string `json:"closed_at,omitempty"`
}

// ListIssuesInput 是查询 Issue 列表的输入参数。
// 所有字段均为可选；空值由客户端跳过，让服务端使用默认值。
//
// 参考: GET /api/v5/repos/{owner}/{repo}/issues
type ListIssuesInput struct {
	// State 是 Issue 状态：open（默认）/ closed / progressing / rejected / all。
	State string
	// Labels 是标签过滤器，逗号分隔。
	Labels string
	// Sort 是排序字段：created（默认）/ updated / comments。
	Sort string
	// Direction 是排序方向：asc / desc（默认）。
	Direction string
	// Since 是时间过滤器，返回此日期后更新的 Issue（RFC3339 格式）。
	Since string
	// Page 是页码（从 1 开始，默认 1）。
	Page int
	// PerPage 是每页数量（1-100，默认 20）。
	PerPage int
	// MilestoneNumber 是里程碑编号过滤器。
	MilestoneNumber int64
}

// ListIssues 调用 GET /repos/:owner/:repo/issues 获取 Issue 列表。
//
// 注意：Gitee v5 接口本身不支持按作者或指派人过滤，调用方如需这些过滤
// 应在拿到结果后基于 Issue.User.Login 或 Issue.Assignee.Login 自行筛选。
func (c *Client) ListIssues(ctx context.Context, owner, repo string, input *ListIssuesInput) ([]Issue, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}

	query := url.Values{}
	if input != nil {
		if input.State != "" {
			query.Set("state", input.State)
		}
		if input.Labels != "" {
			query.Set("labels", input.Labels)
		}
		if input.Sort != "" {
			query.Set("sort", input.Sort)
		}
		if input.Direction != "" {
			query.Set("direction", input.Direction)
		}
		if input.Since != "" {
			query.Set("since", input.Since)
		}
		if input.MilestoneNumber > 0 {
			query.Set("milestone", fmt.Sprintf("%d", input.MilestoneNumber))
		}
		if input.Page > 0 {
			query.Set("page", fmt.Sprintf("%d", input.Page))
		}
		if input.PerPage > 0 {
			query.Set("per_page", fmt.Sprintf("%d", input.PerPage))
		}
	}

	path := fmt.Sprintf("/repos/%s/%s/issues", escapePathSegment(owner), escapePathSegment(repo))

	var issues []Issue
	if err := c.do(ctx, http.MethodGet, path, query, &issues); err != nil {
		return nil, err
	}
	return issues, nil
}

// GetIssue 获取指定编号的 Issue 详情。
// 注意：Gitee Issue 编号是字符串类型（可能包含字母），与 PR 不同。
func (c *Client) GetIssue(ctx context.Context, owner, repo, number string) (*Issue, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if number == "" {
		return nil, fmt.Errorf("Issue 编号不能为空")
	}

	path := fmt.Sprintf("/repos/%s/%s/issues/%s", escapePathSegment(owner), escapePathSegment(repo), escapePathSegment(number))
	var issue Issue
	if err := c.do(ctx, http.MethodGet, path, nil, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// UpdateIssueState 调用 PATCH /repos/:owner/:repo/issues/:number 更新 Issue 状态。
// state 可以是 "open" 或 "closed"。
func (c *Client) UpdateIssueState(ctx context.Context, owner, repo, number, state string) (*Issue, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if number == "" {
		return nil, fmt.Errorf("Issue 编号不能为空")
	}
	if state != "open" && state != "closed" {
		return nil, fmt.Errorf("state 必须为 open 或 closed")
	}

	path := fmt.Sprintf("/repos/%s/%s/issues/%s", escapePathSegment(owner), escapePathSegment(repo), escapePathSegment(number))

	body, err := json.Marshal(map[string]string{"state": state, "repo": repo})
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	var issue Issue
	if err := c.doWithBody(ctx, http.MethodPatch, path, nil, strings.NewReader(string(body)), &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// EditIssueInput 是编辑 Issue 的输入参数。
//
// 所有字段均为指针类型，遵循 PATCH 部分更新语义：
//   - nil 表示该字段保持不变（不提交到请求体）；
//   - 非 nil（即使指向空字符串/0）表示显式更新为该值。
//
// 注意：Gitee v5 编辑 Issue 接口的指派人字段名为 assignee（单数），
// 里程碑字段名为 milestone。
type EditIssueInput struct {
	// Title 是 Issue 标题。
	Title *string
	// Body 是 Issue 描述内容。
	Body *string
	// Labels 是标签列表，逗号分隔的字符串。
	Labels *string
	// Assignee 是指派人登录名（单个）。
	Assignee *string
	// MilestoneNumber 是里程碑编号。
	MilestoneNumber *int64
}

// isEmpty 判断是否没有任何待更新字段。
func (in *EditIssueInput) isEmpty() bool {
	if in == nil {
		return true
	}
	return in.Title == nil && in.Body == nil && in.Labels == nil &&
		in.Assignee == nil && in.MilestoneNumber == nil
}

// EditIssue 调用 PATCH /repos/:owner/:repo/issues/:number 编辑 Issue 的标题/正文/标签/指派人/里程碑。
// 仅提交非 nil 字段，符合部分更新语义。
//
// 注意：与 UpdateIssueState 保持一致，repo 同时出现在 path 与请求体的 repo 字段中，
// 符合 Gitee v5 编辑 Issue 接口约定。
func (c *Client) EditIssue(ctx context.Context, owner, repo, number string, input *EditIssueInput) (*Issue, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if number == "" {
		return nil, fmt.Errorf("Issue 编号不能为空")
	}
	if input.isEmpty() {
		return nil, fmt.Errorf("至少需要指定一个待修改字段")
	}

	path := fmt.Sprintf("/repos/%s/%s/issues/%s", escapePathSegment(owner), escapePathSegment(repo), escapePathSegment(number))

	payload := map[string]interface{}{"repo": repo}
	if input.Title != nil {
		payload["title"] = *input.Title
	}
	if input.Body != nil {
		payload["body"] = *input.Body
	}
	if input.Labels != nil {
		payload["labels"] = *input.Labels
	}
	if input.Assignee != nil {
		payload["assignee"] = *input.Assignee
	}
	if input.MilestoneNumber != nil {
		payload["milestone"] = *input.MilestoneNumber
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	var issue Issue
	if err := c.doWithBody(ctx, http.MethodPatch, path, nil, strings.NewReader(string(body)), &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// ListIssueComments 调用 GET /repos/:owner/:repo/issues/:number/comments 获取 Issue 评论列表。
func (c *Client) ListIssueComments(ctx context.Context, owner, repo, number string) ([]Comment, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if number == "" {
		return nil, fmt.Errorf("Issue 编号不能为空")
	}

	path := fmt.Sprintf("/repos/%s/%s/issues/%s/comments", escapePathSegment(owner), escapePathSegment(repo), escapePathSegment(number))
	var comments []Comment
	if err := c.do(ctx, http.MethodGet, path, nil, &comments); err != nil {
		return nil, err
	}
	return comments, nil
}

// CreateIssueInput 是创建 Issue 的输入参数。
type CreateIssueInput struct {
	// Title 是 Issue 的标题（必填）。
	Title string `json:"title"`
	// Body 是 Issue 的描述内容（可选）。
	Body string `json:"body,omitempty"`
	// Labels 是标签列表（可选），逗号分隔的字符串。
	Labels string `json:"labels,omitempty"`
	// Assignees 是指派人列表（可选），逗号分隔的用户名。
	Assignees string `json:"assignees,omitempty"`
	// MilestoneNumber 是里程碑编号（可选）。
	MilestoneNumber int64 `json:"milestone,omitempty"`
}

// CreateIssue 调用 POST /repos/:owner/:repo/issues 创建 Issue。
func (c *Client) CreateIssue(ctx context.Context, owner, repo string, input *CreateIssueInput) (*Issue, error) {
	if input == nil {
		return nil, fmt.Errorf("input 不能为空")
	}
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if input.Title == "" {
		return nil, fmt.Errorf("title 是必填参数")
	}

	path := fmt.Sprintf("/repos/%s/%s/issues", escapePathSegment(owner), escapePathSegment(repo))

	// 构造请求体
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	// 构造请求
	query := url.Values{}
	if c.token != "" {
		query.Set("access_token", c.token)
	}

	fullURL := c.baseURL + path
	if encoded := query.Encode(); encoded != "" {
		fullURL += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    parseErrorMessage(respBody),
		}
	}

	var issue Issue
	if err := json.Unmarshal(respBody, &issue); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &issue, nil
}

// GetPullRequestDiff 调用 GET /repos/:owner/:repo/pulls/:number.diff 获取 PR 的 unified diff 文本。
// 返回原始 diff 字符串（非 JSON），调用方可直接输出或解析文件名。
func (c *Client) GetPullRequestDiff(ctx context.Context, owner, repo string, number int64) (string, error) {
	if owner == "" || repo == "" {
		return "", fmt.Errorf("owner 和 repo 不能为空")
	}
	if number <= 0 {
		return "", fmt.Errorf("PR 编号必须大于 0")
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%d.diff", escapePathSegment(owner), escapePathSegment(repo), number)

	query := url.Values{}
	if c.token != "" {
		query.Set("access_token", c.token)
	}

	fullURL := c.baseURL + path
	if encoded := query.Encode(); encoded != "" {
		fullURL += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return "", fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Accept", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &APIError{
			StatusCode: resp.StatusCode,
			Message:    parseErrorMessage(body),
		}
	}

	return string(body), nil
}

// CIStatus 表示 Gitee commit 的 CI 状态（来自 GET /repos/:owner/:repo/commits/:ref/statuses）。
// 参考: https://gitee.com/api/v5/swagger#/getV5ReposOwnerRepoCommitsRefStatuses
type CIStatus struct {
	// ID 是状态记录的唯一标识。
	ID int64 `json:"id"`
	// State 是状态值：pending / running / success / failed / error / canceled。
	State string `json:"state"`
	// Description 是状态的简短描述，例如 "Build passed"。
	Description string `json:"description"`
	// TargetURL 是外部 CI 系统的详细链接。
	TargetURL string `json:"target_url"`
	// Context 是状态提供者的名称，例如 "jenkins"、"travis-ci"。
	Context string `json:"context"`
	// CreatedAt 是状态创建时间，RFC3339 格式。
	CreatedAt string `json:"created_at"`
	// UpdatedAt 是状态更新时间，RFC3339 格式。
	UpdatedAt string `json:"updated_at"`
	// Creator 是创建此状态的用户。
	Creator User `json:"creator"`
}

// CombinedStatus 表示 Gitee commit 的聚合状态（来自 GET /repos/:owner/:repo/commits/:ref/status）。
// 参考: https://gitee.com/api/v5/swagger#/getV5ReposOwnerRepoCommitsRefStatus
type CombinedStatus struct {
	// State 是聚合后的整体状态：pending / running / success / failed / error / canceled。
	State string `json:"state"`
	// CommitURL 是 commit 的 API URL。
	CommitURL string `json:"commit_url"`
	// Repository 是所属仓库的精简信息。
	Repository *Repository `json:"repository"`
	// Statuses 是各上下文的详细状态列表。
	Statuses []CIStatus `json:"statuses"`
	// TotalCount 是状态总数。
	TotalCount int `json:"total_count"`
}

// ListCIStatusesInput 是查询 CI 状态列表的输入参数。
type ListCIStatusesInput struct {
	// Page 是页码（从 1 开始，默认 1）。
	Page int
	// PerPage 是每页数量（1-100，默认 20）。
	PerPage int
}

// ListCIStatuses 调用 GET /repos/:owner/:repo/commits/:ref/statuses 获取指定 ref 的 CI 状态列表。
// ref 可以是 commit SHA、分支名或 tag 名。
func (c *Client) ListCIStatuses(ctx context.Context, owner, repo, ref string, input *ListCIStatusesInput) ([]CIStatus, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if ref == "" {
		return nil, fmt.Errorf("ref 不能为空")
	}

	query := url.Values{}
	if input != nil {
		if input.Page > 0 {
			query.Set("page", fmt.Sprintf("%d", input.Page))
		}
		if input.PerPage > 0 {
			query.Set("per_page", fmt.Sprintf("%d", input.PerPage))
		}
	}

	path := fmt.Sprintf("/repos/%s/%s/commits/%s/statuses", escapePathSegment(owner), escapePathSegment(repo), escapePathSegment(ref))
	var statuses []CIStatus
	if err := c.do(ctx, http.MethodGet, path, query, &statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

// GetCombinedStatus 调用 GET /repos/:owner/:repo/commits/:ref/status 获取聚合 CI 状态。
// ref 可以是 commit SHA、分支名或 tag 名。
func (c *Client) GetCombinedStatus(ctx context.Context, owner, repo, ref string) (*CombinedStatus, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if ref == "" {
		return nil, fmt.Errorf("ref 不能为空")
	}

	path := fmt.Sprintf("/repos/%s/%s/commits/%s/status", escapePathSegment(owner), escapePathSegment(repo), escapePathSegment(ref))
	var combined CombinedStatus
	if err := c.do(ctx, http.MethodGet, path, nil, &combined); err != nil {
		return nil, err
	}
	return &combined, nil
}

// MergePullRequestInput 是合并 Pull Request 的输入参数。
//
// 注意：Gitee v5 的 PUT /repos/{owner}/{repo}/pulls/{number}/merge 接口
// 按 formData（application/x-www-form-urlencoded）接收参数，而非 JSON body。
// 官方字段命名为 merge_method / title / description / prune_source_branch。
type MergePullRequestInput struct {
	// MergeMethod 是合并方式：merge（默认）/ squash / rebase，对应 form 字段 merge_method。
	MergeMethod string
	// Title 是自定义合并提交标题（可选），对应 form 字段 title。
	Title string
	// Description 是自定义合并提交信息（可选），对应 form 字段 description。
	Description string
	// PruneSourceBranch 是否在合并后删除源分支（可选），对应 form 字段 prune_source_branch。
	PruneSourceBranch bool
}

// MergePullRequest 调用 PUT /repos/:owner/:repo/pulls/:number/merge 合并 Pull Request。
// 参数以 application/x-www-form-urlencoded 形式提交，符合 Gitee v5 contract。
func (c *Client) MergePullRequest(ctx context.Context, owner, repo string, number int64, input *MergePullRequestInput) error {
	if owner == "" || repo == "" {
		return fmt.Errorf("owner 和 repo 不能为空")
	}
	if number <= 0 {
		return fmt.Errorf("PR 编号必须大于 0")
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", escapePathSegment(owner), escapePathSegment(repo), number)

	// 构造 form 表单：access_token 与各合并参数均以 form 字段提交。
	form := url.Values{}
	if c.token != "" {
		form.Set("access_token", c.token)
	}
	if input != nil {
		if input.MergeMethod != "" {
			form.Set("merge_method", input.MergeMethod)
		}
		if input.Title != "" {
			form.Set("title", input.Title)
		}
		if input.Description != "" {
			form.Set("description", input.Description)
		}
		if input.PruneSourceBranch {
			form.Set("prune_source_branch", "true")
		}
	}

	fullURL := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fullURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    parseErrorMessage(respBody),
		}
	}

	return nil
}

// CreatePullRequestCommentInput 是创建 PR 评论的输入参数。
type CreatePullRequestCommentInput struct {
	// Body 是评论内容（必填）。
	Body string `json:"body"`
}

// CreatePullRequestComment 调用 POST /repos/:owner/:repo/pulls/:number/comments 创建 PR 评论。
func (c *Client) CreatePullRequestComment(ctx context.Context, owner, repo string, number int64, input *CreatePullRequestCommentInput) (*Comment, error) {
	if input == nil {
		return nil, fmt.Errorf("input 不能为空")
	}
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if number <= 0 {
		return nil, fmt.Errorf("PR 编号必须大于 0")
	}
	if input.Body == "" {
		return nil, fmt.Errorf("评论内容不能为空")
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", escapePathSegment(owner), escapePathSegment(repo), number)

	// 构造请求体
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	// 构造请求
	query := url.Values{}
	if c.token != "" {
		query.Set("access_token", c.token)
	}

	fullURL := c.baseURL + path
	if encoded := query.Encode(); encoded != "" {
		fullURL += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    parseErrorMessage(respBody),
		}
	}

	var comment Comment
	if err := json.Unmarshal(respBody, &comment); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &comment, nil
}

// ReviewPullRequestInput 是处理 Pull Request 审查（审查通过）的输入参数。
//
// 注意：Gitee v5 的 POST /repos/{owner}/{repo}/pulls/{number}/review 接口
// 按 formData（application/x-www-form-urlencoded）接收参数，而非 JSON body。
// 官方仅定义 force 字段，用于在开启分支保护时强制通过审查。
type ReviewPullRequestInput struct {
	// Force 是否强制通过，忽略分支保护设置中的审查/测试规则限制（可选），
	// 对应 form 字段 force。
	Force bool
}

// ReviewPullRequest 调用 POST /repos/:owner/:repo/pulls/:number/review 处理 PR 审查（审查通过）。
// 参数以 application/x-www-form-urlencoded 形式提交，符合 Gitee v5 contract。
func (c *Client) ReviewPullRequest(ctx context.Context, owner, repo string, number int64, input *ReviewPullRequestInput) error {
	if owner == "" || repo == "" {
		return fmt.Errorf("owner 和 repo 不能为空")
	}
	if number <= 0 {
		return fmt.Errorf("PR 编号必须大于 0")
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/review", escapePathSegment(owner), escapePathSegment(repo), number)

	// 构造 form 表单：access_token 与 force 均以 form 字段提交。
	form := url.Values{}
	if c.token != "" {
		form.Set("access_token", c.token)
	}
	if input != nil && input.Force {
		form.Set("force", "true")
	}

	fullURL := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    parseErrorMessage(respBody),
		}
	}

	return nil
}

// CreateIssueCommentInput 是创建 Issue 评论的输入参数。
type CreateIssueCommentInput struct {
	// Body 是评论内容（必填）。
	Body string `json:"body"`
}

// CreateIssueComment 调用 POST /repos/:owner/:repo/issues/:number/comments 创建 Issue 评论。
func (c *Client) CreateIssueComment(ctx context.Context, owner, repo, number string, input *CreateIssueCommentInput) (*Comment, error) {
	if input == nil {
		return nil, fmt.Errorf("input 不能为空")
	}
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner 和 repo 不能为空")
	}
	if number == "" {
		return nil, fmt.Errorf("Issue 编号不能为空")
	}
	if input.Body == "" {
		return nil, fmt.Errorf("评论内容不能为空")
	}

	path := fmt.Sprintf("/repos/%s/%s/issues/%s/comments", escapePathSegment(owner), escapePathSegment(repo), escapePathSegment(number))

	// 构造请求体
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	// 构造请求
	query := url.Values{}
	if c.token != "" {
		query.Set("access_token", c.token)
	}

	fullURL := c.baseURL + path
	if encoded := query.Encode(); encoded != "" {
		fullURL += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    parseErrorMessage(respBody),
		}
	}

	var comment Comment
	if err := json.Unmarshal(respBody, &comment); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &comment, nil
}
