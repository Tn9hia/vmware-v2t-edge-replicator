package vcd

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ============================================================
// NSX-V NAT structures (đọc từ /api/admin/edgeGateway/{id})
// ============================================================

type nsxvEdgeGateway struct {
	XMLName       xml.Name          `xml:"EdgeGateway"`
	Configuration nsxvConfiguration `xml:"Configuration"`
}

type nsxvConfiguration struct {
	ServiceConfig nsxvServiceConfig `xml:"EdgeGatewayServiceConfiguration"`
}

type nsxvServiceConfig struct {
	NatService nsxvNatService `xml:"NatService"`
}

type nsxvNatService struct {
	IsEnabled bool         `xml:"IsEnabled"`
	NatRules  []NsxvNatRule `xml:"NatRule"`
}

// NsxvNatRule là NAT rule từ NSX-V edge (legacy XML API)
type NsxvNatRule struct {
	ID          string          `xml:"Id"`
	RuleType    string          `xml:"RuleType"`     // SNAT | DNAT
	Description string          `xml:"Description"`
	IsEnabled   bool            `xml:"IsEnabled"`
	GatewayNat  NsxvGatewayNat  `xml:"GatewayNatRule"`
}

type NsxvGatewayNat struct {
	OriginalIP    string `xml:"OriginalIp"`
	OriginalPort  string `xml:"OriginalPort"`
	TranslatedIP  string `xml:"TranslatedIp"`
	TranslatedPort string `xml:"TranslatedPort"`
	Protocol      string `xml:"Protocol"`
}

// ============================================================
// NSX-T NAT structures (CloudAPI /cloudapi/1.0.0/...)
// ============================================================

// AppPortProfile là application port profile trong NSX-T
type AppPortProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AppPortEntry là một port definition trong profile
type AppPortEntry struct {
	Name             string   `json:"name,omitempty"`
	Protocol         string   `json:"protocol"`
	DestinationPorts []string `json:"destinationPorts"`
}

// NsxtNatRule là NAT rule cho NSX-T edge (CloudAPI)
type NsxtNatRule struct {
	ID                   string          `json:"id,omitempty"`
	Name                 string          `json:"name"`
	Description          string          `json:"description,omitempty"`
	Enabled              bool            `json:"enabled"`
	Type                 string          `json:"type"`              // SNAT | DNAT | NO_SNAT | NO_DNAT
	ExternalAddresses    string          `json:"externalAddresses"` // Public IP
	InternalAddresses    string          `json:"internalAddresses,omitempty"` // Private IP
	DnatExternalPort     string          `json:"dnatExternalPort,omitempty"`
	ApplicationPortProfile *AppPortProfile `json:"applicationPortProfile"`
	Priority             int             `json:"priority,omitempty"`
	Logging              bool            `json:"logging"`
	FirewallMatch        string          `json:"firewallMatch,omitempty"` // MATCH_INTERNAL_ADDRESS
}

type nsxtNatRulesResponse struct {
	ResultTotal int           `json:"resultTotal"`
	Values      []NsxtNatRule `json:"values"`
}

type appPortProfilesResponse struct {
	ResultTotal int              `json:"resultTotal"`
	PageCount   int              `json:"pageCount"`
	Page        int              `json:"page"`
	PageSize    int              `json:"pageSize"`
	Values      []AppPortProfileFull `json:"values"`
}

type AppPortProfileFull struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Scope            string         `json:"scope"`
	ApplicationPorts []AppPortEntry `json:"applicationPorts"`
	UsableForNAT     bool           `json:"usableForNAT"`
}

// ============================================================
// GetNsxvNatRules đọc NAT rules từ NSX-V edge (XML API)
// Chỉ trả về rules có RuleType = "User" (bỏ qua internal_high, etc.)
// ============================================================

func (c *Client) GetNsxvNatRules(edgeID string) ([]NsxvNatRule, error) {
	path := fmt.Sprintf("/api/admin/edgeGateway/%s", edgeID)

	req, err := c.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create get edge gateway request: %w", err)
	}
	req.Header.Set("Accept", fmt.Sprintf("application/*+xml;version=%s", c.apiVersion))

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get edge gateway request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get edge gateway failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var edge nsxvEdgeGateway
	if err := xml.Unmarshal(body, &edge); err != nil {
		return nil, fmt.Errorf("failed to parse edge gateway XML: %w", err)
	}

	var userRules []NsxvNatRule
	for _, rule := range edge.Configuration.ServiceConfig.NatService.NatRules {
		// Admin API trả về SNAT/DNAT trực tiếp - lấy tất cả
		userRules = append(userRules, rule)
	}
	return userRules, nil
}

// ============================================================
// Application Port Profile management
// ============================================================

