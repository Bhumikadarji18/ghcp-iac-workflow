package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
)

// SubscriptionInfo holds basic subscription metadata.
type SubscriptionInfo struct {
	ID          string
	DisplayName string
}

// ListSubscriptions returns all subscriptions accessible by the credential.
func ListSubscriptions(ctx context.Context, cred azcore.TokenCredential) ([]SubscriptionInfo, error) {
	client, err := armsubscriptions.NewClient(cred, nil)
	if err != nil {
		return nil, fmt.Errorf("azure: subscriptions client: %w", err)
	}

	var subs []SubscriptionInfo
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("azure: list subscriptions: %w", err)
		}
		for _, s := range page.Value {
			if s.SubscriptionID == nil {
				continue
			}
			name := ""
			if s.DisplayName != nil {
				name = *s.DisplayName
			}
			subs = append(subs, SubscriptionInfo{
				ID:          *s.SubscriptionID,
				DisplayName: name,
			})
		}
	}
	return subs, nil
}
