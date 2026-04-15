package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/advisor/armadvisor"
)

// AdvisorRecommendation holds a single Azure Advisor recommendation.
type AdvisorRecommendation struct {
	Category          string // Cost, HighAvailability, Performance, Security, OperationalExcellence
	Impact            string // High, Medium, Low
	ImpactedResource  string // resource name
	ImpactedType      string // resource type (e.g. Microsoft.Compute/virtualMachines)
	ResourceGroup     string
	Problem           string
	Solution          string
	PotentialBenefits string
	ResourceID        string
}

// ListAdvisorRecommendations fetches all Azure Advisor recommendations for a
// subscription, optionally filtered by category.
// Pass empty category to get all recommendations.
func ListAdvisorRecommendations(ctx context.Context, cred azcore.TokenCredential, subscriptionID string, category string) ([]AdvisorRecommendation, error) {
	client, err := armadvisor.NewRecommendationsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("azure: advisor client: %w", err)
	}

	opts := &armadvisor.RecommendationsClientListOptions{}
	if category != "" {
		filter := fmt.Sprintf("Category eq '%s'", category)
		opts.Filter = &filter
	}

	var recs []AdvisorRecommendation
	pager := client.NewListPager(opts)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("azure: list advisor recommendations: %w", err)
		}
		for _, r := range page.Value {
			if r.Properties == nil {
				continue
			}
			rec := AdvisorRecommendation{
				ResourceID: valOrEmpty(r.ID),
			}

			if r.Properties.Category != nil {
				rec.Category = string(*r.Properties.Category)
			}
			if r.Properties.Impact != nil {
				rec.Impact = string(*r.Properties.Impact)
			}
			if r.Properties.ImpactedValue != nil {
				rec.ImpactedResource = *r.Properties.ImpactedValue
			}
			if r.Properties.ImpactedField != nil {
				rec.ImpactedType = *r.Properties.ImpactedField
			}
			if r.Properties.ShortDescription != nil {
				if r.Properties.ShortDescription.Problem != nil {
					rec.Problem = *r.Properties.ShortDescription.Problem
				}
				if r.Properties.ShortDescription.Solution != nil {
					rec.Solution = *r.Properties.ShortDescription.Solution
				}
			}
			if r.Properties.PotentialBenefits != nil {
				rec.PotentialBenefits = *r.Properties.PotentialBenefits
			}
			if r.Properties.ResourceMetadata != nil && r.Properties.ResourceMetadata.ResourceID != nil {
				rec.ResourceID = *r.Properties.ResourceMetadata.ResourceID
			}
			rec.ResourceGroup = resourceGroupFromID(rec.ResourceID)

			recs = append(recs, rec)
		}
	}
	return recs, nil
}

// FilterRecommendationsByCategory filters recommendations by category (case-insensitive).
func FilterRecommendationsByCategory(recs []AdvisorRecommendation, category string) []AdvisorRecommendation {
	var filtered []AdvisorRecommendation
	cat := strings.ToLower(category)
	for _, r := range recs {
		if strings.ToLower(r.Category) == cat {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
