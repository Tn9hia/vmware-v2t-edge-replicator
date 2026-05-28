package migrator

import (
	"fmt"
	"log"
	"strings"
	"time"

	"v2t-replicate-edge/internal/vcd"
)

// FirewallMigrator thực hiện copy Firewall rules từ NSX-V sang NSX-T
type FirewallMigrator struct {
	client *vcd.Client
	dryRun bool
}

// NewFirewallMigrator khởi tạo FirewallMigrator
func NewFirewallMigrator(client *vcd.Client, dryRun bool) *FirewallMigrator {
	return &FirewallMigrator{client: client, dryRun: dryRun}
}

// resolveResult là kết quả resolve source/destination của một firewall rule.
// Bao gồm cả IP thô và references đến Firewall Group (IP Set) trên NSX-T.
type resolveResult struct {
	IPs    []string
	Groups []vcd.EntityReference
}

// Migrate copy tất cả User-defined Firewall rules từ srcEdge sang dstEdge
func (m *FirewallMigrator) Migrate(srcEdge, dstEdge *vcd.EdgeGateway) error {
	log.Printf("=== Firewall Migration: [%s] (NSX-V) → [%s] (NSX-T) ===", srcEdge.Name, dstEdge.Name)

	// Bước 1: Đọc thông tin interfaces để map subnet
	log.Printf("Step 1: Reading interfaces from source edge %q to map subnets...", srcEdge.Name)
	interfaces, err := m.client.GetNsxvGatewayInterfaces(srcEdge.ID)
	if err != nil {
		return fmt.Errorf("failed to read source edge interfaces: %w", err)
	}
	log.Printf("  Loaded %d interfaces", len(interfaces))
	for i, iface := range interfaces {
		log.Printf("    [%d] Name: %q, Type: %q, IP: %s, Subnet: %s",
			i, iface.Name, iface.InterfaceType, iface.SubnetParticipation.IpAddress, iface.SubnetCIDR())
	}

	// Bước 2: Đọc firewall rules từ NSX-V
	log.Printf("\nStep 2: Reading firewall rules from source edge %q...", srcEdge.Name)
	srcRules, err := m.client.GetNsxvFirewallRules(srcEdge.ID)
	if err != nil {
		return fmt.Errorf("failed to read source firewall rules: %w", err)
	}

	// Lọc chỉ copy rule có ruleType = "user"
	var userRules []vcd.NsxvFirewallRule
	for _, r := range srcRules {
		if strings.EqualFold(r.RuleType, "user") {
			userRules = append(userRules, r)
		}
	}
	log.Printf("  Found %d user-defined firewall rules (out of %d total rules)", len(userRules), len(srcRules))

	if len(userRules) == 0 {
		log.Println("  No rules to migrate.")
		return nil
	}

	// Xem trước các rule
	for i, r := range userRules {
		log.Printf("  [%d] tag=%s name=%q enabled=%v action=%s source=%+v dest=%+v",
			i+1, r.RuleTag, r.Name, r.Enabled, r.Action,
			r.Source, r.Destination)
	}

	dstURN := fmt.Sprintf("urn:vcloud:gateway:%s", dstEdge.ID)

	// Bước 3: Load tất cả application port profiles
	log.Printf("\nStep 3: Loading existing application port profiles...")
	profiles, err := m.client.ListAppPortProfiles()
	if err != nil {
		return fmt.Errorf("failed to load app port profiles: %w", err)
	}
	log.Printf("  Loaded %d application port profiles", len(profiles))

	// Lấy Owner URN context và OrgRef để tạo custom application profiles
	contextURN, orgRef, err := m.client.GetEdgeOwnerAndOrgRef(dstURN)
	if err != nil {
		return fmt.Errorf("failed to get destination edge owner and orgRef: %w", err)
	}

	// Bước 4: Load tất cả Firewall Groups (IP Sets) đã có trên NSX-T
	log.Printf("\nStep 4: Loading existing NSX-T firewall groups (IP Sets)...")
	existingFwGroups, err := m.client.ListNsxtFirewallGroups(dstURN)
	if err != nil {
		return fmt.Errorf("failed to load existing NSX-T firewall groups: %w", err)
	}
	log.Printf("  Loaded %d existing firewall groups", len(existingFwGroups))

	// ownerRef dành cho việc tạo Firewall Group mới trên NSX-T
	// Dùng edge gateway URN làm owner
	fwGroupOwnerRef := &vcd.EntityReference{
		ID:   dstURN,
		Name: dstEdge.Name,
	}

	// Bước 5: Kiểm tra rules đã tồn tại trên NSX-T (để tránh duplicate)
	log.Printf("\nStep 5: Checking existing NSX-T user firewall rules...")
	existingRules, err := m.client.GetNsxtFirewallRules(dstURN)
	if err != nil {
		return fmt.Errorf("failed to get existing NSX-T firewall rules: %w", err)
	}
	existingNames := map[string]bool{}
	for _, er := range existingRules {
		existingNames[er.Name] = true
	}
	log.Printf("  %d rules already on destination edge", len(existingRules))

	// Bước 6: Tạo rules trên NSX-T từ CUỐI lên ĐẦU (last→first)
	log.Printf("\nStep 6: Creating firewall rules on destination edge (last→first)...")
	created := 0
	skipped := 0

	for i := len(userRules) - 1; i >= 0; i-- {
		rule := userRules[i]
		ruleName := fmt.Sprintf("%s-%s", strings.TrimSpace(rule.RuleTag), strings.TrimSpace(rule.Name))
		if strings.TrimSpace(rule.Name) == "" {
			ruleName = strings.TrimSpace(rule.RuleTag)
		}

		if existingNames[ruleName] {
			log.Printf("  [SKIP] %q already exists on destination", ruleName)
			skipped++
			continue
		}

		// Giải quyết dải IP nguồn/đích từ các vnicGroupId/ipAddress/groupingObjectId
		// Đây là bước tạo Firewall Groups trên NSX-T nếu cần (cho IPSet)
		srcResult, updatedGroups, err := m.resolveSourceDest(
			srcEdge.ID, rule.Source, interfaces,
			existingFwGroups, fwGroupOwnerRef,
		)
		if err != nil {
			log.Printf("  [WARN] Failed to resolve source for rule %q: %v", ruleName, err)
			srcResult = &resolveResult{}
		}
		existingFwGroups = updatedGroups

		dstResult, updatedGroups, err := m.resolveSourceDest(
			srcEdge.ID, rule.Destination, interfaces,
			existingFwGroups, fwGroupOwnerRef,
		)
		if err != nil {
			log.Printf("  [WARN] Failed to resolve destination for rule %q: %v", ruleName, err)
			dstResult = &resolveResult{}
		}
		existingFwGroups = updatedGroups

		// Giải quyết profiles ứng dụng cho ports/protocols
		appProfiles, err := m.ResolveFirewallAppProfiles(rule.Application, profiles, ruleName, contextURN, orgRef)
		if err != nil {
			return fmt.Errorf("failed to resolve app profiles for rule %q: %w", ruleName, err)
		}

		// Xác định Action (ALLOW / DROP / REJECT)
		actionValue := "ALLOW"
		if strings.EqualFold(rule.Action, "deny") {
			actionValue = "DROP"
		}

		nsxtRule := vcd.NsxtFirewallRule{
			Name:                           ruleName,
			Description:                    rule.Description,
			Enabled:                        rule.Enabled,
			ActionValue:                    actionValue,
			Logging:                        rule.LoggingEnabled,
			SourceFirewallGroups:           srcResult.Groups,
			DestinationFirewallGroups:      dstResult.Groups,
			SourceFirewallIpAddresses:      srcResult.IPs,
			DestinationFirewallIpAddresses: dstResult.IPs,
			ApplicationPortProfiles:        appProfiles,
		}

		if m.dryRun {
			log.Printf("  [DRY-RUN] Would create firewall rule %q: Action=%s, SrcGroups=%v, SrcIPs=%v, DstGroups=%v, DstIPs=%v, Apps=%v",
				nsxtRule.Name, nsxtRule.ActionValue,
				nsxtRule.SourceFirewallGroups, nsxtRule.SourceFirewallIpAddresses,
				nsxtRule.DestinationFirewallGroups, nsxtRule.DestinationFirewallIpAddresses,
				nsxtRule.ApplicationPortProfiles)
			created++
			continue
		}

		log.Printf("  Creating firewall rule %q...", nsxtRule.Name)
		_, err = m.client.CreateNsxtFirewallRule(dstURN, nsxtRule)
		if err != nil {
			log.Printf("  ✗ FAILED to create firewall rule %q: %v", ruleName, err)
			continue
		}
		log.Printf("  ✓ Created firewall rule %q", nsxtRule.Name)
		created++

		// Đợi Edge realization
		log.Printf("  Waiting for edge gateway to realize firewall rule %q...", ruleName)
		if waitErr := m.client.WaitForEdgeRealized(dstURN, 120*time.Second, 3*time.Second); waitErr != nil {
			log.Printf("  ⚠ Warning: Failed to wait for edge gateway realization: %v", waitErr)
		} else {
			log.Printf("  ✓ Edge gateway status is REALIZED")
		}
	}

	log.Printf("\n=== Migration complete: %d created, %d skipped ===", created, skipped)
	return nil
}

