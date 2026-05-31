package vcd

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"v2t-replicate-edge/internal/config"
)

const (
	// VCD API version - dùng cho các API legacy (như auth, /api/admin/edgeGateway)
	apiVersion = "37.2"

	// CloudAPI version - dùng cho các endpoint /cloudapi/1.0.0/ (như NAT rules)
	cloudApiVersion = "38.0"

	// CloudAPI v2 version - dùng cho các endpoint /cloudapi/2.0.0/ (như firewall rules với rawPortProtocols)
	// rawPortProtocols được thêm từ version 38.1 theo spec
	cloudApiV2Version = "38.1"

	// API headers
	headerAccept        = "Accept"
	headerAuthorization = "Authorization"
	headerContentType   = "Content-Type"

	// Response headers chứa token
	headerAccessToken   = "X-Vmware-Vcloud-Access-Token"
	headerTokenType     = "X-Vmware-Vcloud-Token-Type"
	headerVcloudAuth    = "X-Vcloud-Authorization"
)

// Client là VCD API client đã xác thực
type Client struct {
	httpClient      *http.Client
	host            string
	accessToken     string   // Bearer JWT (System org - dùng cho /api/...)
	orgAccessToken  string   // Bearer JWT (Org context - dùng cho /cloudapi/...)
	legacyToken     string   // x-vcloud-authorization (legacy API)
	apiVersion      string
	cloudApiVersion string
	orgName         string
}

// Session chứa thông tin token trả về từ VCD
type Session struct {
	AccessToken string
	TokenType   string
	LegacyToken string // x-vcloud-authorization
}

// NewClient tạo VCD client mới và thực hiện xác thực
// VCD CloudAPI cần 2 token:
//   - System token: dùng cho admin API (/api/...)
//   - Org token:    dùng cho CloudAPI (/cloudapi/...) scoped theo org
func NewClient(cfg *config.Config) (*Client, error) {
	// Bỏ qua TLS verify cho lab environment (self-signed cert)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	c := &Client{
		httpClient:      httpClient,
		host:            strings.TrimRight(cfg.Host, "/"),
		apiVersion:      apiVersion,
		cloudApiVersion: cloudApiVersion,
	}

	// Authenticate bằng org account từ passfile (dùng cho cả /api/ và /cloudapi/)
	orgSession, err := c.authenticate(cfg)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	// Dùng cùng một token cho cả legacy API (/api/) và CloudAPI (/cloudapi/)
	c.accessToken = orgSession.AccessToken
	c.legacyToken = orgSession.LegacyToken
	c.orgAccessToken = orgSession.AccessToken
	c.orgName = cfg.Org

	return c, nil
}

// authenticate thực hiện POST /api/sessions để lấy token
// VCD dùng HTTP Basic auth với format: username@org:password
func (c *Client) authenticate(cfg *config.Config) (*Session, error) {
	url := fmt.Sprintf("%s/api/sessions", c.host)

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth request: %w", err)
	}

	// Build Basic auth: "username@org:password" -> base64
	credentials := fmt.Sprintf("%s@%s:%s", cfg.Username, cfg.Org, cfg.Password)
	encoded := base64.StdEncoding.EncodeToString([]byte(credentials))

	req.Header.Set(headerAuthorization, "Basic "+encoded)
	req.Header.Set(headerAccept, fmt.Sprintf("application/*+json;version=%s", c.apiVersion))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("authentication request failed: %w", err)
	}
	defer resp.Body.Close()

	// Đọc body để debug nếu có lỗi
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authentication failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	session := &Session{
		AccessToken: resp.Header.Get(headerAccessToken),
		TokenType:   resp.Header.Get(headerTokenType),
		LegacyToken: resp.Header.Get(headerVcloudAuth),
	}

	if session.AccessToken == "" {
		return nil, fmt.Errorf("authentication succeeded but no access token in response headers")
	}

	return session, nil
}

// NewRequest tạo HTTP request đã có auth headers (dùng cho /api/ - System scope)
func (c *Client) NewRequest(method, path string, body io.Reader) (*http.Request, error) {
	url := fmt.Sprintf("%s%s", c.host, path)

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set(headerAuthorization, "Bearer "+c.accessToken)
	req.Header.Set("X-Vcloud-Authorization", c.legacyToken)
	req.Header.Set("Accept", fmt.Sprintf("application/*+json;version=%s", c.apiVersion))

	return req, nil
}

