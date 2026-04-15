package azure

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/costmanagement/armcostmanagement"
)

// SubscriptionCost holds a single month's cost for a subscription.
type SubscriptionCost struct {
	SubscriptionID   string
	SubscriptionName string
	Month            string
	Cost             float64
	Currency         string
}

// ResourceGroupCost holds a single month's cost for a resource group.
type ResourceGroupCost struct {
	ResourceGroup string
	Month         string
	Cost          float64
	Currency      string
}

// CostClient wraps the Azure Cost Management Query API.
type CostClient struct {
	client *armcostmanagement.QueryClient
}

// NewCostClient creates a CostClient for querying cost data.
func NewCostClient(cred azcore.TokenCredential) (*CostClient, error) {
	client, err := armcostmanagement.NewQueryClient(cred, nil)
	if err != nil {
		return nil, fmt.Errorf("azure: cost query client: %w", err)
	}
	return &CostClient{client: client}, nil
}

// QueryMonthlyCostByResourceGroup returns resource-group-level monthly costs
// for the given subscription over the last N months.
func (c *CostClient) QueryMonthlyCostByResourceGroup(ctx context.Context, subscriptionID string, from, toTime time.Time) ([]ResourceGroupCost, error) {
	scope := fmt.Sprintf("/subscriptions/%s", subscriptionID)

	granularity := armcostmanagement.GranularityTypeDaily
	queryDef := armcostmanagement.QueryDefinition{
		Type:      to.Ptr(armcostmanagement.ExportTypeActualCost),
		Timeframe: to.Ptr(armcostmanagement.TimeframeTypeCustom),
		TimePeriod: &armcostmanagement.QueryTimePeriod{
			From: to.Ptr(from),
			To:   to.Ptr(toTime),
		},
		Dataset: &armcostmanagement.QueryDataset{
			Granularity: &granularity,
			Aggregation: map[string]*armcostmanagement.QueryAggregation{
				"totalCost": {
					Name:     to.Ptr("PreTaxCost"),
					Function: to.Ptr(armcostmanagement.FunctionTypeSum),
				},
			},
			Grouping: []*armcostmanagement.QueryGrouping{
				{
					Type: to.Ptr(armcostmanagement.QueryColumnTypeDimension),
					Name: to.Ptr("ResourceGroup"),
				},
			},
		},
	}

	resp, err := c.client.Usage(ctx, scope, queryDef, nil)
	if err != nil {
		return nil, fmt.Errorf("azure: cost query for %s: %w", subscriptionID, err)
	}

	return aggregateRGCostByMonth(parseRGCostRows(resp.QueryResult)), nil
}

// QuerySubscriptionTotalCost returns the total monthly cost for a subscription
// over the last N months.
func (c *CostClient) QuerySubscriptionTotalCost(ctx context.Context, subscriptionID string, from, toTime time.Time) ([]SubscriptionCost, error) {
	scope := fmt.Sprintf("/subscriptions/%s", subscriptionID)

	granularity := armcostmanagement.GranularityTypeDaily
	queryDef := armcostmanagement.QueryDefinition{
		Type:      to.Ptr(armcostmanagement.ExportTypeActualCost),
		Timeframe: to.Ptr(armcostmanagement.TimeframeTypeCustom),
		TimePeriod: &armcostmanagement.QueryTimePeriod{
			From: to.Ptr(from),
			To:   to.Ptr(toTime),
		},
		Dataset: &armcostmanagement.QueryDataset{
			Granularity: &granularity,
			Aggregation: map[string]*armcostmanagement.QueryAggregation{
				"totalCost": {
					Name:     to.Ptr("PreTaxCost"),
					Function: to.Ptr(armcostmanagement.FunctionTypeSum),
				},
			},
		},
	}

	resp, err := c.client.Usage(ctx, scope, queryDef, nil)
	if err != nil {
		return nil, fmt.Errorf("azure: subscription cost query for %s: %w", subscriptionID, err)
	}

	return aggregateSubCostByMonth(parseSubCostRows(resp.QueryResult, subscriptionID)), nil
}

