package vcd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// EdgeGateway đại diện cho một edge gateway trong VCD
type EdgeGateway struct {
	ID          string // UUID thuần (không có urn prefix)
	Name        string
	GatewayType string // NSXV_BACKED | NSXT_BACKED
	Status      string
	OrgName     string
	VdcName     string
	Href        string
}

// queryResultRecord là record trả về từ VCD Query API (field names lấy từ response thực tế)
type queryResultRecord struct {
	Name        string `json:"name"`
	Href        string `json:"href"`
	GatewayType string `json:"edgeGatewayType"` // NSXV_BACKED | NSXT_BACKED
	Status      string `json:"gatewayStatus"`   // READY | FAILED | etc.
	OrgName     string `json:"orgName"`
	VdcName     string `json:"orgVdcName"`       // orgVdcName chứa tên VDC
}

// queryResult là kết quả từ VCD Query API (JSON format)
type queryResult struct {
	Total             float64             `json:"total"`
	PageCount         float64             `json:"pageCount"`
	Page              float64             `json:"page"`
	PageSize          float64             `json:"pageSize"`
	EdgeGatewayRecord []queryResultRecord `json:"record"`
}

// ListEdgeGateways trả về tất cả edge gateways mà user có quyền thấy
// Dùng VCD Query API: GET /api/query?type=edgeGateway
func (c *Client) ListEdgeGateways() ([]EdgeGateway, error) {
	var allEdges []EdgeGateway
	page := 1
	pageSize := 128

	for {
		params := url.Values{}
		params.Set("type", "edgeGateway")
		params.Set("format", "records")
		params.Set("page", fmt.Sprintf("%d", page))
		params.Set("pageSize", fmt.Sprintf("%d", pageSize))

		path := "/api/query?" + params.Encode()

		req, err := c.NewRequest(http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create list edge gateways request: %w", err)
		}
		// VCD Query API yêu cầu wildcard accept header
		req.Header.Set("Accept", fmt.Sprintf("application/*+json;version=%s", c.apiVersion))

		resp, err := c.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list edge gateways request failed: %w", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list edge gateways failed (HTTP %d): %s", resp.StatusCode, string(body))
		}

		var result queryResult
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse edge gateway list: %w\nBody: %s", err, string(body))
		}

		for _, r := range result.EdgeGatewayRecord {
			eg := EdgeGateway{
				ID:          extractID(r.Href),
				Name:        r.Name,
				GatewayType: r.GatewayType,
				Status:      r.Status,
				OrgName:     r.OrgName,
				VdcName:     r.VdcName,
				Href:        r.Href,
			}
			allEdges = append(allEdges, eg)
		}

		// Kiểm tra còn trang tiếp theo không (dùng result.Total thay vì result.PageCount)
		if len(allEdges) >= int(result.Total) || len(result.EdgeGatewayRecord) == 0 {
			break
		}
		page++
	}

	return allEdges, nil
}

// FindEdgeGatewayByName tìm edge gateway theo tên (case-insensitive)
// Trả về error nếu không tìm thấy hoặc có nhiều hơn 1 kết quả
func (c *Client) FindEdgeGatewayByName(name string) (*EdgeGateway, error) {
	edges, err := c.ListEdgeGateways()
	if err != nil {
		return nil, err
	}

	var matches []EdgeGateway
	for _, eg := range edges {
		if strings.EqualFold(eg.Name, name) {
			matches = append(matches, eg)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("edge gateway %q not found", name)
	case 1:
		return &matches[0], nil
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = fmt.Sprintf("%s (org: %s, vdc: %s)", m.Name, m.OrgName, m.VdcName)
		}
		return nil, fmt.Errorf("found %d edge gateways with name %q:\n  %s",
			len(matches), name, strings.Join(names, "\n  "))
	}
}

