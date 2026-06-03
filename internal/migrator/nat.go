package migrator

import (
	"fmt"
	"log"
	"strings"
	"time"

	"v2t-replicate-edge/internal/vcd"
)

// NATMigrator thực hiện copy NAT rules từ NSX-V sang NSX-T
type NATMigrator struct {
	client  *vcd.Client
	dryRun  bool
}

// NewNATMigrator khởi tạo NATMigrator
func NewNATMigrator(client *vcd.Client, dryRun bool) *NATMigrator {
	return &NATMigrator{client: client, dryRun: dryRun}
}

// Migrate copy tất cả User-defined NAT rules từ srcEdge sang dstEdge
// Quy trình:
//  1. Đọc NAT rules từ NSX-V (chỉ type=User)
//  2. Gom danh sách port/protocol cần custom application profile
//  3. Tạo custom app port profiles nếu chưa có
//  4. Tạo NAT rules trên NSX-T theo thứ tự từ last→first
func (m *NATMigrator) Migrate(srcEdge, dstEdge *vcd.EdgeGateway) error {
	log.Printf("=== NAT Migration: [%s] (NSX-V) → [%s] (NSX-T) ===", srcEdge.Name, dstEdge.Name)

	// Bước 1: Đọc NAT rules từ NSX-V
	log.Printf("Step 1: Reading NAT rules from source edge %q...", srcEdge.Name)
	srcRules, err := m.client.GetNsxvNatRules(srcEdge.ID)
	if err != nil {
		return fmt.Errorf("failed to read source NAT rules: %w", err)
	}
	log.Printf("  Found %d User-defined NAT rules", len(srcRules))

	if len(srcRules) == 0 {
		log.Println("  No rules to migrate.")
		return nil
	}

	// In preview
	for i, r := range srcRules {
		log.Printf("  [%d] [%s] id=%s desc=%q orig=%s:%s → trans=%s:%s proto=%s",
			i+1, r.RuleType, r.ID, r.Description,
			r.GatewayNat.OriginalIP, r.GatewayNat.OriginalPort,
			r.GatewayNat.TranslatedIP, r.GatewayNat.TranslatedPort,
			r.GatewayNat.Protocol)
	}

	// Bước 2: Load tất cả application port profiles hiện có
	log.Printf("\nStep 2: Loading existing application port profiles...")
	profiles, err := m.client.ListAppPortProfiles()
	if err != nil {
		return fmt.Errorf("failed to load app port profiles: %w", err)
	}
	log.Printf("  Loaded %d application port profiles", len(profiles))

	// Build NSX-T URN cho destination edge
	dstURN := fmt.Sprintf("urn:vcloud:gateway:%s", dstEdge.ID)

	// Bước 3: Build mapping protocol+port → AppPortProfile
	// Gom tất cả cặp (protocol, port) cần dùng để tạo trước
	type portKey struct{ proto, port string }
	appProfileCache := map[portKey]*vcd.AppPortProfile{}

	log.Printf("\nStep 3: Resolving application port profiles...")
	// Lấy Owner URN context và OrgRef để tạo custom application profiles
	contextURN, orgRef, err := m.client.GetEdgeOwnerAndOrgRef(dstURN)
	if err != nil {
		return fmt.Errorf("failed to get destination edge owner URN: %w", err)
	}

	for _, rule := range srcRules {
		proto := strings.ToLower(strings.TrimSpace(rule.GatewayNat.Protocol))
		// App profile dùng internal port (TranslatedPort cho DNAT) — đây là port của service thực bên trong
		appPort := determineAppPort(rule)
		key := portKey{proto, appPort}

		if _, exists := appProfileCache[key]; !exists {
			ruleName := buildRuleName(rule)
			profile, err := m.client.FindOrCreateAppPortProfile(profiles, proto, appPort, ruleName, contextURN, orgRef)
			if err != nil {
				return fmt.Errorf("app port profile error for rule %q: %w", rule.ID, err)
			}
			appProfileCache[key] = profile
			if profile != nil {
				log.Printf("  ✓ [%s/%s] → %s (%s)", proto, appPort, profile.Name, profile.ID)
			} else {
				log.Printf("  ✓ [%s/%s] → no app profile needed (any protocol/port)", proto, appPort)
			}
		}
	}

	// Kiểm tra rules đã tồn tại trên NSX-T (để tránh duplicate)
	log.Printf("\nStep 4: Checking existing NSX-T NAT rules...")
	existingRules, err := m.client.GetNsxtNatRules(dstURN)
	if err != nil {
		return fmt.Errorf("failed to get existing NSX-T NAT rules: %w", err)
	}
	existingNames := map[string]bool{}
	for _, er := range existingRules {
		existingNames[er.Name] = true
	}
	log.Printf("  %d rules already on destination edge", len(existingRules))

	// Bước 4: Tạo rules trên NSX-T từ CUỐI lên ĐẦU (theo .note)
	log.Printf("\nStep 5: Creating NAT rules on destination edge (last→first)...")
	created := 0
	skipped := 0

	for i := len(srcRules) - 1; i >= 0; i-- {
		rule := srcRules[i]
		ruleName := buildRuleName(rule)

		// Skip nếu đã tồn tại
		if existingNames[ruleName] {
			log.Printf("  [SKIP] %q already exists on destination", ruleName)
			skipped++
			continue
		}

		// Resolve app port profile dựa theo internal port (TranslatedPort cho DNAT)
		proto := strings.ToLower(strings.TrimSpace(rule.GatewayNat.Protocol))
		appPort := determineAppPort(rule)
		key := portKey{proto, appPort}
		appProfile := appProfileCache[key]

		// Build NSX-T NAT rule
		nsxtRule := buildNsxtRule(rule, ruleName, appProfile)

		if m.dryRun {
			log.Printf("  [DRY-RUN] Would create [%s] %q ext=%s int=%s app=%v",
				nsxtRule.Type, nsxtRule.Name,
				nsxtRule.ExternalAddresses, nsxtRule.InternalAddresses,
				nsxtRule.ApplicationPortProfile)
			created++
			continue
		}

		log.Printf("  Creating [%s] %q...", nsxtRule.Type, nsxtRule.Name)
		_, err := m.client.CreateNsxtNatRule(dstURN, nsxtRule)
		if err != nil {
			log.Printf("  ✗ FAILED to create %q: %v", ruleName, err)
			// Tiếp tục với rule tiếp theo thay vì abort
			continue
		}
		log.Printf("  ✓ Created [%s] %q", nsxtRule.Type, nsxtRule.Name)
		created++

		// Chờ edge gateway update xong rule mới tạo rule tiếp theo
		log.Printf("  Waiting for edge gateway to realize rule %q...", ruleName)
		if waitErr := m.client.WaitForEdgeRealized(dstURN, 120*time.Second, 3*time.Second); waitErr != nil {
			log.Printf("  ⚠ Warning: Failed to wait for edge gateway realization: %v", waitErr)
		} else {
			log.Printf("  ✓ Edge gateway status is REALIZED")
		}
	}

	log.Printf("\n=== Migration complete: %d created, %d skipped ===", created, skipped)
	return nil
}

