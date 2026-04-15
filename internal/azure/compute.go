package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
)

// VMInfo holds the relevant details of a running VM.
type VMInfo struct {
	Name          string
	ResourceGroup string
	ResourceID    string
	VMSize        string
	Location      string
	OSType        string
}

// ListVMs returns all VMs in the given subscription.
func ListVMs(ctx context.Context, cred azcore.TokenCredential, subscriptionID string) ([]VMInfo, error) {
	client, err := armcompute.NewVirtualMachinesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("azure: vm client: %w", err)
	}

	var vms []VMInfo
	pager := client.NewListAllPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("azure: list vms: %w", err)
		}
		for _, vm := range page.Value {
			if vm.ID == nil {
				continue
			}
			info := VMInfo{
				ResourceID: *vm.ID,
				Location:   valOrEmpty(vm.Location),
				Name:       valOrEmpty(vm.Name),
			}
			// Extract resource group from the resource ID
			info.ResourceGroup = resourceGroupFromID(*vm.ID)

			if vm.Properties != nil && vm.Properties.HardwareProfile != nil && vm.Properties.HardwareProfile.VMSize != nil {
				info.VMSize = string(*vm.Properties.HardwareProfile.VMSize)
			}
			if vm.Properties != nil && vm.Properties.StorageProfile != nil &&
				vm.Properties.StorageProfile.OSDisk != nil && vm.Properties.StorageProfile.OSDisk.OSType != nil {
				info.OSType = string(*vm.Properties.StorageProfile.OSDisk.OSType)
			}
			vms = append(vms, info)
		}
	}
	return vms, nil
}

// SKUInfo describes a VM SKU within a series for right-sizing.
type SKUInfo struct {
	Name     string
	VCPUs    int
	MemoryGB float64
	Hourly   float64
}

// VMSeriesSKUs maps series names (like "B", "Dsv3") to their SKU options
// ordered from smallest to largest.
var VMSeriesSKUs = map[string][]SKUInfo{
	"B": {
		{Name: "Standard_B1s", VCPUs: 1, MemoryGB: 1, Hourly: 0.0104},
		{Name: "Standard_B1ms", VCPUs: 1, MemoryGB: 2, Hourly: 0.0207},
		{Name: "Standard_B2s", VCPUs: 2, MemoryGB: 4, Hourly: 0.0416},
		{Name: "Standard_B2ms", VCPUs: 2, MemoryGB: 8, Hourly: 0.0832},
		{Name: "Standard_B4ms", VCPUs: 4, MemoryGB: 16, Hourly: 0.166},
		{Name: "Standard_B8ms", VCPUs: 8, MemoryGB: 32, Hourly: 0.333},
	},
	"Dsv3": {
		{Name: "Standard_D2s_v3", VCPUs: 2, MemoryGB: 8, Hourly: 0.096},
		{Name: "Standard_D4s_v3", VCPUs: 4, MemoryGB: 16, Hourly: 0.192},
		{Name: "Standard_D8s_v3", VCPUs: 8, MemoryGB: 32, Hourly: 0.384},
		{Name: "Standard_D16s_v3", VCPUs: 16, MemoryGB: 64, Hourly: 0.768},
	},
	"Dsv4": {
		{Name: "Standard_D2s_v4", VCPUs: 2, MemoryGB: 8, Hourly: 0.096},
		{Name: "Standard_D4s_v4", VCPUs: 4, MemoryGB: 16, Hourly: 0.192},
		{Name: "Standard_D8s_v4", VCPUs: 8, MemoryGB: 32, Hourly: 0.384},
	},
	"Dsv5": {
		{Name: "Standard_D2s_v5", VCPUs: 2, MemoryGB: 8, Hourly: 0.096},
		{Name: "Standard_D4s_v5", VCPUs: 4, MemoryGB: 16, Hourly: 0.192},
		{Name: "Standard_D8s_v5", VCPUs: 8, MemoryGB: 32, Hourly: 0.384},
	},
	"Esv3": {
		{Name: "Standard_E2s_v3", VCPUs: 2, MemoryGB: 16, Hourly: 0.126},
		{Name: "Standard_E4s_v3", VCPUs: 4, MemoryGB: 32, Hourly: 0.252},
		{Name: "Standard_E8s_v3", VCPUs: 8, MemoryGB: 64, Hourly: 0.504},
	},
	"Fsv2": {
		{Name: "Standard_F2s_v2", VCPUs: 2, MemoryGB: 4, Hourly: 0.085},
		{Name: "Standard_F4s_v2", VCPUs: 4, MemoryGB: 8, Hourly: 0.169},
		{Name: "Standard_F8s_v2", VCPUs: 8, MemoryGB: 16, Hourly: 0.338},
	},
}

// DetectSeries returns the series key for a given VM size, e.g.
// "Standard_D4s_v3" → "Dsv3", "Standard_B2ms" → "B".
func DetectSeries(vmSize string) string {
	lower := strings.ToLower(vmSize)
	for series := range VMSeriesSKUs {
		// Build a pattern from the series: "Dsv3" → "d" + "s_v3"
		// Use the first SKU in the series as a template
		skus := VMSeriesSKUs[series]
		if len(skus) == 0 {
			continue
		}
		template := strings.ToLower(skus[0].Name)
		// All SKUs in a series share the same suffix after the size digit
		// e.g. Standard_D2s_v3, Standard_D4s_v3 share "_d" and "s_v3"
		if sameSeriesPattern(lower, template) {
			return series
		}
	}
	return ""
}

// sameSeriesPattern checks whether two SKU names belong to the same series
// by comparing the prefix (Standard_) and the suffix after the digit(s).
func sameSeriesPattern(a, b string) bool {
	aSuffix := extractSuffix(a)
	bSuffix := extractSuffix(b)
	return aSuffix != "" && aSuffix == bSuffix
}

// extractSuffix returns everything after "standard_" and after the first digit run.
// "standard_d4s_v3" → "d" + "s_v3" → "ds_v3"
func extractSuffix(sku string) string {
	s := strings.TrimPrefix(sku, "standard_")
	if s == sku {
		return ""
	}
	// Find the first digit
	var prefix, suffix string
	inDigits := false
	for i, c := range s {
		if c >= '0' && c <= '9' {
			prefix = s[:i]
			inDigits = true
			continue
		}
		if inDigits {
			suffix = s[i:]
			break
		}
	}
	if !inDigits {
		return ""
	}
	return prefix + suffix
}

// GetDownsizeRecommendation returns the next smaller SKU in the same series.
// If the VM is already the smallest, it returns empty.
func GetDownsizeRecommendation(currentSKU string) (recommended SKUInfo, current SKUInfo, found bool) {
	series := DetectSeries(currentSKU)
	if series == "" {
		return SKUInfo{}, SKUInfo{}, false
	}
	skus := VMSeriesSKUs[series]
	lower := strings.ToLower(currentSKU)
	for i, s := range skus {
		if strings.ToLower(s.Name) == lower {
			if i == 0 {
				// Already the smallest in the series
				return SKUInfo{}, s, false
			}
			return skus[i-1], s, true
		}
	}
	return SKUInfo{}, SKUInfo{}, false
}

// --- helpers ---

func valOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func resourceGroupFromID(id string) string {
	parts := strings.Split(id, "/")
	for i, p := range parts {
		if strings.EqualFold(p, "resourceGroups") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