// NewCloudAPIRequest tạo HTTP request dùng org token cho /cloudapi/ endpoints
func (c *Client) NewCloudAPIRequest(method, path string, body io.Reader) (*http.Request, error) {
	url := fmt.Sprintf("%s%s", c.host, path)

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set(headerAuthorization, "Bearer "+c.orgAccessToken)
	req.Header.Set("Accept", fmt.Sprintf("application/json;version=%s", c.cloudApiVersion))

	return req, nil
}

// NewCloudAPIV2Request tạo HTTP request dùng org token cho /cloudapi/2.0.0/ endpoints
// Cần dùng cho các API yêu cầu version 38.1+ như firewall rules với rawPortProtocols
func (c *Client) NewCloudAPIV2Request(method, path string, body io.Reader) (*http.Request, error) {
	url := fmt.Sprintf("%s%s", c.host, path)

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set(headerAuthorization, "Bearer "+c.orgAccessToken)
	req.Header.Set("Accept", fmt.Sprintf("application/json;version=%s", cloudApiV2Version))

	return req, nil
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	return c.httpClient.Do(req)
}

// AccessToken trả về Bearer JWT token hiện tại
func (c *Client) AccessToken() string {
	return c.accessToken
}

// LegacyToken trả về x-vcloud-authorization token
func (c *Client) LegacyToken() string {
	return c.legacyToken
}

// Host trả về VCD host URL
func (c *Client) Host() string {
	return c.host
}

type cloudApiTask struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Owner  *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"owner"`
	Result *struct {
		ResultReference *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"resultReference"`
	} `json:"result"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// NormalizePath checks if rawURL starts with http/https and returns only the request URI (path + query).
func (c *Client) NormalizePath(rawURL string) string {
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		u, err := url.Parse(rawURL)
		if err == nil {
			return u.RequestURI()
		}
	}
	return rawURL
}

// WaitForCloudApiTask waits for a task (CloudAPI or legacy VCD API) to complete.
// Returns the task object if successful, or an error.
func (c *Client) WaitForCloudApiTask(taskURI string, timeout, interval time.Duration) (*cloudApiTask, error) {
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	if interval == 0 {
		interval = 3 * time.Second
	}

	normalizedPath := c.NormalizePath(taskURI)
	deadline := time.Now().Add(timeout)
	attempt := 0

	for time.Now().Before(deadline) {
		attempt++
		var req *http.Request
		var err error

		// Sử dụng request phù hợp dựa trên endpoint (legacy /api/ vs /cloudapi/)
		if strings.Contains(normalizedPath, "/api/") {
			req, err = c.NewRequest(http.MethodGet, normalizedPath, nil)
		} else {
			req, err = c.NewCloudAPIRequest(http.MethodGet, normalizedPath, nil)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to create task check request: %w", err)
		}

		resp, err := c.Do(req)
		if err != nil {
			// Lỗi kết nối tạm thời, thử lại sau interval
			time.Sleep(interval)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("task status check failed with HTTP %d: %s", resp.StatusCode, string(body))
		}

		var task cloudApiTask
		if err := json.Unmarshal(body, &task); err != nil {
			return nil, fmt.Errorf("failed to parse task status JSON: %w (body: %s)", err, string(body))
		}

		statusUpper := strings.ToUpper(task.Status)
		if attempt == 1 || attempt%5 == 0 || statusUpper == "SUCCESS" || statusUpper == "ERROR" || statusUpper == "ABORTED" {
			fmt.Printf("  [wait] Task %s status: %s (attempt %d)\n", taskURI, task.Status, attempt)
		}

		switch statusUpper {
		case "SUCCESS":
			return &task, nil
		case "ERROR", "ABORTED":
			errMsg := "task failed"
			if task.Error != nil && task.Error.Message != "" {
				errMsg = task.Error.Message
			}
			return nil, fmt.Errorf("task failed (status: %s): %s", task.Status, errMsg)
		default:
			// RUNNING, QUEUED, PENDING, etc. - tiếp tục chờ
			time.Sleep(interval)
		}
	}

	return nil, fmt.Errorf("timeout waiting for task %s to complete", taskURI)
}
