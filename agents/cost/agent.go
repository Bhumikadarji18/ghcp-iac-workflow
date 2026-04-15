// Package cost provides the Cost Estimator agent.
package cost

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/ghcp-iac/ghcp-iac-workflow/internal/azure"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/config"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/llm"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/parser"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/protocol"
)

// Agent estimates monthly Azure costs for IaC resources.
// When Azure credentials are configured it also supports live queries:
// subscription/RG cost breakdown, VM right-sizing, and idle resource detection.
type Agent struct {
	llmClient *llm.Client
	enableLLM bool
	cfg       *config.Config
}

// New creates a new cost Agent.
func New(opts ...Option) *Agent {
	a := &Agent{}
	for _, o := range opts {
		o(a)
	}
	return a
}

// Option configures a cost Agent.
type Option func(*Agent)

// WithLLM enables LLM-enhanced cost analysis.
func WithLLM(client *llm.Client) Option {
	return func(a *Agent) {
		a.llmClient = client
		a.enableLLM = client != nil
	}
}

// WithAzureConfig supplies Azure credentials so the agent can query live APIs.
func WithAzureConfig(cfg *config.Config) Option {
	return func(a *Agent) {
		a.cfg = cfg
	}
}

func (a *Agent) ID() string { return "cost" }

func (a *Agent) Metadata() protocol.AgentMetadata {
	return protocol.AgentMetadata{
		ID:          "cost",
		Name:        "Cost Estimator",
		Description: "Estimates monthly Azure costs for declared IaC resources using static pricing tables",
		Version:     "1.0.0",
	}
}

func (a *Agent) Capabilities() protocol.AgentCapabilities {
	return protocol.AgentCapabilities{
		Formats:       []protocol.SourceFormat{protocol.FormatTerraform, protocol.FormatBicep},
		NeedsIaCInput: false, // live Azure queries don't need IaC
	}
}

// Handle routes cost requests to the right handler based on user intent.
// It checks the user prompt for keywords first; if none match and IaC code
// is present it falls back to static cost estimation.
func (a *Agent) Handle(ctx context.Context, req protocol.AgentRequest, emit protocol.Emitter) error {
	prompt := strings.ToLower(protocol.PromptText(req))

	// --- Live Azure query routes (need credentials) ---
	switch {
	case protocol.MatchesAny(prompt, "cost breakdown", "subscription cost", "monthly cost",
		"resource group cost", "cost details", "subscription wise", "resource group wise"):
		return a.handleCostBreakdown(ctx, emit)

	case protocol.MatchesAny(prompt, "rightsiz", "right-siz", "cpu usage", "underutiliz",
		"vm recommendation", "downsize", "less than 50", "lesser cost",
		"advisor", "recommendation"):
		return a.handleRightsizing(ctx, emit)

	case protocol.MatchesAny(prompt, "idle", "unused", "not use", "orphan",
		"waste", "unattached", "not used"):
		return a.handleIdleResources(ctx, emit)
	}

	// --- Default: static IaC cost estimation ---
	if !protocol.RequireIaC(req, emit, "cost estimation") {
		return nil
	}

	var total float64
	var items []costItem

	for _, res := range req.IaC.Resources {
		est := estimateResource(res)
		items = append(items, costItem{
			Name:    parser.ShortType(res.Type) + "." + res.Name,
			SKU:     est.sku,
			Monthly: est.monthly,
		})
		total += est.monthly
	}

	emit.SendMessage(fmt.Sprintf("## Estimated Monthly Cost: **$%.2f**\n\n", total))
	emit.SendMessage("| Resource | SKU | Monthly |\n|----------|-----|---------|\n")
	for _, it := range items {
		emit.SendMessage(fmt.Sprintf("| %s | %s | $%.2f |\n", it.Name, it.SKU, it.Monthly))
	}
	emit.SendMessage("\n")

	if a.enableLLM && a.llmClient != nil && req.Token != "" {
		a.enhanceWithLLM(ctx, req, items, total, emit)
	}

	return nil
}

type costItem struct {
	Name    string
	SKU     string
	Monthly float64
}

const costPrompt = `You are a senior Azure FinOps engineer. Given the IaC code and cost estimates below, provide:
1. Cost optimization recommendations (reserved instances, right-sizing, cheaper SKUs)
2. Potential hidden costs not reflected in the estimates (egress, storage transactions, IP addresses)
3. A monthly vs. annual cost comparison if reserved instances were used

Be specific. Reference actual resource names and SKUs. Use markdown. Keep it under 200 words.`