// buildRuleName tạo tên rule theo format: "<ruleTag> - <description>" hoặc "<ruleTag>"
func buildRuleName(rule vcd.NsxvNatRule) string {
	tag := strings.TrimSpace(rule.ID)
	desc := strings.TrimSpace(rule.Description)
	if desc != "" {
		return fmt.Sprintf("%s - %s", tag, desc)
	}
	return tag
}

// determineAppPort lấy port dùng cho application port profile.
// NSX-T dùng app profile để xác định service cần match:
//   - DNAT: dùng TranslatedPort (internal port) — đây là port thực của service bên trong
//     ExternalPort sẽ được set riêng trong DnatExternalPort
//   - SNAT: không cần port match (thường any)
func determineAppPort(rule vcd.NsxvNatRule) string {
	if strings.EqualFold(rule.RuleType, "DNAT") {
		port := strings.ToLower(strings.TrimSpace(rule.GatewayNat.TranslatedPort))
		if port == "" {
			return "any"
		}
		return port
	}
	// SNAT: không có port destination concept trong NSX-T
	return "any"
}

// buildNsxtRule convert NSX-V rule sang NSX-T format
func buildNsxtRule(src vcd.NsxvNatRule, name string, appProfile *vcd.AppPortProfile) vcd.NsxtNatRule {
	rule := vcd.NsxtNatRule{
		Name:        name,
		Description: src.Description,
		Enabled:     src.IsEnabled,
		Type:        strings.ToUpper(src.RuleType), // SNAT | DNAT
		Logging:     false,
		// NSX-T: MATCH_INTERNAL_ADDRESS là default phổ biến
		FirewallMatch: "MATCH_INTERNAL_ADDRESS",
		ApplicationPortProfile: appProfile,
	}

	switch strings.ToUpper(src.RuleType) {
	case "SNAT":
		// SNAT: externalAddresses = translatedIP, internalAddresses = originalIP
		rule.ExternalAddresses = src.GatewayNat.TranslatedIP
		rule.InternalAddresses = src.GatewayNat.OriginalIP
	case "DNAT":
		// DNAT: externalAddresses = originalIP (public), internalAddresses = translatedIP (private)
		rule.ExternalAddresses = src.GatewayNat.OriginalIP
		rule.InternalAddresses = src.GatewayNat.TranslatedIP
		// dnatExternalPort: port ngoài cần translate
		if port := strings.TrimSpace(src.GatewayNat.OriginalPort); port != "" && port != "any" {
			rule.DnatExternalPort = port
		}
	}

	return rule
}