// resolveSourceDest chuyển đổi các vnic-X, internal, external, vse, ipset... sang CIDR IP
// hoặc Firewall Group references trên NSX-T (cho IPSet).
// Trả về resolveResult + danh sách existingFwGroups đã cập nhật (có thể có group mới).
func (m *FirewallMigrator) resolveSourceDest(
	edgeID string,
	sd vcd.NsxvFirewallSourceDest,
	interfaces []vcd.NsxvGatewayInterface,
	existingFwGroups []vcd.NsxtFirewallGroup,
	ownerRef *vcd.EntityReference,
) (*resolveResult, []vcd.NsxtFirewallGroup, error) {
	result := &resolveResult{}

	// Xử lý vnicGroupId
	if sd.VnicGroupId != "" {
		vnicLower := strings.ToLower(strings.TrimSpace(sd.VnicGroupId))
		result.IPs = append(result.IPs, m.resolveKeyword(vnicLower, interfaces)...)
	}

	// Xử lý các ipAddress
	for _, ip := range sd.IpAddress {
		ipLower := strings.ToLower(strings.TrimSpace(ip))
		result.IPs = append(result.IPs, m.resolveKeyword(ipLower, interfaces)...)
	}

	// Xử lý các groupingObjectId (IP sets / Org VDC Networks)
	for _, goid := range sd.GroupingObjectIds {
		goid = strings.TrimSpace(goid)
		if goid == "" {
			continue
		}

		// 1. Kiểm tra nếu là network URN
		if strings.HasPrefix(goid, "urn:vcloud:network:") {
			found := false
			for _, iface := range interfaces {
				if iface.Network.ID == goid {
					cidr := iface.SubnetCIDR()
					if cidr != "" {
						result.IPs = append(result.IPs, cidr)
						found = true
						break
					}
				}
			}
			if !found {
				log.Printf("  [RESOLVE] Network URN %s not found in edge interfaces. Fetching network config...", goid)
				cidr, err := m.client.GetOrgVdcNetworkCIDR(goid)
				if err != nil {
					log.Printf("  [WARN] Failed to get CIDR for network %s: %v", goid, err)
				} else {
					result.IPs = append(result.IPs, cidr)
				}
			}
			continue
		}

		// 2. Kiểm tra nếu là IP Set → tạo/tìm Firewall Group trên NSX-T
		if strings.Contains(goid, "ipset-") {
			log.Printf("  [RESOLVE] IPSet ID %s detected. Resolving to NSX-T Firewall Group...", goid)

			// Lấy thông tin đầy đủ của IPSet từ NSX-V (tên + IPs)
			ipsetInfo, err := m.client.GetIPSetFull(edgeID, goid)
			if err != nil {
				log.Printf("  [WARN] Failed to get IPSet info for %s: %v — falling back to IP list", goid, err)
				// Fallback: dùng IP list thay vì group reference
				ips, fallbackErr := m.client.GetIPSetIPs(edgeID, goid)
				if fallbackErr != nil {
					log.Printf("  [WARN] Failed to get IPs for IPSet %s: %v", goid, fallbackErr)
				} else {
					result.IPs = append(result.IPs, ips...)
				}
				continue
			}

			ipAddresses := splitIPSetValues(ipsetInfo.Value)
			log.Printf("  [RESOLVE] IPSet %q (ID: %s) contains %d IPs: %v",
				ipsetInfo.Name, goid, len(ipAddresses), ipAddresses)

			if m.dryRun {
				// Dry-run: không tạo group thực, ghi log và dùng IPs
				log.Printf("  [DRY-RUN] Would create/find Firewall Group %q with %d IPs on NSX-T",
					ipsetInfo.Name, len(ipAddresses))
				result.IPs = append(result.IPs, ipAddresses...)
				continue
			}

			// Tạo hoặc tìm Firewall Group trên NSX-T
			fwGroup, updatedGroups, err := m.client.FindOrCreateNsxtFirewallGroup(
				existingFwGroups,
				ipsetInfo.Name,
				ipAddresses,
				ownerRef.ID,
				ownerRef,
			)
			existingFwGroups = updatedGroups

			if err != nil {
				log.Printf("  [WARN] Failed to create/find Firewall Group for IPSet %q: %v — falling back to IP list", ipsetInfo.Name, err)
				result.IPs = append(result.IPs, ipAddresses...)
				continue
			}

			log.Printf("  [RESOLVE] ✓ Firewall Group %q (ID: %s) ready", fwGroup.Name, fwGroup.ID)
			result.Groups = append(result.Groups, vcd.EntityReference{
				ID:   fwGroup.ID,
				Name: fwGroup.Name,
			})
			continue
		}

		// Fallback: đối xử như keyword thông thường
		result.IPs = append(result.IPs, m.resolveKeyword(strings.ToLower(goid), interfaces)...)
	}

	result.IPs = uniqueStrings(result.IPs)
	result.Groups = uniqueEntityRefs(result.Groups)
	return result, existingFwGroups, nil
}