func (a *Agent) enhanceWithLLM(ctx context.Context, req protocol.AgentRequest, items []costItem, total float64, emit protocol.Emitter) {
	var sb strings.Builder
	sb.WriteString("## IaC Code\n```\n")
	if req.IaC != nil {
		sb.WriteString(req.IaC.RawCode)
	}
	sb.WriteString("\n```\n\n## Cost Estimates\n")
	sb.WriteString(fmt.Sprintf("Total: $%.2f/month\n", total))
	for _, it := range items {
		sb.WriteString(fmt.Sprintf("- %s (%s): $%.2f/month\n", it.Name, it.SKU, it.Monthly))
	}

	emit.SendMessage("\n#### AI Cost Optimization\n\n")
	messages := []llm.ChatMessage{{Role: llm.RoleUser, Content: sb.String()}}
	contentCh, errCh := a.llmClient.Stream(ctx, req.Token, costPrompt, messages)
	for content := range contentCh {
		emit.SendMessage(content)
	}
	if err := <-errCh; err != nil {
		emit.SendMessage(fmt.Sprintf("\n_LLM enhancement unavailable: %v_\n", err))
	}
	emit.SendMessage("\n\n")
}

type estimate struct {
	sku     string
	monthly float64
}

func estimateResource(res protocol.Resource) estimate {
	region := "eastus"
	if loc, ok := res.Properties["location"].(string); ok && loc != "" {
		region = loc
	}
	_ = region // region for future API lookups

	switch res.Type {
	case "azurerm_kubernetes_cluster":
		return estimateAKS(res)
	case "azurerm_virtual_machine", "azurerm_linux_virtual_machine", "azurerm_windows_virtual_machine":
		return estimateVM(res)
	case "azurerm_storage_account":
		return estimateStorage(res)
	case "azurerm_app_service_plan", "azurerm_service_plan":
		return estimateAppService(res)
	case "azurerm_container_registry":
		return estimateACR(res)
	case "azurerm_key_vault":
		return estimate{sku: "Standard", monthly: 3.00}
	case "azurerm_virtual_network", "azurerm_subnet", "azurerm_network_security_group":
		return estimate{sku: "N/A", monthly: 0}
	default:
		return estimate{sku: "Unknown", monthly: 0}
	}
}

const hoursPerMonth = 730

func estimateAKS(res protocol.Resource) estimate {
	vmSize := "Standard_D2s_v3"
	nodeCount := 3
	if pool, ok := res.Properties["default_node_pool"].(map[string]interface{}); ok {
		if s, ok := pool["vm_size"].(string); ok {
			vmSize = s
		}
		if c, ok := pool["node_count"].(int); ok {
			nodeCount = c
		}
	}
	hourly := vmPrice(vmSize)
	monthly := hourly*hoursPerMonth*float64(nodeCount) + 18.25
	return estimate{
		sku:     fmt.Sprintf("%dx %s", nodeCount, vmSize),
		monthly: monthly,
	}
}

func estimateVM(res protocol.Resource) estimate {
	vmSize := "Standard_D2s_v3"
	if s, ok := res.Properties["vm_size"].(string); ok {
		vmSize = s
	} else if s, ok := res.Properties["size"].(string); ok {
		vmSize = s
	}
	hourly := vmPrice(vmSize)
	if res.Type == "azurerm_windows_virtual_machine" {
		hourly *= 1.5
	}
	return estimate{sku: vmSize, monthly: hourly * hoursPerMonth}
}

func estimateStorage(res protocol.Resource) estimate {
	sku := "Standard_LRS"
	if rep, ok := res.Properties["account_replication_type"].(string); ok {
		sku = "Standard_" + rep
	}
	pricePerGB := storagePrices[sku]
	if pricePerGB == 0 {
		pricePerGB = 0.0184
	}
	return estimate{sku: sku, monthly: pricePerGB * 100}
}

func estimateAppService(res protocol.Resource) estimate {
	sku := "B1"
	if s, ok := res.Properties["sku_name"].(string); ok {
		sku = s
	}
	monthly := appServicePrices[sku]
	if monthly == 0 {
		monthly = 13.14
	}
	return estimate{sku: sku, monthly: monthly}
}

