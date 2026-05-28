package vcd

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// NsxvFirewallRules đại diện cho cấu trúc XML firewall từ NSX-V proxy API
type NsxvFirewallRules struct {
	XMLName xml.Name           `xml:"firewall"`
	Enabled bool               `xml:"enabled"`
	Rules   []NsxvFirewallRule `xml:"firewallRules>firewallRule"`
}

type NsxvFirewallRule struct {
	ID             string                  `xml:"id"`
	RuleTag        string                  `xml:"ruleTag"`
	Name           string                  `xml:"name"`
	RuleType       string                  `xml:"ruleType"` // user | internal_high | default_policy
	Enabled        bool                    `xml:"enabled"`
	LoggingEnabled bool                    `xml:"loggingEnabled"`
	Description    string                  `xml:"description"`
	Action         string                  `xml:"action"` // accept | deny
	Source         NsxvFirewallSourceDest  `xml:"source"`
	Destination    NsxvFirewallSourceDest  `xml:"destination"`
	Application    NsxvFirewallApplication `xml:"application"`
}

type NsxvFirewallSourceDest struct {
	Exclude           bool     `xml:"exclude"`
	VnicGroupId       string   `xml:"vnicGroupId"` // vse, vnic-0, vnic-1, etc.
	IpAddress         []string `xml:"ipAddress"`
	GroupingObjectIds []string `xml:"groupingObjectId"`
}

type NsxvFirewallApplication struct {
	Services []NsxvFirewallService `xml:"service"`
}

type NsxvFirewallService struct {
	Protocol   string   `xml:"protocol"` // tcp, udp, icmp, gre, esp, etc.
	Ports      []string `xml:"port"`     // port numbers
	SourcePort string   `xml:"sourcePort"`
}

// NsxtFirewallRule đại diện cho firewall rule trên NSX-T (CloudAPI)
type NsxtFirewallRule struct {
	ID                             string            `json:"id,omitempty"`
	Name                           string            `json:"name"`
	Description                    string            `json:"description,omitempty"`
	Enabled                        bool              `json:"enabled"`
	ActionValue                    string            `json:"actionValue"` // ALLOW | DROP | REJECT
	Direction                      string            `json:"direction,omitempty"` // IN_OUT | IN | OUT
	Logging                        bool              `json:"logging"`
	SourceFirewallGroups           []EntityReference `json:"sourceFirewallGroups,omitempty"`
	DestinationFirewallGroups      []EntityReference `json:"destinationFirewallGroups,omitempty"`
	SourceFirewallIpAddresses      []string          `json:"sourceFirewallIpAddresses,omitempty"`
	DestinationFirewallIpAddresses []string          `json:"destinationFirewallIpAddresses,omitempty"`
	ApplicationPortProfiles        []AppPortProfile  `json:"applicationPortProfiles,omitempty"`
}

type nsxtFirewallRulesResponse struct {
	UserDefinedRules []NsxtFirewallRule `json:"userDefinedRules"`
	SystemRules      []NsxtFirewallRule `json:"systemRules"`
	DefaultRules     []NsxtFirewallRule `json:"defaultRules"`
}

// NsxvGatewayInterface đại diện cho interface trong configuration của NSX-V edge
type NsxvGatewayInterface struct {
	Name                string                  `xml:"Name"`
	DisplayName         string                  `xml:"DisplayName"`
	InterfaceType       string                  `xml:"InterfaceType"` // uplink | internal
	SubnetParticipation NsxvSubnetParticipation `xml:"SubnetParticipation"`
	Network             struct {
		Href string `xml:"href,attr"`
		ID   string `xml:"id,attr"`
		Name string `xml:"name,attr"`
	} `xml:"Network"`
}

type NsxvSubnetParticipation struct {
	Gateway            string `xml:"Gateway"`
	Netmask            string `xml:"Netmask"`
	SubnetPrefixLength int    `xml:"SubnetPrefixLength"`
	IpAddress          string `xml:"IpAddress"`
}