// splitIPSetValues tách chuỗi comma-separated IPs từ value của IPSet
func splitIPSetValues(value string) []string {
	if value == "" {
		return nil
	}
	var ips []string
	for _, ip := range strings.Split(value, ",") {
		ip = strings.TrimSpace(ip)
		if ip != "" {
			ips = append(ips, ip)
		}
	}
	return ips
}

// ResolveSourceDest là phiên bản cũ (chỉ trả về IPs) — giữ lại để backward compat
// Khuyến cáo dùng resolveSourceDest thay thế.
func (m *FirewallMigrator) ResolveSourceDest(edgeID string, sd vcd.NsxvFirewallSourceDest, interfaces []vcd.NsxvGatewayInterface) []string {
	result, _, _ := m.resolveSourceDest(edgeID, sd, interfaces, nil, nil)
	all := append(result.IPs, func() []string {
		// Không thể chuyển group thành IP trong phiên bản compat này
		return nil
	}()...)
	return uniqueStrings(all)
}

func (m *FirewallMigrator) resolveKeyword(kw string, interfaces []vcd.NsxvGatewayInterface) []string {
	var resolved []string

	if kw == "" || kw == "any" {
		return nil
	}

	// 1. Khớp vnic-X
	if strings.HasPrefix(kw, "vnic-") || strings.HasPrefix(kw, "vnic") {
		idxStr := strings.TrimPrefix(kw, "vnic-")
		idxStr = strings.TrimPrefix(idxStr, "vnic")
		var idx int
		if _, err := fmt.Sscanf(idxStr, "%d", &idx); err == nil {
			if idx >= 0 && idx < len(interfaces) {
				cidr := interfaces[idx].SubnetCIDR()
				if cidr != "" {
					resolved = append(resolved, cidr)
				}
			}
		}
		return resolved
	}

	// 2. Khớp internal / org network
	if kw == "internal" || kw == "org network" || kw == "org-network" {
		for _, iface := range interfaces {
			if strings.EqualFold(iface.InterfaceType, "internal") {
				cidr := iface.SubnetCIDR()
				if cidr != "" {
					resolved = append(resolved, cidr)
				}
			}
		}
		return resolved
	}

	// 3. Khớp external / uplink
	if kw == "external" || kw == "uplink" {
		for _, iface := range interfaces {
			if strings.EqualFold(iface.InterfaceType, "uplink") {
				cidr := iface.SubnetCIDR()
				if cidr != "" {
					resolved = append(resolved, cidr)
				}
			}
		}
		return resolved
	}

	// 4. Khớp vse (Edge ip addresses)
	if kw == "vse" {
		for _, iface := range interfaces {
			if iface.SubnetParticipation.IpAddress != "" {
				resolved = append(resolved, iface.SubnetParticipation.IpAddress+"/32")
			}
		}
		return resolved
	}

	// 5. Khớp tên interface (case-insensitive)
	for _, iface := range interfaces {
		if strings.EqualFold(iface.Name, kw) || strings.EqualFold(iface.DisplayName, kw) {
			cidr := iface.SubnetCIDR()
			if cidr != "" {
				resolved = append(resolved, cidr)
			}
		}
	}
	if len(resolved) > 0 {
		return resolved
	}

	// 6. Ngược lại là IP/CIDR thông thường
	resolved = append(resolved, kw)
	return resolved
}