func estimateACR(res protocol.Resource) estimate {
	sku := "Basic"
	if s, ok := res.Properties["sku"].(string); ok {
		sku = s
	}
	monthly := acrPrices[sku]
	if monthly == 0 {
		monthly = 5.00
	}
	return estimate{sku: sku, monthly: monthly}
}

func vmPrice(sku string) float64 {
	if p, ok := vmSkuPrices[sku]; ok {
		return p
	}
	return 0.096
}

var vmSkuPrices = map[string]float64{
	"Standard_B1s": 0.0104, "Standard_B1ms": 0.0207,
	"Standard_B2s": 0.0416, "Standard_B2ms": 0.0832,
	"Standard_D2s_v3": 0.096, "Standard_D4s_v3": 0.192, "Standard_D8s_v3": 0.384,
	"Standard_D2s_v4": 0.096, "Standard_D4s_v4": 0.192, "Standard_D8s_v4": 0.384,
	"Standard_D2s_v5": 0.096, "Standard_D4s_v5": 0.192, "Standard_D8s_v5": 0.384,
	"Standard_E2s_v3": 0.126, "Standard_E4s_v3": 0.252, "Standard_E8s_v3": 0.504,
	"Standard_F2s_v2": 0.085, "Standard_F4s_v2": 0.169, "Standard_F8s_v2": 0.338,
}

var storagePrices = map[string]float64{
	"Standard_LRS": 0.0184, "Standard_GRS": 0.0368, "Standard_ZRS": 0.023,
	"Standard_GZRS": 0.0414, "Premium_LRS": 0.15, "Standard_RA-GRS": 0.046,
}

var appServicePrices = map[string]float64{
	"F1": 0, "D1": 9.49, "B1": 13.14, "B2": 26.28, "B3": 52.56,
	"S1": 69.35, "S2": 138.70, "S3": 277.40,
	"P1v2": 73.00, "P2v2": 146.00, "P3v2": 292.00,
	"P1v3": 95.63, "P2v3": 191.25, "P3v3": 382.50,
}

var acrPrices = map[string]float64{
	"Basic": 5.00, "Standard": 20.00, "Premium": 50.00,
}

// ========================================================================
// Live Azure Handlers
// ========================================================================

// azureCredential lazily creates an Azure credential from config.
func (a *Agent) azureCredential() error {
	if a.cfg == nil {
		return fmt.Errorf("Azure credentials not configured. Set AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET environment variables")
	}
	return nil
}

