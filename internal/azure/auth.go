// Package azure provides clients for querying live Azure APIs
// (Cost Management, Monitor, Compute, Network) using the Azure SDK for Go.
package azure

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// NewCredential creates an Azure token credential from explicit service principal
// fields. If clientID or clientSecret is empty it falls back to
// DefaultAzureCredential which supports managed-identity, Azure CLI, etc.
func NewCredential(tenantID, clientID, clientSecret string) (azcore.TokenCredential, error) {
	if tenantID != "" && clientID != "" && clientSecret != "" {
		cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
		if err != nil {
			return nil, fmt.Errorf("azure: client secret credential: %w", err)
		}
		return cred, nil
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("azure: default credential: %w", err)
	}
	return cred, nil
}