// FindSourceEdgeGateway tìm edge gateway nguồn (phải là NSX-V backed)
func (c *Client) FindSourceEdgeGateway(name string) (*EdgeGateway, error) {
	edges, err := c.ListEdgeGateways()
	if err != nil {
		return nil, err
	}

	var matches []EdgeGateway
	var otherTypes []EdgeGateway

	for _, eg := range edges {
		if strings.EqualFold(eg.Name, name) {
			if eg.GatewayType == "NSXV_BACKED" {
				matches = append(matches, eg)
			} else {
				otherTypes = append(otherTypes, eg)
			}
		}
	}

	switch len(matches) {
	case 0:
		if len(otherTypes) > 0 {
			return nil, fmt.Errorf("source edge gateway %q found but it is backed by NSX-T (must be NSX-V)", name)
		}
		return nil, fmt.Errorf("source edge gateway %q not found", name)
	case 1:
		return &matches[0], nil
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = fmt.Sprintf("%s (org: %s, vdc: %s, type: NSX-V)", m.Name, m.OrgName, m.VdcName)
		}
		return nil, fmt.Errorf("found multiple NSX-V source edge gateways with name %q:\n  %s",
			name, strings.Join(names, "\n  "))
	}
}

// FindDestinationEdgeGateway tìm edge gateway đích (phải là NSX-T backed)
func (c *Client) FindDestinationEdgeGateway(name string) (*EdgeGateway, error) {
	edges, err := c.ListEdgeGateways()
	if err != nil {
		return nil, err
	}

	var matches []EdgeGateway
	var otherTypes []EdgeGateway

	for _, eg := range edges {
		if strings.EqualFold(eg.Name, name) {
			if eg.GatewayType == "NSXT_BACKED" {
				matches = append(matches, eg)
			} else {
				otherTypes = append(otherTypes, eg)
			}
		}
	}

	switch len(matches) {
	case 0:
		if len(otherTypes) > 0 {
			return nil, fmt.Errorf("destination edge gateway %q found but it is backed by NSX-V (must be NSX-T)", name)
		}
		return nil, fmt.Errorf("destination edge gateway %q not found", name)
	case 1:
		return &matches[0], nil
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = fmt.Sprintf("%s (org: %s, vdc: %s, type: NSX-T)", m.Name, m.OrgName, m.VdcName)
		}
		return nil, fmt.Errorf("found multiple NSX-T destination edge gateways with name %q:\n  %s",
			name, strings.Join(names, "\n  "))
	}
}

// extractID lấy UUID từ href VCD
// Ví dụ: https://host/api/admin/edgeGateway/011430c1-e8f7-467e-a210-43be1c3922f2
//
//	→ 011430c1-e8f7-467e-a210-43be1c3922f2
func extractID(href string) string {
	if href == "" {
		return ""
	}
	parts := strings.Split(href, "/")
	return parts[len(parts)-1]
}

// EntityReference đại diện cho một tham chiếu đối tượng trong CloudAPI
type EntityReference struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// GetEdgeOwnerAndOrgRef fetches the edge gateway from CloudAPI and returns the Owner ID URN and Org URN
func (c *Client) GetEdgeOwnerAndOrgRef(edgeURN string) (string, *EntityReference, error) {
	path := fmt.Sprintf("/cloudapi/1.0.0/edgeGateways/%s", edgeURN)

	req, err := c.NewCloudAPIRequest(http.MethodGet, path, nil)
	if err != nil {
		return "", nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("failed to fetch edge details: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("get edge details failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var eg struct {
		OwnerRef *EntityReference `json:"ownerRef"`
		OrgVdc   *EntityReference `json:"orgVdc"`
		OrgRef   *EntityReference `json:"orgRef"`
	}
	if err := json.Unmarshal(body, &eg); err != nil {
		return "", nil, fmt.Errorf("failed to parse edge details: %w", err)
	}

	var ownerURN string
	if eg.OwnerRef != nil && eg.OwnerRef.ID != "" {
		ownerURN = eg.OwnerRef.ID
	} else if eg.OrgVdc != nil && eg.OrgVdc.ID != "" {
		ownerURN = eg.OrgVdc.ID
	}

	if ownerURN == "" {
		return "", nil, fmt.Errorf("could not find ownerRef or orgVdc reference for edge %s", edgeURN)
	}
	if eg.OrgRef == nil || eg.OrgRef.ID == "" {
		return "", nil, fmt.Errorf("could not find orgRef reference for edge %s", edgeURN)
	}

	return ownerURN, eg.OrgRef, nil
}