// ListAppPortProfiles lấy toàn bộ application port profiles có usableForNAT=true
func (c *Client) ListAppPortProfiles() ([]AppPortProfileFull, error) {
	var all []AppPortProfileFull
	page := 1
	pageSize := 32

	for {
		if page == 1 || page%10 == 0 {
			fmt.Printf("  Loading app port profiles (page %d)...\n", page)
		}
		path := fmt.Sprintf("/cloudapi/1.0.0/applicationPortProfiles?pageSize=%d&page=%d", pageSize, page)

		req, err := c.NewCloudAPIRequest(http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list application port profiles failed: %w", err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list application port profiles (HTTP %d): %s", resp.StatusCode, string(body))
		}

		var result appPortProfilesResponse
		if err := json.Unmarshal(body, &result); err != nil {
			snippet := string(body)
			if len(snippet) > 500 {
				snippet = snippet[:500]
			}
			return nil, fmt.Errorf("failed to parse app port profiles: %w (HTTP status %d, body: %q)", err, resp.StatusCode, snippet)
		}

		for _, p := range result.Values {
			all = append(all, p)
		}

		if page >= result.PageCount || len(result.Values) == 0 {
			break
		}
		page++
	}
	return all, nil
}

// FindOrCreateAppPortProfile tìm application port profile theo protocol+port.
// Nếu chưa có thì tạo mới (org-scoped custom profile).
// Các trường hợp đặc biệt:
//   - protocol=icmp → trả về ICMPv4-ALL pre-defined profile
//   - port=any hoặc rỗng → trả về nil (không cần app profile)
//
// Lưu ý: NSX-T NAT yêu cầu profile có đúng 1 application port entry và tối đa 1 destination port.
// Vì vậy, chỉ trả về profile hiện có nếu nó đáp ứng điều kiện đó. Profile hệ thống có nhiều
// port (ví dụ "Update Manager" chứa 80+443) sẽ bị bỏ qua và thay bằng custom profile.
func (c *Client) FindOrCreateAppPortProfile(
	profiles []AppPortProfileFull,
	protocol, port string,
	ruleName string,
	contextEntityID string,
	orgRef *EntityReference,
) (*AppPortProfile, error) {
	proto := strings.ToLower(strings.TrimSpace(protocol))
	portStr := strings.ToLower(strings.TrimSpace(port))

	// Protocol "any" hoặc không có proto → không dùng app profile
	if proto == "any" || proto == "" {
		return nil, nil
	}

	// ICMP → tìm ICMPv4-ALL trong danh sách profiles (không hardcode URN vì khác nhau giữa các VCD env)
	if proto == "icmp" {
		for _, p := range profiles {
			if strings.EqualFold(p.Name, "ICMPv4-ALL") {
				return &AppPortProfile{ID: p.ID, Name: p.Name}, nil
			}
		}
		// Không tìm thấy sẵn → tạo mới ICMP profile
		created, err := c.createAppPortProfile("ICMPv4-ALL", "ICMPv4", "any", contextEntityID, orgRef)
		if err != nil {
			return nil, fmt.Errorf("failed to create ICMPv4-ALL app port profile: %w", err)
		}
		return created, nil
	}

	// Port "any" hoặc rỗng → không cần app profile (ngoại trừ ICMP đã xử lý ở trên)
	if portStr == "any" || portStr == "" {
		return nil, nil
	}

	// Chuẩn hoá protocol để match với NSX-T
	nsxtProto := strings.ToUpper(proto) // TCP, UDP, etc.

	// Tìm trong danh sách profiles hiện có.
	// Chỉ chấp nhận profile có đúng 1 application port entry VÀ đúng 1 destination port
	// để đảm bảo hợp lệ cho cả NAT và Firewall (NSX-T NAT không cho phép profile nhiều port).
	for _, p := range profiles {
		if len(p.ApplicationPorts) != 1 {
			// Profile có nhiều application port entry → bỏ qua
			continue
		}
		ap := p.ApplicationPorts[0]
		if !strings.EqualFold(ap.Protocol, nsxtProto) {
			continue
		}
		if len(ap.DestinationPorts) != 1 {
			// Profile có nhiều destination port → bỏ qua (không hợp lệ cho NAT)
			continue
		}
		if ap.DestinationPorts[0] == portStr {
			return &AppPortProfile{ID: p.ID, Name: p.Name}, nil
		}
	}

	// Chưa có → tạo custom profile mới với format: <port>-<protocol> (ví dụ: 2259-tcp)
	profileName := fmt.Sprintf("%s-%s", portStr, strings.ToLower(nsxtProto))

	created, err := c.createAppPortProfile(profileName, nsxtProto, portStr, contextEntityID, orgRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create app port profile %q: %w", profileName, err)
	}
	return created, nil
}

