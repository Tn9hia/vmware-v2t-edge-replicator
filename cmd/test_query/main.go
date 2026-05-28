package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"v2t-replicate-edge/internal/config"
	"v2t-replicate-edge/internal/vcd"
)

func main() {
	cfg, err := config.LoadFromFile("/Users/nghia/Documents/code/Viettel/v2t-migration/v2t-replicate-edge/passfile")
	if err != nil {
		fmt.Printf("Load config failed: %v\n", err)
		return
	}

	client, err := vcd.NewClient(cfg)
	if err != nil {
		fmt.Printf("Auth failed: %v\n", err)
		return
	}

	tests := []struct {
		name   string
		path   string
		accept string
	}{
		{"Lab Edge Gateway Details", "/api/admin/edgeGateway/a212f426-1bfd-453f-8eea-1e2dafda03ef", "application/*+xml;version=37.2"},
		{"Proxied IPSet List: globalroot-0", "/network/edges/a212f426-1bfd-453f-8eea-1e2dafda03ef/api/2.0/services/ipsets/scope/globalroot-0", "application/*+xml;version=37.2"},
		{"Proxied IPSet List: vdc-UUID", "/network/edges/a212f426-1bfd-453f-8eea-1e2dafda03ef/api/2.0/services/ipsets/scope/vdc-cc255b20-b86a-4337-8e21-205936fbca31", "application/*+xml;version=37.2"},
	}

	out, err := os.Create("/Users/nghia/Documents/code/Viettel/v2t-migration/v2t-replicate-edge/test_outputs.txt")
	if err != nil {
		fmt.Printf("Create output file failed: %v\n", err)
		return
	}
	defer out.Close()

	for _, t := range tests {
		fmt.Fprintf(out, "\n--- Endpoint: %s ---\n", t.name)
		var req *http.Request
		var err error
		if strings.Contains(t.path, "/cloudapi/") {
			req, err = client.NewCloudAPIRequest(http.MethodGet, t.path, nil)
		} else {
			req, err = client.NewRequest(http.MethodGet, t.path, nil)
		}
		if err != nil {
			fmt.Fprintf(out, "Request build failed: %v\n", err)
			continue
		}
		req.Header.Set("Accept", t.accept)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(out, "Request failed: %v\n", err)
			continue
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(out, "HTTP Status: %d\n", resp.StatusCode)
		fmt.Fprintf(out, "Body:\n%s\n", string(body))
	}
}
