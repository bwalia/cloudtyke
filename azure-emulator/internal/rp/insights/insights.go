// Package insights implements the parts of Microsoft.Insights available in
// phase 1: the Activity Log query API.
package insights

import (
	"net/http"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/activitylog"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
)

// Register wires the RP.
func Register(s *arm.Server) {
	s.Registry.Register(&arm.TypeRegistration{
		Namespace: "Microsoft.Insights", Types: []string{"eventtypes", "values"},
		APIVersions: []string{"2017-03-01-preview", "2015-04-01"}, Scopes: []arm.ScopeKind{arm.KindSubscription}, Phase: 1,
		Notes:   "Activity Log: GET /subscriptions/{id}/providers/Microsoft.Insights/eventtypes/management/values with $filter on eventTimestamp, resourceGroupName, resourceUri, resourceProvider, correlationId, caller",
		Handler: func(q *arm.Request) { activity(s, q) },
	})
}

func activity(s *arm.Server, q *arm.Request) {
	if q.R.Method != http.MethodGet {
		q.Error(405, "MethodNotAllowed", "Only GET is supported.")
		return
	}
	sub := arm.ScopeSubscription(q.ID.Scope)
	if sub == "" || len(q.ID.Names) == 0 || q.ID.Names[0] != "management" {
		q.Error(400, "BadRequest", "Use /subscriptions/{subscriptionId}/providers/Microsoft.Insights/eventtypes/management/values.")
		return
	}
	events := s.Activity.Query(sub, activitylog.ParseFilter(q.R.URL.Query().Get("$filter")))
	items := make([]map[string]any, 0, len(events))
	for _, e := range events {
		items = append(items, map[string]any{
			"authorization": e.Authorization, "caller": e.Caller, "channels": e.Channels, "correlationId": e.CorrelationID,
			"description": e.Description, "eventDataId": e.EventDataID, "eventName": e.EventName, "category": e.Category,
			"httpRequest": e.HTTPRequest, "id": e.ID, "level": e.Level, "resourceGroupName": e.ResourceGroupName,
			"resourceProviderName": e.ResourceProvider, "resourceId": e.ResourceID, "resourceType": e.ResourceType,
			"operationId": e.OperationID, "operationName": e.OperationName, "properties": e.Properties, "status": e.Status,
			"subStatus": e.SubStatus, "eventTimestamp": e.EventTimestamp, "submissionTimestamp": e.SubmissionTimestamp,
			"subscriptionId": e.SubscriptionID, "tenantId": e.TenantID,
		})
	}
	// keep Azure's newest-first ordering (List would re-sort by id)
	q.JSON(200, map[string]any{"value": items})
}