// SubnetCIDR tính toán dải IP dạng CIDR cho interface từ IP và netmask/prefix
func (g NsxvGatewayInterface) SubnetCIDR() string {
	if g.SubnetParticipation.IpAddress == "" {
		return ""
	}
	ip := net.ParseIP(g.SubnetParticipation.IpAddress)
	if ip == nil {
		return ""
	}
	prefixLen := g.SubnetParticipation.SubnetPrefixLength
	if prefixLen <= 0 && g.SubnetParticipation.Netmask != "" {
		maskIP := net.ParseIP(g.SubnetParticipation.Netmask)
		if maskIP != nil {
			mask := net.IPMask(maskIP.To4())
			prefixLen, _ = mask.Size()
		}
	}
	if prefixLen <= 0 {
		prefixLen = 24 // fallback mặc định nếu không xác định được
	}
	mask := net.CIDRMask(prefixLen, 32)
	network := ip.Mask(mask)
	return fmt.Sprintf("%s/%d", network.String(), prefixLen)
}

type nsxvEdgeGatewayWithInterfaces struct {
	XMLName       xml.Name `xml:"EdgeGateway"`
	Configuration struct {
		GatewayInterfaces []NsxvGatewayInterface `xml:"GatewayInterfaces>GatewayInterface"`
	} `xml:"Configuration"`
}

// GetNsxvGatewayInterfaces lấy danh sách interfaces và subnets của NSX-V edge
func (c *Client) GetNsxvGatewayInterfaces(edgeID string) ([]NsxvGatewayInterface, error) {
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

	var eg nsxvEdgeGatewayWithInterfaces
	if err := xml.Unmarshal(body, &eg); err != nil {
		return nil, fmt.Errorf("failed to parse edge gateway interfaces XML: %w", err)
	}

	return eg.Configuration.GatewayInterfaces, nil
}

// GetNsxvFirewallRules đọc cấu hình firewall của NSX-V edge thông qua proxy endpoint
func (c *Client) GetNsxvFirewallRules(edgeID string) ([]NsxvFirewallRule, error) {
	path := fmt.Sprintf("/network/edges/%s/firewall/config", edgeID)

	req, err := c.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create firewall request: %w", err)
	}
	req.Header.Set("Accept", "application/xml")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get firewall config failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get firewall config failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var fw NsxvFirewallRules
	if err := xml.Unmarshal(body, &fw); err != nil {
		return nil, fmt.Errorf("failed to parse firewall config XML: %w", err)
	}

	return fw.Rules, nil
}

// GetNsxtFirewallRules lấy danh sách user-defined firewall rules trên NSX-T edge
func (c *Client) GetNsxtFirewallRules(edgeURN string) ([]NsxtFirewallRule, error) {
	path := fmt.Sprintf("/cloudapi/1.0.0/edgeGateways/%s/firewall/rules", edgeURN)

	req, err := c.NewCloudAPIRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get NSX-T firewall rules failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get NSX-T firewall rules (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result nsxtFirewallRulesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse NSX-T firewall rules: %w", err)
	}
	return result.UserDefinedRules, nil
}

// CreateNsxtFirewallRule tạo một firewall rule mới trên NSX-T edge
func (c *Client) CreateNsxtFirewallRule(edgeURN string, rule NsxtFirewallRule) (*NsxtFirewallRule, error) {
	path := fmt.Sprintf("/cloudapi/1.0.0/edgeGateways/%s/firewall/rules", edgeURN)

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
		return nil, fmt.Errorf("create NSX-T firewall rule failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("create firewall rule %q failed (HTTP %d): %s", rule.Name, resp.StatusCode, string(body))
	}

	var created NsxtFirewallRule
	if err := json.Unmarshal(body, &created); err != nil {
		// VCD may return empty body
		return &rule, nil
	}
	return &created, nil
}

type NsxvOrgVdcNetwork struct {
	XMLName       xml.Name `xml:"OrgVdcNetwork"`
	Name          string   `xml:"name,attr"`
	ID            string   `xml:"id,attr"`
	Href          string   `xml:"href,attr"`
	Configuration struct {
		IpScopes []NsxvIpScope `xml:"IpScopes>IpScope"`
	} `xml:"Configuration"`
}

type NsxvIpScope struct {
	Gateway            string `xml:"Gateway"`
	Netmask            string `xml:"Netmask"`
	SubnetPrefixLength int    `xml:"SubnetPrefixLength"`
}

