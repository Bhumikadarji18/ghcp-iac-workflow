package azure

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
)

// IdleResource describes a resource that appears to be unused or orphaned.
type IdleResource struct {
	Name           string
	Type           string
	ResourceGroup  string
	ResourceID     string
	Reason         string
	EstMonthlyCost float64
}

// orphanQuery defines one KQL query that detects a category of orphaned resources.
type orphanQuery struct {
	Name    string  // short label for logging
	Type    string  // display type in output
	KQL     string  // Azure Resource Graph KQL query
	Reason  string  // reason template (may contain %s for dynamic fields)
	EstCost float64 // default estimated monthly cost per resource (0 = free)
}

// orphanQueries is the comprehensive list of KQL queries for orphaned/idle resources.
var orphanQueries = []orphanQuery{
	{
		Name: "UnattachedDisks",
		Type: "Disk",
		KQL: `Resources
| where type =~ "microsoft.compute/disks"
| where properties.diskState == "Unattached"
| project name, resourceGroup, id, diskSizeGB = tostring(properties.diskSizeGB)`,
		Reason:  "Unattached disk",
		EstCost: 0.04, // per GB
	},
	{
		Name: "UnassociatedPublicIPs",
		Type: "PublicIP",
		KQL: `Resources
| where type =~ "microsoft.network/publicipaddresses"
| where isnull(properties.ipConfiguration) and isnull(properties.natGateway)
| project name, resourceGroup, id`,
		Reason:  "Public IP not associated to any resource",
		EstCost: 3.65,
	},
	{
		Name: "OrphanedNICs",
		Type: "NIC",
		KQL: `Resources
| where type =~ "microsoft.network/networkinterfaces"
| where isnull(properties.virtualMachine) and isnull(properties.privateEndpoint)
| project name, resourceGroup, id`,
		Reason:  "Network interface not attached to any VM or private endpoint",
		EstCost: 0,
	},
	{
		Name: "OrphanedNSGs",
		Type: "NSG",
		KQL: `Resources
| where type =~ "microsoft.network/networksecuritygroups"
| where isnull(properties.networkInterfaces) or properties.networkInterfaces == "[]"
| where isnull(properties.subnets) or properties.subnets == "[]"
| project name, resourceGroup, id`,
		Reason:  "Network security group not attached to any NIC or subnet",
		EstCost: 0,
	},
	{
		Name: "OrphanedRouteTables",
		Type: "RouteTable",
		KQL: `Resources
| where type =~ "microsoft.network/routetables"
| where isnull(properties.subnets) or properties.subnets == "[]"
| project name, resourceGroup, id`,
		Reason:  "Route table not associated to any subnet",
		EstCost: 0,
	},
	{
		Name: "OrphanedNATGateways",
		Type: "NATGateway",
		KQL: `Resources
| where type =~ "microsoft.network/natgateways"
| where isnull(properties.subnets) or properties.subnets == "[]"
| project name, resourceGroup, id`,
		Reason:  "NAT gateway not associated to any subnet",
		EstCost: 32.0,
	},
	{
		Name: "EmptyLoadBalancers",
		Type: "LoadBalancer",
		KQL: `Resources
| where type =~ "microsoft.network/loadbalancers"
| where properties.backendAddressPools == "[]" or isnull(properties.backendAddressPools)
| project name, resourceGroup, id`,
		Reason:  "Load balancer has no backends configured",
		EstCost: 18.0,
	},
	{
		Name: "EmptyAppGateways",
		Type: "AppGateway",
		KQL: `Resources
| where type =~ "microsoft.network/applicationgateways"
| where properties.backendAddressPools == "[]" or isnull(properties.backendAddressPools)
| project name, resourceGroup, id`,
		Reason:  "Application gateway has no backend targets",
		EstCost: 175.0,
	},
	{
		Name: "EmptyAppServicePlans",
		Type: "AppServicePlan",
		KQL: `Resources
| where type =~ "microsoft.web/serverfarms"
| where properties.numberOfSites == 0
| project name, resourceGroup, id, sku = tostring(sku.name)`,
		Reason:  "App Service plan has no apps",
		EstCost: 55.0,
	},
	{
		Name: "OrphanedPrivateDNSZones",
		Type: "PrivateDNSZone",
		KQL: `Resources
| where type =~ "microsoft.network/privatednszones"
| where properties.numberOfVirtualNetworkLinks == 0
| project name, resourceGroup, id`,
		Reason:  "Private DNS zone has no virtual network links",
		EstCost: 0.50,
	},
	{
		Name: "EmptyAvailabilitySets",
		Type: "AvailabilitySet",
		KQL: `Resources
| where type =~ "microsoft.compute/availabilitysets"
| where properties.virtualMachines == "[]" or isnull(properties.virtualMachines)
| project name, resourceGroup, id`,
		Reason:  "Availability set has no virtual machines",
		EstCost: 0,
	},
	{
		Name: "OldSnapshots",
		Type: "Snapshot",
		KQL: `Resources
| where type =~ "microsoft.compute/snapshots"
| where properties.timeCreated < ago(30d)
| project name, resourceGroup, id, diskSizeGB = tostring(properties.diskSizeGB), age = datetime_diff('day', now(), todatetime(properties.timeCreated))`,
		Reason:  "Snapshot older than 30 days",
		EstCost: 0.05, // per GB
	},
	{
		Name: "ExpiredCertificates",
		Type: "Certificate",
		KQL: `Resources
| where type =~ "microsoft.web/certificates"
| where properties.expirationDate < now()
| project name, resourceGroup, id, expiry = tostring(properties.expirationDate)`,
		Reason:  "Certificate has expired",
		EstCost: 0,
	},
	{
		Name: "OrphanedAPIConnections",
		Type: "APIConnection",
		KQL: `Resources
| where type =~ "microsoft.web/connections"
| where properties.statuses[0].status =~ "Error" or properties.statuses[0].status =~ "Unauthenticated"
| project name, resourceGroup, id, status = tostring(properties.statuses[0].status)`,
		Reason:  "API connection is in error/unauthenticated state",
		EstCost: 0,
	},
	{
		Name: "OrphanedPrivateEndpoints",
		Type: "PrivateEndpoint",
		KQL: `Resources
| where type =~ "microsoft.network/privateendpoints"
| where array_length(properties.networkInterfaces) == 0 or properties.privateLinkServiceConnections[0].properties.privateLinkServiceConnectionState.status =~ "Disconnected" or properties.privateLinkServiceConnections[0].properties.privateLinkServiceConnectionState.status =~ "Rejected"
| project name, resourceGroup, id`,
		Reason:  "Private endpoint is disconnected or has no NICs",
		EstCost: 0,
	},
	{
		Name: "OrphanedImages",
		Type: "Image",
		KQL: `Resources
| where type =~ "microsoft.compute/images"
| project name, resourceGroup, id`,
		Reason:  "Custom VM image (review if still needed)",
		EstCost: 0,
	},
	{
		Name: "UnattachedIPPrefixes",
		Type: "IPPrefix",
		KQL: `Resources
| where type =~ "microsoft.network/publicipprefixes"
| where isnull(properties.publicIPAddresses) or properties.publicIPAddresses == "[]"
| project name, resourceGroup, id`,
		Reason:  "Public IP prefix not assigned to any IP address",
		EstCost: 0,
	},
	{
		Name: "OrphanedDDOSProtection",
		Type: "DDoSProtection",
		KQL: `Resources
| where type =~ "microsoft.network/ddosprotectionplans"
| where isnull(properties.virtualNetworks) or properties.virtualNetworks == "[]"
| project name, resourceGroup, id`,
		Reason:  "DDoS protection plan not attached to any VNet",
		EstCost: 2944.0, // ~$2944/month
	},
	{
		Name: "OrphanedFrontDoorWAF",
		Type: "FrontDoorWAF",
		KQL: `Resources
| where type =~ "microsoft.network/frontdoorwebapplicationfirewallpolicies"
| where isnull(properties.frontendEndpointLinks) or properties.frontendEndpointLinks == "[]"
| where isnull(properties.securityPolicyLinks) or properties.securityPolicyLinks == "[]"
| project name, resourceGroup, id`,
		Reason:  "Front Door WAF policy not linked to any endpoint",
		EstCost: 0,
	},
	{
		Name: "StoppedVMs",
		Type: "VirtualMachine",
		KQL: `Resources
| where type =~ "microsoft.compute/virtualmachines"
| where properties.extended.instanceView.powerState.displayStatus =~ "VM deallocated" or properties.extended.instanceView.powerState.displayStatus =~ "VM stopped"
| project name, resourceGroup, id, powerState = tostring(properties.extended.instanceView.powerState.displayStatus)`,
		Reason:  "VM is stopped/deallocated (still incurs disk cost)",
		EstCost: 0,
	},
}