// handleCostBreakdown queries Azure Cost Management for subscription and
// resource-group level monthly cost breakdown for the full year 2026.
func (a *Agent) handleCostBreakdown(ctx context.Context, emit protocol.Emitter) error {
	if err := a.azureCredential(); err != nil {
		emit.SendMessage(fmt.Sprintf("**Error:** %v\n", err))
		return nil
	}

	cred, err := azure.NewCredential(a.cfg.AzureTenantID, a.cfg.AzureClientID, a.cfg.AzureClientSecret)
	if err != nil {
		emit.SendMessage(fmt.Sprintf("**Authentication error:** %v\n", err))
		return nil
	}

	emit.SendMessage("## Monthly Cost Breakdown — 2026\n\n")
	emit.SendMessage("_Querying Azure Cost Management API for Jan 2026 – Dec 2026..._\n\n")

	subs, err := azure.ListSubscriptions(ctx, cred)
	if err != nil {
		emit.SendMessage(fmt.Sprintf("**Error listing subscriptions:** %v\n", err))
		return nil
	}
	if len(subs) == 0 {
		emit.SendMessage("No accessible subscriptions found.\n")
		return nil
	}

	costClient, err := azure.NewCostClient(cred)
	if err != nil {
		emit.SendMessage(fmt.Sprintf("**Error creating cost client:** %v\n", err))
		return nil
	}

	// Full year 2026 range
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	now := time.Now().UTC()
	if to.After(now) {
		to = now
	}

	// --- Section 1: Subscription-level monthly summary ---
	emit.SendMessage("### Subscription Monthly Summary (2026)\n\n")
	emit.SendMessage("| Subscription | Name | Month | Cost | Currency |\n")
	emit.SendMessage("|-------------|------|-------|------|----------|\n")

	for _, sub := range subs {
		costs, err := costClient.QuerySubscriptionTotalCost(ctx, sub.ID, from, to)
		if err != nil {
			log.Printf("cost breakdown: subscription %s: %v", sub.ID, err)
			continue
		}
		sortSubCostsByMonth(costs)
		for _, c := range costs {
			emit.SendMessage(fmt.Sprintf("| %s | %s | %s | $%.2f | %s |\n",
				sub.ID, sub.DisplayName, c.Month, c.Cost, c.Currency))
		}
	}
	emit.SendMessage("\n")

	// --- Section 2: Resource group breakdown per subscription ---
	emit.SendMessage("### Resource Group Breakdown (2026)\n\n")

	var topRGs []azure.ResourceGroupCost

	for _, sub := range subs {
		rgCosts, err := costClient.QueryMonthlyCostByResourceGroup(ctx, sub.ID, from, to)
		if err != nil {
			log.Printf("cost breakdown: RG costs for %s: %v", sub.ID, err)
			continue
		}
		if len(rgCosts) == 0 {
			continue
		}

		sortRGCostsByMonth(rgCosts)

		emit.SendMessage(fmt.Sprintf("#### Subscription: %s (%s)\n\n", sub.DisplayName, sub.ID))
		emit.SendMessage("| Resource Group | Month | Cost | Currency |\n")
		emit.SendMessage("|---------------|-------|------|----------|\n")
		for _, rg := range rgCosts {
			emit.SendMessage(fmt.Sprintf("| %s | %s | $%.2f | %s |\n",
				rg.ResourceGroup, rg.Month, rg.Cost, rg.Currency))
		}
		emit.SendMessage("\n")
		topRGs = append(topRGs, rgCosts...)
	}

	// --- Section 3: Top 10 most expensive resource groups ---
	if len(topRGs) > 0 {
		sortRGsByCost(topRGs)
		limit := 10
		if len(topRGs) < limit {
			limit = len(topRGs)
		}
		emit.SendMessage("### Top 10 Most Expensive Resource Groups (2026)\n\n")
		emit.SendMessage("| # | Resource Group | Month | Cost | Currency |\n")
		emit.SendMessage("|---|---------------|-------|------|----------|\n")
		for i := 0; i < limit; i++ {
			emit.SendMessage(fmt.Sprintf("| %d | %s | %s | $%.2f | %s |\n",
				i+1, topRGs[i].ResourceGroup, topRGs[i].Month, topRGs[i].Cost, topRGs[i].Currency))
		}
		emit.SendMessage("\n")
	}

	emit.SendMessage("\n_Note: Azure cost data may have a 24–48 hour delay. Future months will show as data becomes available._\n")
	return nil
}

// envLookbackDays returns the CPU analysis lookback period (in days) based on
// the resource group name convention:
//
//	Production  (contains "-pr" or "-co"): 90 days
//	Development (contains "-de"):          60 days
//	Test/Sandbox (contains "-te" or "-sa"):30 days
//	Default (unmatched):                   30 days
func envLookbackDays(resourceGroup string) (int, string) {
	rg := strings.ToLower(resourceGroup)
	switch {
	case strings.Contains(rg, "-pr") || strings.Contains(rg, "-co"):
		return 90, "Production"
	case strings.Contains(rg, "-de"):
		return 60, "Development"
	case strings.Contains(rg, "-te") || strings.Contains(rg, "-sa"):
		return 30, "Test/Sandbox"
	default:
		return 30, "Other"
	}
}

