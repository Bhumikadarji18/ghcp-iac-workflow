// Package dashboard provides REST API endpoints that return JSON data
// for the governance dashboard UI.
package dashboard

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/azure"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/config"
)

// Handler holds the dependencies for dashboard API endpoints.
type Handler struct {
	cfg *config.Config
}

// New creates a new dashboard Handler.
func New(cfg *config.Config) *Handler {
	return &Handler{cfg: cfg}
}

// Register mounts all dashboard API routes on the given mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/dashboard/costs", h.handleCosts)
	mux.HandleFunc("GET /api/dashboard/rightsizing", h.handleRightsizing)
	mux.HandleFunc("GET /api/dashboard/idle", h.handleIdle)
	mux.HandleFunc("GET /dashboard", h.serveDashboard)
}

func (h *Handler) cred() (azcore.TokenCredential, error) {
	return azure.NewCredential(h.cfg.AzureTenantID, h.cfg.AzureClientID, h.cfg.AzureClientSecret)
}

// --- Unified Costs Endpoint (sub-wise + RG-wise, monthly) ---

type CostsResponse struct {
	Year          int           `json:"year"`
	Months        []string      `json:"months"`
	Subscriptions []SubCostData `json:"subscriptions"`
}

type SubCostData struct {
	Name           string             `json:"name"`
	ID             string             `json:"id"`
	Currency       string             `json:"currency"`
	MonthlyTotals  map[string]float64 `json:"monthlyTotals"`
	YTDTotal       float64            `json:"ytdTotal"`
	ResourceGroups []RGCostData       `json:"resourceGroups"`
}

type RGCostData struct {
	Name          string             `json:"name"`
	MonthlyTotals map[string]float64 `json:"monthlyTotals"`
	YTDTotal      float64            `json:"ytdTotal"`
}

func (h *Handler) handleCosts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()

	cred, err := h.cred()
	if err != nil {
		jsonError(w, "auth error", 500)
		return
	}

	now := time.Now().UTC()
	from := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)

	subs, err := azure.ListSubscriptions(ctx, cred)
	if err != nil {
		jsonError(w, "list subs: "+err.Error(), 500)
		return
	}

	costClient, err := azure.NewCostClient(cred)
	if err != nil {
		jsonError(w, "cost client: "+err.Error(), 500)
		return
	}

	monthSet := map[string]bool{}
	var subResults []SubCostData

	for _, sub := range subs {
		// Query RG-level costs (includes subscription total when aggregated)
		rgCosts, err := costClient.QueryMonthlyCostByResourceGroup(ctx, sub.ID, from, now)
		if err != nil {
			log.Printf("dashboard: costs %s: %v", sub.ID, err)
			continue
		}

		// Aggregate per RG
		type rgKey struct{ name, month string }
		rgMap := map[string]map[string]float64{}
		subMonthly := map[string]float64{}
		currency := "USD"

		for _, rc := range rgCosts {
			if rc.Currency != "" {
				currency = rc.Currency
			}
			rgName := rc.ResourceGroup
			if rgName == "" {
				rgName = "(no resource group)"
			}
			if _, ok := rgMap[rgName]; !ok {
				rgMap[rgName] = map[string]float64{}
			}
			rgMap[rgName][rc.Month] += rc.Cost
			subMonthly[rc.Month] += rc.Cost
			monthSet[rc.Month] = true
		}

		// Build RG list
		var rgs []RGCostData
		for rgName, months := range rgMap {
			ytd := 0.0
			for _, c := range months {
				ytd += c
			}
			rgs = append(rgs, RGCostData{Name: rgName, MonthlyTotals: months, YTDTotal: ytd})
		}
		sort.Slice(rgs, func(i, j int) bool { return rgs[i].YTDTotal > rgs[j].YTDTotal })

		ytd := 0.0
		for _, c := range subMonthly {
			ytd += c
		}

		subResults = append(subResults, SubCostData{
			Name: sub.DisplayName, ID: sub.ID, Currency: currency,
			MonthlyTotals: subMonthly, YTDTotal: ytd,
			ResourceGroups: rgs,
		})
	}

	sort.Slice(subResults, func(i, j int) bool { return subResults[i].YTDTotal > subResults[j].YTDTotal })

	// Sorted month list
	months := make([]string, 0, len(monthSet))
	for m := range monthSet {
		months = append(months, m)
	}
	sort.Strings(months)

	writeJSON(w, CostsResponse{
		Year:          now.Year(),
		Months:        months,
		Subscriptions: subResults,
	})
}