// parseRGCostRows extracts ResourceGroupCost entries from the query result.
// Expected columns: PreTaxCost, ResourceGroup, BillingMonth, Currency
// parseRGCostRows extracts daily ResourceGroupCost entries from the query result.
// With Daily granularity the date column is "UsageDate".
func parseRGCostRows(result armcostmanagement.QueryResult) []ResourceGroupCost {
	if result.Properties == nil || len(result.Properties.Rows) == 0 {
		return nil
	}

	colIdx := columnIndex(result.Properties.Columns)
	costCol := colIdx["PreTaxCost"]
	rgCol := colIdx["ResourceGroup"]
	dateCol := colIdx["UsageDate"]
	currCol := colIdx["Currency"]

	var costs []ResourceGroupCost
	for _, row := range result.Properties.Rows {
		costs = append(costs, ResourceGroupCost{
			Cost:          toFloat(row, costCol),
			ResourceGroup: toStr(row, rgCol),
			Month:         toMonthStr(row, dateCol),
			Currency:      toStr(row, currCol),
		})
	}
	return costs
}

func parseSubCostRows(result armcostmanagement.QueryResult, subID string) []SubscriptionCost {
	if result.Properties == nil || len(result.Properties.Rows) == 0 {
		return nil
	}
	colIdx := columnIndex(result.Properties.Columns)
	costCol := colIdx["PreTaxCost"]
	dateCol := colIdx["UsageDate"]
	currCol := colIdx["Currency"]

	var costs []SubscriptionCost
	for _, row := range result.Properties.Rows {
		costs = append(costs, SubscriptionCost{
			SubscriptionID: subID,
			Cost:           toFloat(row, costCol),
			Month:          toMonthStr(row, dateCol),
			Currency:       toStr(row, currCol),
		})
	}
	return costs
}

// aggregateRGCostByMonth sums daily costs into monthly buckets per resource group.
func aggregateRGCostByMonth(daily []ResourceGroupCost) []ResourceGroupCost {
	type key struct{ RG, Month string }
	agg := make(map[key]*ResourceGroupCost)
	for _, d := range daily {
		k := key{d.ResourceGroup, d.Month}
		if existing, ok := agg[k]; ok {
			existing.Cost += d.Cost
		} else {
			copy := d
			agg[k] = &copy
		}
	}
	result := make([]ResourceGroupCost, 0, len(agg))
	for _, v := range agg {
		result = append(result, *v)
	}
	return result
}

// aggregateSubCostByMonth sums daily costs into monthly buckets per subscription.
func aggregateSubCostByMonth(daily []SubscriptionCost) []SubscriptionCost {
	type key struct{ SubID, Month string }
	agg := make(map[key]*SubscriptionCost)
	for _, d := range daily {
		k := key{d.SubscriptionID, d.Month}
		if existing, ok := agg[k]; ok {
			existing.Cost += d.Cost
		} else {
			copy := d
			agg[k] = &copy
		}
	}
	result := make([]SubscriptionCost, 0, len(agg))
	for _, v := range agg {
		result = append(result, *v)
	}
	return result
}

// --- helpers ---

func columnIndex(cols []*armcostmanagement.QueryColumn) map[string]int {
	m := make(map[string]int, len(cols))
	for i, c := range cols {
		if c.Name != nil {
			m[*c.Name] = i
		}
	}
	return m
}

func toFloat(row []any, idx int) float64 {
	if idx < 0 || idx >= len(row) {
		return 0
	}
	switch v := row[idx].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	default:
		return 0
	}
}

func toStr(row []any, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	s, _ := row[idx].(string)
	return s
}

func toMonthStr(row []any, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	switch v := row[idx].(type) {
	case float64:
		// e.g. 20260301 → "2026-03"
		y := int(v) / 10000
		m := (int(v) % 10000) / 100
		return fmt.Sprintf("%d-%02d", y, m)
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}