// handleRightsizing fetches Azure Advisor Cost recommendations (right-sizing,
// shutdown, reserved instances) across all subscriptions.
func (a *Agent) handleRightsizing(ctx context.Context, emit protocol.Emitter) error {
	if err := a.azureCredential(); err != nil {
		emit.SendMessage(fmt.Sprintf("**Error:** %v\n", err))
		return nil
	}

	cred, err := azure.NewCredential(a.cfg.AzureTenantID, a.cfg.AzureClientID, a.cfg.AzureClientSecret)
	if err != nil {
		emit.SendMessage(fmt.Sprintf("**Authentication error:** %v\n", err))
		return nil
	}

	emit.SendMessage("## Azure Advisor — Right-Sizing Recommendations\n\n")
	emit.SendMessage("_Fetching Cost category recommendations from Azure Advisor..._\n\n")

	subs, err := azure.ListSubscriptions(ctx, cred)
	if err != nil {
		emit.SendMessage(fmt.Sprintf("**Error listing subscriptions:** %v\n", err))
		return nil
	}

	var allRecs []advisorRecWithSub

	for _, sub := range subs {
		recs, err := azure.ListAdvisorRecommendations(ctx, cred, sub.ID, "Cost")
		if err != nil {
			log.Printf("advisor: sub %s: %v", sub.ID, err)
			continue
		}
		for _, r := range recs {
			low := strings.ToLower(r.Problem + " " + r.Solution)
			if !strings.Contains(low, "right-size") && !strings.Contains(low, "underutilized") {
				continue
			}
			allRecs = append(allRecs, advisorRecWithSub{
				AdvisorRecommendation: r,
				SubscriptionName:      sub.DisplayName,
			})
		}
	}

	if len(allRecs) == 0 {
		emit.SendMessage("**No right-sizing recommendations found.** Your resources are well-sized.\n")
		return nil
	}

	emit.SendMessage(fmt.Sprintf("**%d right-sizing recommendation(s) found across %d subscription(s)**\n\n", len(allRecs), len(subs)))

	emit.SendMessage("| # | Impact | Resource | Type | Resource Group | Subscription | Problem | Solution | Potential Benefits |\n")
	emit.SendMessage("|---|--------|----------|------|---------------|-------------|---------|----------|--------------------|\n")

	for i, r := range allRecs {
		emit.SendMessage(fmt.Sprintf("| %d | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			i+1, r.Impact, r.ImpactedResource,
			shortType(r.ImpactedType), r.ResourceGroup,
			r.SubscriptionName,
			r.Problem, r.Solution, r.PotentialBenefits))
	}

	// Impact summary
	high, med, low := 0, 0, 0
	for _, r := range allRecs {
		switch r.Impact {
		case "High":
			high++
		case "Medium":
			med++
		case "Low":
			low++
		}
	}
	emit.SendMessage(fmt.Sprintf("\n**Summary:** %d High, %d Medium, %d Low impact\n", high, med, low))
	emit.SendMessage("\n_Recommendations sourced from Azure Advisor (Cost category)._\n")
	return nil
}

type advisorRecWithSub struct {
	azure.AdvisorRecommendation
	SubscriptionName string
}