func (n NsxvOrgVdcNetwork) SubnetCIDR() string {
	if len(n.Configuration.IpScopes) == 0 {
		return ""
	}
	scope := n.Configuration.IpScopes[0]
	if scope.Gateway == "" {
		return ""
	}
	ip := net.ParseIP(scope.Gateway)
	if ip == nil {
		return ""
	}
	prefixLen := scope.SubnetPrefixLength
	if prefixLen <= 0 && scope.Netmask != "" {
		maskIP := net.ParseIP(scope.Netmask)
		if maskIP != nil {
			mask := net.IPMask(maskIP.To4())
			prefixLen, _ = mask.Size()
		}
	}
	if prefixLen <= 0 {
		prefixLen = 24
	}
	mask := net.CIDRMask(prefixLen, 32)
	network := ip.Mask(mask)
	return fmt.Sprintf("%s/%d", network.String(), prefixLen)
}

type NsxvIPSet struct {
	XMLName     xml.Name `xml:"ipset"`
	Name        string   `xml:"name"`
	Description string   `xml:"description"`
	Value       string   `xml:"value"`
}

// NsxtFirewallGroup đại diện cho Firewall Group (IP Set) trên NSX-T (CloudAPI)
type NsxtFirewallGroup struct {
	ID          string          `json:"id,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	TypeValue   string          `json:"typeValue"` // IP_SET | STATIC_MEMBERS | MAC_SET | VM_CRITERIA
	OwnerRef    *EntityReference `json:"ownerRef,omitempty"`
	IPAddresses []string        `json:"ipAddresses,omitempty"`
}

type nsxtFirewallGroupsResponse struct {
	ResultTotal int                  `json:"resultTotal"`
	PageCount   int                  `json:"pageCount"`
	Page        int                  `json:"page"`
	PageSize    int                  `json:"pageSize"`
	Values      []NsxtFirewallGroup  `json:"values"`
}

// GetOrgVdcNetworkCIDR fetches the CIDR of an Org VDC network from VCD API
func (c *Client) GetOrgVdcNetworkCIDR(networkURN string) (string, error) {
	uuid := strings.TrimPrefix(networkURN, "urn:vcloud:network:")
	path := fmt.Sprintf("/api/network/%s", uuid)

	req, err := c.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create get network request: %w", err)
	}
	req.Header.Set("Accept", fmt.Sprintf("application/*+xml;version=%s", c.apiVersion))

	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("get network request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get network failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var nw NsxvOrgVdcNetwork
	if err := xml.Unmarshal(body, &nw); err != nil {
		return "", fmt.Errorf("failed to parse network XML: %w", err)
	}

	cidr := nw.SubnetCIDR()
	if cidr == "" {
		return "", fmt.Errorf("could not determine CIDR for network %s", networkURN)
	}

	return cidr, nil
}

// GetIPSetIPs fetches the list of IP addresses/subnets contained in an NSX-V IP set
func (c *Client) GetIPSetIPs(edgeID, ipsetID string) ([]string, error) {
	ipset, err := c.GetIPSetFull(edgeID, ipsetID)
	if err != nil {
		return nil, err
	}
	return splitIPSetValue(ipset.Value), nil
}

// GetIPSetFull fetches the full IPSet information (name, description, IPs) from NSX-V
func (c *Client) GetIPSetFull(edgeID, ipsetID string) (*NsxvIPSet, error) {
	// Extract the ipset-X part if input is like vdcUUID:ipset-X
	cleanID := ipsetID
	if idx := strings.LastIndex(ipsetID, ":"); idx != -1 {
		cleanID = ipsetID[idx+1:]
	}

	path := fmt.Sprintf("/network/edges/%s/api/2.0/services/ipsets/%s", edgeID, cleanID)

	req, err := c.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create ipset request: %w", err)
	}
	req.Header.Set("Accept", "application/xml")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get ipset request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get ipset failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var ipset NsxvIPSet
	if err := xml.Unmarshal(body, &ipset); err != nil {
		return nil, fmt.Errorf("failed to parse ipset XML: %w", err)
	}

	// Fallback: nếu XML không có <name>, dùng cleanID làm tên
	if ipset.Name == "" {
		ipset.Name = cleanID
	}

	return &ipset, nil
}

// splitIPSetValue tách chuỗi comma-separated IPs từ trường <value> của IPSet
func splitIPSetValue(value string) []string {
	if value == "" {
		return nil
	}
	rawIPs := strings.Split(value, ",")
	var ips []string
	for _, ip := range rawIPs {
		ip = strings.TrimSpace(ip)
		if ip != "" {
			ips = append(ips, ip)
		}
	}
	return ips
}

// ============================================================
// NSX-T Firewall Group (IP Set) management
// ============================================================

// ListNsxtFirewallGroups lấy danh sách Firewall Groups trên NSX-T theo edge gateway
// Sử dụng endpoint /firewallGroups/summaries (GET) theo CloudAPI spec.
func (c *Client) ListNsxtFirewallGroups(edgeURN string) ([]NsxtFirewallGroup, error) {
	var all []NsxtFirewallGroup
	page := 1
	pageSize := 64

	for {
		// Endpoint GET dùng /summaries; filter edgeGatewayId theo FIQL format
		path := fmt.Sprintf("/cloudapi/1.0.0/firewallGroups/summaries?pageSize=%d&page=%d&filter=edgeGatewayId==%s",
			pageSize, page, edgeURN)

		req, err := c.NewCloudAPIRequest(http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list firewall groups failed: %w", err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list firewall groups (HTTP %d): %s", resp.StatusCode, string(body))
		}

		var result nsxtFirewallGroupsResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse firewall groups response: %w", err)
		}

		all = append(all, result.Values...)

		if page >= result.PageCount || len(result.Values) == 0 {
			break
		}
		page++
	}
	return all, nil
}

// CreateNsxtFirewallGroup tạo một Firewall Group (IP_SET) mới trên NSX-T
func (c *Client) CreateNsxtFirewallGroup(group NsxtFirewallGroup) (*NsxtFirewallGroup, error) {
	bodyBytes, err := json.Marshal(group)
	if err != nil {
		return nil, err
	}

	req, err := c.NewCloudAPIRequest(http.MethodPost, "/cloudapi/1.0.0/firewallGroups",
		strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", fmt.Sprintf("application/json;version=%s", c.cloudApiVersion))

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create firewall group request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// NSX-T trả về 202 Accepted với Location header chứa task URL
	if resp.StatusCode == http.StatusAccepted {
		location := resp.Header.Get("Location")
		if location == "" {
			return nil, fmt.Errorf("create firewall group accepted (202) but Location header is missing")
		}
		task, err := c.WaitForCloudApiTask(location, 120*time.Second, 3*time.Second)
		if err != nil {
			return nil, fmt.Errorf("failed waiting for firewall group creation task: %w", err)
		}
		if task.Result != nil && task.Result.ResultReference != nil && task.Result.ResultReference.ID != "" {
			return &NsxtFirewallGroup{
				ID:   task.Result.ResultReference.ID,
				Name: task.Result.ResultReference.Name,
			}, nil
		}
		if task.Owner != nil && task.Owner.ID != "" {
			return &NsxtFirewallGroup{
				ID:   task.Owner.ID,
				Name: task.Owner.Name,
			}, nil
		}
		return nil, fmt.Errorf("firewall group task succeeded but could not find created group ID")
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create firewall group failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var created NsxtFirewallGroup
	if err := json.Unmarshal(body, &created); err != nil {
		return &group, nil
	}
	return &created, nil
}

// FindOrCreateNsxtFirewallGroup tìm hoặc tạo mới Firewall Group (IP_SET) trên NSX-T
// tương ứng với một IPSet từ NSX-V. Cache kết quả vào existingGroups để tránh tạo lại.
func (c *Client) FindOrCreateNsxtFirewallGroup(
	existingGroups []NsxtFirewallGroup,
	ipsetName string,
	ipAddresses []string,
	edgeURN string,
	ownerRef *EntityReference,
) (*NsxtFirewallGroup, []NsxtFirewallGroup, error) {
	// Tìm trong danh sách đã có (case-insensitive)
	for i := range existingGroups {
		if strings.EqualFold(existingGroups[i].Name, ipsetName) {
			return &existingGroups[i], existingGroups, nil
		}
	}

	// Chưa có → tạo mới
	newGroup := NsxtFirewallGroup{
		Name:      ipsetName,
		TypeValue: "IP_SET",
		OwnerRef:  ownerRef,
		IPAddresses: ipAddresses,
	}

	created, err := c.CreateNsxtFirewallGroup(newGroup)
	if err != nil {
		return nil, existingGroups, fmt.Errorf("failed to create NSX-T firewall group %q: %w", ipsetName, err)
	}

	// Thêm vào cache
	existingGroups = append(existingGroups, *created)
	return created, existingGroups, nil
}
