package azure

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/monitor/armmonitor"
)

// GetVMCPUAverage returns the average CPU percentage for a VM over the past N days.
// The subscriptionID is extracted from the resourceID.
func GetVMCPUAverage(ctx context.Context, cred azcore.TokenCredential, resourceID string, days int) (float64, error) {
	subID := extractSubscriptionID(resourceID)
	client, err := armmonitor.NewMetricsClient(subID, cred, nil)
	if err != nil {
		return 0, fmt.Errorf("azure: monitor client: %w", err)
	}

	now := time.Now().UTC()
	from := now.AddDate(0, 0, -days)
	timespan := fmt.Sprintf("%s/%s", from.Format(time.RFC3339), now.Format(time.RFC3339))

	resp, err := client.List(ctx, resourceID, &armmonitor.MetricsClientListOptions{
		Timespan:        to.Ptr(timespan),
		Interval:        to.Ptr("P1D"),
		Metricnames:     to.Ptr("Percentage CPU"),
		Aggregation:     to.Ptr("average"),
		Metricnamespace: to.Ptr("Microsoft.Compute/virtualMachines"),
	})
	if err != nil {
		return 0, fmt.Errorf("azure: get cpu metrics for %s: %w", resourceID, err)
	}

	return extractAverageCPU(resp), nil
}

// extractAverageCPU calculates the overall average from the timeseries data.
func extractAverageCPU(resp armmonitor.MetricsClientListResponse) float64 {
	if len(resp.Value) == 0 {
		return 0
	}
	var total float64
	var count int
	for _, metric := range resp.Value {
		for _, ts := range metric.Timeseries {
			for _, dp := range ts.Data {
				if dp.Average != nil {
					total += *dp.Average
					count++
				}
			}
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

// extractSubscriptionID parses the subscription ID from an Azure resource ID.
// Format: /subscriptions/{subID}/resourceGroups/...
func extractSubscriptionID(resourceID string) string {
	parts := splitResourceID(resourceID)
	for i, p := range parts {
		if strings.EqualFold(p, "subscriptions") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func splitResourceID(id string) []string {
	var parts []string
	for _, p := range strings.Split(id, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}
