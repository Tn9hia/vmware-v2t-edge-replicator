package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"v2t-replicate-edge/internal/config"
	"v2t-replicate-edge/internal/migrator"
	"v2t-replicate-edge/internal/vcd"
)

func main() {
	var (
		passfile string
		srcName  string
		dstName  string
		action   string
		listOnly bool
		dryRun   bool
	)

	flag.StringVar(&passfile, "f", "", "Path to passfile containing VCD credentials")
	flag.StringVar(&srcName, "s", "", "Source Edge Gateway name (NSX-V)")
	flag.StringVar(&dstName, "d", "", "Destination Edge Gateway name (NSX-T)")
	flag.StringVar(&action, "a", "", "Action: nat | firewall | list")
	flag.BoolVar(&listOnly, "list", false, "List all available edge gateways and exit")
	flag.BoolVar(&dryRun, "dry-run", false, "Preview rules without creating them")
	flag.Parse()

	// Validate passfile luôn bắt buộc
	if passfile == "" {
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  List edges:  %s -f <passfile> -list\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  Copy rules:  %s -f <passfile> -s <src_edge_name> -d <dst_edge_name> -a <nat|firewall>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Load config từ passfile
	cfg, err := config.LoadFromFile(passfile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Connecting to VCD: %s (org: %s)", cfg.Host, cfg.Org)

	// Authenticate với VCD
	client, err := vcd.NewClient(cfg)
	if err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}
	log.Printf("✓ Authentication successful!")

	// --- Chế độ list tất cả edge gateways ---
	if listOnly || action == "list" {
		printEdgeList(client)
		return
	}

	// Validate thêm flags khi không phải list mode
	if srcName == "" || dstName == "" || action == "" {
		fmt.Fprintf(os.Stderr, "Error: -s, -d, and -a are required for copy operations\n")
		os.Exit(1)
	}

	// --- Resolve tên edge -> ID ---
	log.Printf("Looking up source edge: %q", srcName)
	srcEdge, err := client.FindSourceEdgeGateway(srcName)
	if err != nil {
		log.Fatalf("Source edge lookup failed: %v", err)
	}
	log.Printf("  ✓ Found: %s | ID: %s | Type: %s | VDC: %s",
		srcEdge.Name, srcEdge.ID, srcEdge.GatewayType, srcEdge.VdcName)

	log.Printf("Looking up destination edge: %q", dstName)
	dstEdge, err := client.FindDestinationEdgeGateway(dstName)
	if err != nil {
		log.Fatalf("Destination edge lookup failed: %v", err)
	}
	log.Printf("  ✓ Found: %s | ID: %s | Type: %s | VDC: %s",
		dstEdge.Name, dstEdge.ID, dstEdge.GatewayType, dstEdge.VdcName)

	// Dispatch action
	switch action {
	case "nat":
		if dryRun {
			log.Println("[DRY-RUN] No rules will be created")
		}
		m := migrator.NewNATMigrator(client, dryRun)
		if err := m.Migrate(srcEdge, dstEdge); err != nil {
			log.Fatalf("NAT migration failed: %v", err)
		}
	case "firewall":
		if dryRun {
			log.Println("[DRY-RUN] No rules will be created")
		}
		m := migrator.NewFirewallMigrator(client, dryRun)
		if err := m.Migrate(srcEdge, dstEdge); err != nil {
			log.Fatalf("Firewall migration failed: %v", err)
		}
	default:
		log.Fatalf("Unknown action %q. Use: nat | firewall | list", action)
	}
}

// printEdgeList hiển thị danh sách tất cả edge gateways theo dạng bảng
func printEdgeList(client *vcd.Client) {
	log.Println("Fetching edge gateway list...")
	edges, err := client.ListEdgeGateways()
	if err != nil {
		log.Fatalf("Failed to list edge gateways: %v", err)
	}

	if len(edges) == 0 {
		fmt.Println("No edge gateways found.")
		return
	}

	fmt.Printf("\n%-45s %-15s %-36s %-20s %-6s\n",
		"NAME", "TYPE", "ID", "VDC", "STATUS")
	fmt.Printf("%s\n", fmt.Sprintf("%0*d", 130, 0))
	fmt.Printf("%-45s %-15s %-36s %-20s %-6s\n",
		"----", "----", "--", "---", "------")

	for _, eg := range edges {
		gtype := eg.GatewayType
		switch gtype {
		case "NSXV_BACKED":
			gtype = "NSX-V"
		case "NSXT_BACKED":
			gtype = "NSX-T"
		}
		fmt.Printf("%-45s %-15s %-36s %-20s %-6s\n",
			eg.Name, gtype, eg.ID, eg.VdcName, eg.Status)
	}
	fmt.Printf("\nTotal: %d edge gateways\n", len(edges))
}