// ResolveFirewallAppProfiles map dịch vụ NSX-V sang application port profiles của NSX-T
func (m *FirewallMigrator) ResolveFirewallAppProfiles(
	app vcd.NsxvFirewallApplication,
	existingProfiles []vcd.AppPortProfileFull,
	ruleName string,
	contextURN string,
	orgRef *vcd.EntityReference,
) ([]vcd.AppPortProfile, error) {
	var result []vcd.AppPortProfile

	for _, service := range app.Services {
		proto := strings.ToLower(strings.TrimSpace(service.Protocol))
		if proto == "" || proto == "any" {
			continue
		}

		// ICMP -> ICMPv4-ALL
		if proto == "icmp" {
			profile, err := m.client.FindOrCreateAppPortProfile(existingProfiles, "icmp", "any", ruleName, contextURN, orgRef)
			if err != nil {
				return nil, err
			}
			if profile != nil {
				result = append(result, *profile)
			}
			continue
		}

		// TCP / UDP: Xử lý port
		if len(service.Ports) == 0 {
			profile, err := m.client.FindOrCreateAppPortProfile(existingProfiles, proto, "any", ruleName, contextURN, orgRef)
			if err != nil {
				return nil, err
			}
			if profile != nil {
				result = append(result, *profile)
			}
			continue
		}

		for _, port := range service.Ports {
			portStr := strings.ToLower(strings.TrimSpace(port))
			profile, err := m.client.FindOrCreateAppPortProfile(existingProfiles, proto, portStr, ruleName, contextURN, orgRef)
			if err != nil {
				return nil, err
			}
			if profile != nil {
				result = append(result, *profile)
			}
		}
	}

	// Loại bỏ profile trùng lặp
	var unique []vcd.AppPortProfile
	seen := map[string]bool{}
	for _, p := range result {
		if !seen[p.ID] {
			seen[p.ID] = true
			unique = append(unique, p)
		}
	}

	return unique, nil
}

func uniqueStrings(slice []string) []string {
	keys := make(map[string]bool)
	var list []string
	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func uniqueEntityRefs(refs []vcd.EntityReference) []vcd.EntityReference {
	seen := map[string]bool{}
	var unique []vcd.EntityReference
	for _, r := range refs {
		if !seen[r.ID] {
			seen[r.ID] = true
			unique = append(unique, r)
		}
	}
	return unique
}