// --- Rightsizing ---

type RightsizingResponse struct {
	Total           int               `json:"total"`
	Recommendations []RightsizingItem `json:"recommendations"`
	ByImpact        map[string]int    `json:"byImpact"`
	BySub           map[string]int    `json:"bySubscription"`
}

type RightsizingItem struct {
	Resource     string `json:"resource"`
	Type         string `json:"type"`
	RG           string `json:"resourceGroup"`
	Subscription string `json:"subscription"`
	Impact       string `json:"impact"`
	Problem      string `json:"problem"`
	Solution     string `json:"solution"`
}

func (h *Handler) handleRightsizing(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	cred, err := h.cred()
	if err != nil {
		jsonError(w, "auth error", 500)
		return
	}

	subs, err := azure.ListSubscriptions(ctx, cred)
	if err != nil {
		jsonError(w, "list subs: "+err.Error(), 500)
		return
	}

	var items []RightsizingItem
	byImpact := map[string]int{}
	bySub := map[string]int{}

	for _, sub := range subs {
		recs, err := azure.ListAdvisorRecommendations(ctx, cred, sub.ID, "Cost")
		if err != nil {
			log.Printf("dashboard: advisor %s: %v", sub.ID, err)
			continue
		}
		for _, r := range recs {
			low := strings.ToLower(r.Problem + " " + r.Solution)
			if !strings.Contains(low, "right-size") && !strings.Contains(low, "underutilized") {
				continue
			}
			items = append(items, RightsizingItem{
				Resource:     r.ImpactedResource,
				Type:         shortType(r.ImpactedType),
				RG:           r.ResourceGroup,
				Subscription: sub.DisplayName,
				Impact:       r.Impact,
				Problem:      r.Problem,
				Solution:     r.Solution,
			})
			byImpact[r.Impact]++
			bySub[sub.DisplayName]++
		}
	}

	writeJSON(w, RightsizingResponse{
		Total:           len(items),
		Recommendations: items,
		ByImpact:        byImpact,
		BySub:           bySub,
	})
}

// --- Idle Resources ---

type IdleResponse struct {
	Total       int                `json:"total"`
	Resources   []IdleItem         `json:"resources"`
	ByType      map[string]int     `json:"byType"`
	BySub       map[string]int     `json:"bySubscription"`
	TotalWaste  float64            `json:"totalWaste"`
	WasteByType map[string]float64 `json:"wasteByType"`
}

type IdleItem struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	RG           string  `json:"resourceGroup"`
	Subscription string  `json:"subscription"`
	Reason       string  `json:"reason"`
	EstCost      float64 `json:"estMonthlyCost"`
}

func (h *Handler) handleIdle(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	cred, err := h.cred()
	if err != nil {
		jsonError(w, "auth error", 500)
		return
	}

	subs, err := azure.ListSubscriptions(ctx, cred)
	if err != nil {
		jsonError(w, "list subs: "+err.Error(), 500)
		return
	}

	subIDs := make([]string, len(subs))
	subNames := map[string]string{}
	for i, s := range subs {
		subIDs[i] = s.ID
		subNames[s.ID] = s.DisplayName
	}

	idle, err := azure.DetectIdleResources(ctx, cred, subIDs)
	if err != nil {
		jsonError(w, "detect idle: "+err.Error(), 500)
		return
	}

	var items []IdleItem
	byType := map[string]int{}
	bySub := map[string]int{}
	wasteByType := map[string]float64{}
	totalWaste := 0.0

	for _, r := range idle {
		subID := extractSubFromID(r.ResourceID)
		subName := subNames[subID]
		items = append(items, IdleItem{
			Name: r.Name, Type: r.Type, RG: r.ResourceGroup,
			Subscription: subName, Reason: r.Reason, EstCost: r.EstMonthlyCost,
		})
		byType[r.Type]++
		bySub[subName]++
		wasteByType[r.Type] += r.EstMonthlyCost
		totalWaste += r.EstMonthlyCost
	}

	writeJSON(w, IdleResponse{
		Total: len(items), Resources: items,
		ByType: byType, BySub: bySub,
		TotalWaste: totalWaste, WasteByType: wasteByType,
	})
}

// serveDashboard serves the HTML dashboard page.
func (h *Handler) serveDashboard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func shortType(t string) string {
	parts := strings.Split(t, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return t
}

func extractSubFromID(id string) string {
	parts := strings.Split(strings.ToLower(id), "/")
	for i, p := range parts {
		if p == "subscriptions" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