// shortType extracts the last part of a resource type path for display.
func shortType(fullType string) string {
	parts := strings.Split(fullType, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return fullType
}

// extractSubscriptionFromResourceID pulls the subscription ID from a full resource ID.
func extractSubscriptionFromResourceID(resourceID string) string {
	parts := strings.Split(strings.ToLower(resourceID), "/")
	for i, p := range parts {
		if p == "subscriptions" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// handleIdleResources finds orphaned and idle resources across all subscriptions.
func (a *Agent) handleIdleResources(ctx context.Context, emit protocol.Emitter) error {
	if err := a.azureCredential(); err != nil {
		emit.SendMessage(fmt.Sprintf("**Error:** %v\n", err))
		return nil
	}

	cred, err := azure.NewCredential(a.cfg.AzureTenantID, a.cfg.AzureClientID, a.cfg.AzureClientSecret)
	if err != nil {
		emit.SendMessage(fmt.Sprintf("**Authentication error:** %v\n", err))
		return nil
	}

	emit.SendMessage("## Idle & Orphaned Resources\n\n")
	emit.SendMessage("_Scanning all subscriptions via Azure Resource Graph for orphaned resources..._\n\n")

	subs, err := azure.ListSubscriptions(ctx, cred)
	if err != nil {
		emit.SendMessage(fmt.Sprintf("**Error listing subscriptions:** %v\n", err))
		return nil
	}

	// Build subscription ID list and name map
	subIDs := make([]string, len(subs))
	subNames := make(map[string]string) // subscriptionID -> displayName
	for i, sub := range subs {
		subIDs[i] = sub.ID
		subNames[sub.ID] = sub.DisplayName
	}

	type idleWithSub struct {
		azure.IdleResource
		SubscriptionName string
	}

	// Detect orphaned resources via Resource Graph (all subs in one call)
	idle, err := azure.DetectIdleResources(ctx, cred, subIDs)
	if err != nil {
		emit.SendMessage(fmt.Sprintf("**Error scanning resources:** %v\n", err))
		return nil
	}

	var allIdle []idleWithSub
	for _, r := range idle {
		// Extract subscription ID from resource ID
		subID := extractSubscriptionFromResourceID(r.ResourceID)
		allIdle = append(allIdle, idleWithSub{
			IdleResource:     r,
			SubscriptionName: subNames[subID],
		})
	}

	// Also check idle VMs (CPU < 5% for 30 days) via Monitor metrics
	for _, sub := range subs {
		vms, err := azure.ListVMs(ctx, cred, sub.ID)
		if err != nil {
			log.Printf("idle: list vms for %s: %v", sub.ID, err)
			continue
		}
		for _, vm := range vms {
			avg, err := azure.GetVMCPUAverage(ctx, cred, vm.ResourceID, 30)
			if err != nil {
				continue
			}
			if avg < 5 {
				hourly := vmPrice(vm.VMSize)
				allIdle = append(allIdle, idleWithSub{
					IdleResource: azure.IdleResource{
						Name:           vm.Name,
						Type:           "Virtual Machine",
						ResourceGroup:  vm.ResourceGroup,
						ResourceID:     vm.ResourceID,
						Reason:         fmt.Sprintf("Avg CPU %.1f%% over 30 days", avg),
						EstMonthlyCost: hourly * hoursPerMonth,
					},
					SubscriptionName: sub.DisplayName,
				})
			}
		}
	}

	if len(allIdle) == 0 {
		emit.SendMessage("**No idle or orphaned resources detected.** Your environment looks clean.\n")
		return nil
	}

	emit.SendMessage(fmt.Sprintf("**%d orphaned/idle resource(s) found across %d subscription(s)**\n\n", len(allIdle), len(subs)))

	// Unified table sorted by type
	emit.SendMessage("| # | Resource Name | Type | Resource Group | Subscription | Reason | Est. Monthly Cost |\n")
	emit.SendMessage("|---|--------------|------|---------------|-------------|--------|-------------------|\n")

	var totalWaste float64
	for i, r := range allIdle {
		emit.SendMessage(fmt.Sprintf("| %d | %s | %s | %s | %s | %s | $%.2f |\n",
			i+1, r.Name, r.Type, r.ResourceGroup, r.SubscriptionName, r.Reason, r.EstMonthlyCost))
		totalWaste += r.EstMonthlyCost
	}

	emit.SendMessage(fmt.Sprintf("\n**Total estimated monthly waste: $%.2f**\n", totalWaste))

	// Summary by type
	typeCounts := map[string]int{}
	typeCosts := map[string]float64{}
	for _, r := range allIdle {
		typeCounts[r.Type]++
		typeCosts[r.Type] += r.EstMonthlyCost
	}
	emit.SendMessage("\n### Summary by Resource Type\n\n")
	emit.SendMessage("| Resource Type | Count | Est. Monthly Cost |\n")
	emit.SendMessage("|--------------|-------|-------------------|\n")
	for t, count := range typeCounts {
		emit.SendMessage(fmt.Sprintf("| %s | %d | $%.2f |\n", t, count, typeCosts[t]))
	}

	emit.SendMessage("\n_Recommendation: Review and delete/deallocate these resources to reduce waste._\n")
	return nil
}

// sortRGsByCost sorts resource group costs descending by cost (simple insertion sort).
func sortRGsByCost(rgs []azure.ResourceGroupCost) {
	for i := 1; i < len(rgs); i++ {
		for j := i; j > 0 && rgs[j].Cost > rgs[j-1].Cost; j-- {
			rgs[j], rgs[j-1] = rgs[j-1], rgs[j]
		}
	}
}

// sortRGCostsByMonth sorts resource group costs ascending by month string.
func sortRGCostsByMonth(rgs []azure.ResourceGroupCost) {
	sort.Slice(rgs, func(i, j int) bool {
		if rgs[i].Month == rgs[j].Month {
			return rgs[i].ResourceGroup < rgs[j].ResourceGroup
		}
		return rgs[i].Month < rgs[j].Month
	})
}

// sortSubCostsByMonth sorts subscription costs ascending by month string.
func sortSubCostsByMonth(costs []azure.SubscriptionCost) {
	sort.Slice(costs, func(i, j int) bool {
		return costs[i].Month < costs[j].Month
	})
}
