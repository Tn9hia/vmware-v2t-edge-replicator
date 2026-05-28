# V2T Edge Replicator (NSX-V to NSX-T Migration Tool)

A CLI tool written in Go designed to automate the migration of Firewall and NAT rules from VMware NSX-V Edge Gateways to NSX-T Edge Gateways within VMware Cloud Director (VCD).

## Features

* **Firewall Rule Migration:** Automatically copies user-defined firewall rules from NSX-V to NSX-T.
* **NAT Rule Migration:** Supports migration of both SNAT and DNAT rules.
* **Intelligent Object Resolution:** Automatically resolves NSX-V specific grouping objects (like IP Sets, vNICs, internal/external interfaces) into NSX-T compatible IPs, CIDRs, or NSX-T Firewall Groups.
* **App Port Profile Mapping:** Automatically maps NSX-V services/protocols to NSX-T Application Port Profiles, handling restrictions like strict single-port profiles for NAT rules.
* **Dry-Run Mode:** Safely preview the rule migration process without making any actual changes to the destination NSX-T edge.
* **Edge Listing:** Quickly list all available Edge Gateways in your VCD organization.

## Prerequisites

* Go 1.20 or higher (for building from source)
* Access to VMware Cloud Director API (System admin or Org admin credentials)

## Configuration (Passfile)

The tool requires a credentials file (referred to as a `passfile`) to authenticate with VCD.

Create a `passfile` with the following format:

```ini
host=https://your-vcd-domain.com
username=your_username
password=your_password
org=your_org_name
```

## Usage

You can run the tool directly using `go run` or build it into a binary.

### Build (Optional)

```bash
go build -o v2t-replicate ./cmd
```

### 1. List Available Edge Gateways
List all edges within the configured organization to easily find your source (NSX-V) and destination (NSX-T) edge names.

```bash
go run ./cmd/main.go -f passfile --list
```

### 2. Migrate Firewall Rules
Run in `--dry-run` mode first to preview the changes:

```bash
go run ./cmd/main.go -f passfile -s "Source-Edge-V" -d "Dest-Edge-T" -a firewall --dry-run
```

To actually execute the migration:

```bash
go run ./cmd/main.go -f passfile -s "Source-Edge-V" -d "Dest-Edge-T" -a firewall
```

### 3. Migrate NAT Rules
Run in `--dry-run` mode first to preview the changes:

```bash
go run ./cmd/main.go -f passfile -s "Source-Edge-V" -d "Dest-Edge-T" -a nat --dry-run
```

To actually execute the migration:

```bash
go run ./cmd/main.go -f passfile -s "Source-Edge-V" -d "Dest-Edge-T" -a nat
```

## Command Line Arguments

* `-f <path>`: (Required) Path to the passfile containing VCD credentials.
* `-s <name>`: Source Edge Gateway name (NSX-V).
* `-d <name>`: Destination Edge Gateway name (NSX-T).
* `-a <action>`: Action to perform (`firewall`, `nat`, or `list`).
* `--list`: List all available edge gateways and exit.
* `--dry-run`: Preview rules without creating them on the destination edge.

## Architecture

The migration tool is split into two primary layers:
1. `internal/vcd`: API Client handling authentication, legacy XML VCD APIs for NSX-V, and modern JSON CloudAPI for NSX-T.
2. `internal/migrator`: Migration orchestrator handling the business logic of translating NSX-V structures (rules, IPSets, services) into their equivalent NSX-T structures.