// createAppPortProfile tạo custom application port profile mới trong org scope
func (c *Client) createAppPortProfile(name, protocol, port string, contextEntityID string, orgRef *EntityReference) (*AppPortProfile, error) {
	payload := map[string]interface{}{
		"name":            name,
		"contextEntityId": contextEntityID,
		"scope":           "TENANT",
		"orgRef":          orgRef,
		"applicationPorts": []map[string]interface{}{
			{
				"protocol":         protocol,
				"destinationPorts": []string{port},
			},
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := c.NewCloudAPIRequest(http.MethodPost, "/cloudapi/1.0.0/applicationPortProfiles", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", fmt.Sprintf("application/json;version=%s", c.cloudApiVersion))

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create app port profile request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusAccepted {
		location := resp.Header.Get("Location")
		if location == "" {
			return nil, fmt.Errorf("create app port profile accepted (202) but Location header is missing")
		}
		// Đợi task hoàn thành
		task, err := c.WaitForCloudApiTask(location, 120*time.Second, 3*time.Second)
		if err != nil {
			return nil, fmt.Errorf("failed waiting for app port profile creation task: %w", err)
		}
		// Trích xuất ID và Name từ kết quả task
		if task.Result != nil && task.Result.ResultReference != nil && task.Result.ResultReference.ID != "" {
			return &AppPortProfile{
				ID:   task.Result.ResultReference.ID,
				Name: task.Result.ResultReference.Name,
			}, nil
		}
		// Thử lấy từ Owner (legacy task)
		if task.Owner != nil && task.Owner.ID != "" {
			return &AppPortProfile{
				ID:   task.Owner.ID,
				Name: task.Owner.Name,
			}, nil
		}
		// Fallback: tìm lại profile bằng name
		existing, err := c.ListAppPortProfiles()
		if err != nil {
			return nil, fmt.Errorf("failed to list app port profiles for fallback search: %w", err)
		}
		for _, p := range existing {
			if strings.EqualFold(p.Name, name) {
				return &AppPortProfile{ID: p.ID, Name: p.Name}, nil
			}
		}
		return nil, fmt.Errorf("app port profile task succeeded, but could not find the created profile with name %q", name)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create app port profile failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result AppPortProfileFull
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse created app port profile: %w", err)
	}

	return &AppPortProfile{ID: result.ID, Name: result.Name}, nil
}

// ============================================================
// NSX-T NAT rule CRUD
// ============================================================

// GetNsxtNatRules lấy danh sách NAT rules từ NSX-T edge
func (c *Client) GetNsxtNatRules(edgeURN string) ([]NsxtNatRule, error) {
	path := fmt.Sprintf("/cloudapi/1.0.0/edgeGateways/%s/nat/rules", edgeURN)

	req, err := c.NewCloudAPIRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get NSX-T NAT rules failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get NSX-T NAT rules (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result nsxtNatRulesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse NSX-T NAT rules: %w", err)
	}
	return result.Values, nil
}

// CreateNsxtNatRule tạo một NAT rule mới trên NSX-T edge
func (c *Client) CreateNsxtNatRule(edgeURN string, rule NsxtNatRule) (*NsxtNatRule, error) {
	path := fmt.Sprintf("/cloudapi/1.0.0/edgeGateways/%s/nat/rules", edgeURN)

	bodyBytes, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}

	req, err := c.NewCloudAPIRequest(http.MethodPost, path, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", fmt.Sprintf("application/json;version=%s", c.cloudApiVersion))

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create NSX-T NAT rule failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("create NAT rule %q failed (HTTP %d): %s", rule.Name, resp.StatusCode, string(body))
	}

	var created NsxtNatRule
	if err := json.Unmarshal(body, &created); err != nil {
		// Một số VCD versions không trả về body sau khi tạo - không coi là lỗi
		return &rule, nil
	}
	return &created, nil
}

// ============================================================
// Edge Gateway status polling

// ============================================================

type edgeGatewayStatus struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // REALIZED | PENDING | ERROR | UNKNOWN
}

// GetEdgeStatus lấy trạng thái hiện tại của NSX-T edge gateway
func (c *Client) GetEdgeStatus(edgeURN string) (string, error) {
	path := fmt.Sprintf("/cloudapi/1.0.0/edgeGateways/%s", edgeURN)

	req, err := c.NewCloudAPIRequest(http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("get edge status failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get edge status (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var eg edgeGatewayStatus
	if err := json.Unmarshal(body, &eg); err != nil {
		return "", fmt.Errorf("failed to parse edge status: %w", err)
	}
	return eg.Status, nil
}

// WaitForEdgeRealized poll trạng thái edge gateway cho đến khi REALIZED hoặc ERROR.
// timeout: thời gian tối đa chờ (default 120s)
// interval: khoảng thời gian giữa mỗi lần poll (default 3s)
func (c *Client) WaitForEdgeRealized(edgeURN string, timeout, interval time.Duration) error {
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	if interval == 0 {
		interval = 3 * time.Second
	}

	deadline := time.Now().Add(timeout)
	attempt := 0

	for time.Now().Before(deadline) {
		attempt++
		status, err := c.GetEdgeStatus(edgeURN)
		if err != nil {
			// Lỗi tạm thời - thử lại
			time.Sleep(interval)
			continue
		}

		switch status {
		case "REALIZED":
			return nil
		case "ERROR":
			return fmt.Errorf("edge gateway entered ERROR state after update")
		default:
			// PENDING, UNKNOWN, hoặc trạng thái khác - tiếp tục chờ
			if attempt == 1 || attempt%5 == 0 {
				// In log mỗi 5 lần để tránh spam
				fmt.Printf("  [wait] Edge status: %s (attempt %d)\n", status, attempt)
			}
			time.Sleep(interval)
		}
	}

	return fmt.Errorf("timeout (%s) waiting for edge gateway to become REALIZED", timeout)
}