// DetectIdleResources uses Azure Resource Graph to detect ALL orphaned/idle
// resources across the given subscriptions with KQL queries.
func DetectIdleResources(ctx context.Context, cred azcore.TokenCredential, subscriptionIDs []string) ([]IdleResource, error) {
	client, err := armresourcegraph.NewClient(cred, nil)
	if err != nil {
		return nil, fmt.Errorf("azure: resource graph client: %w", err)
	}

	subPtrs := make([]*string, len(subscriptionIDs))
	for i, s := range subscriptionIDs {
		subPtrs[i] = to.Ptr(s)
	}

	var all []IdleResource
	for _, q := range orphanQueries {
		results, err := runOrphanQuery(ctx, client, subPtrs, q)
		if err != nil {
			log.Printf("idle: %s: %v", q.Name, err)
			continue
		}
		all = append(all, results...)
	}
	return all, nil
}

// runOrphanQuery executes a single KQL query and converts results to IdleResource.
func runOrphanQuery(ctx context.Context, client *armresourcegraph.Client, subs []*string, q orphanQuery) ([]IdleResource, error) {
	req := armresourcegraph.QueryRequest{
		Query:         to.Ptr(q.KQL),
		Subscriptions: subs,
	}

	var all []IdleResource
	for {
		resp, err := client.Resources(ctx, req, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", q.Name, err)
		}

		rows, ok := resp.Data.([]interface{})
		if !ok {
			break
		}

		for _, row := range rows {
			m, ok := row.(map[string]interface{})
			if !ok {
				continue
			}

			name := strVal(m, "name")
			rg := strVal(m, "resourceGroup")
			id := strVal(m, "id")
			reason := q.Reason
			estCost := q.EstCost

			// Enhance reason/cost with dynamic fields when available
			if diskGB := strVal(m, "diskSizeGB"); diskGB != "" {
				reason = fmt.Sprintf("%s (%s GB)", q.Reason, diskGB)
				if q.Name == "UnattachedDisks" || q.Name == "OldSnapshots" {
					gb := parseFloat(diskGB)
					estCost = gb * q.EstCost
				}
			}
			if age := strVal(m, "age"); age != "" {
				reason = fmt.Sprintf("%s (%s days old)", q.Reason, age)
			}
			if expiry := strVal(m, "expiry"); expiry != "" {
				reason = fmt.Sprintf("%s (expired: %s)", q.Reason, truncDate(expiry))
			}
			if status := strVal(m, "status"); status != "" {
				reason = fmt.Sprintf("%s (status: %s)", q.Reason, status)
			}
			if sku := strVal(m, "sku"); sku != "" {
				reason = fmt.Sprintf("%s (SKU: %s)", q.Reason, sku)
				if strings.EqualFold(sku, "F1") || strings.EqualFold(sku, "FREE") {
					estCost = 0
				}
			}
			if ps := strVal(m, "powerState"); ps != "" {
				reason = fmt.Sprintf("%s (%s)", q.Reason, ps)
			}

			all = append(all, IdleResource{
				Name:           name,
				Type:           q.Type,
				ResourceGroup:  rg,
				ResourceID:     id,
				Reason:         reason,
				EstMonthlyCost: estCost,
			})
		}

		// Handle pagination
		if resp.SkipToken == nil || *resp.SkipToken == "" {
			break
		}
		req.Options = &armresourcegraph.QueryRequestOptions{
			SkipToken: resp.SkipToken,
		}
	}
	return all, nil
}

// strVal extracts a string value from a map.
func strVal(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%.0f", val)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// parseFloat converts string to float64, returns 0 on failure.
func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// truncDate extracts just the date part from an ISO timestamp.
func truncDate(s string) string {
	if idx := strings.IndexByte(s, 'T'); idx > 0 {
		return s[:idx]
	}
	return s
}
