// Package activitylog records control-plane write, delete and action
// operations, served through the Microsoft.Insights eventtypes/management
// API like the Azure Activity Log.
package activitylog

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// Event is an activity log entry (subset of the Azure schema).
type Event struct {
	Authorization       map[string]any `json:"authorization"`
	Caller              string         `json:"caller"`
	Channels            string         `json:"channels"`
	Claims              map[string]any `json:"claims,omitempty"`
	CorrelationID       string         `json:"correlationId"`
	Description         string         `json:"description"`
	EventDataID         string         `json:"eventDataId"`
	EventName           LocalizedValue `json:"eventName"`
	Category            LocalizedValue `json:"category"`
	HTTPRequest         map[string]any `json:"httpRequest"`
	ID                  string         `json:"id"`
	Level               string         `json:"level"`
	ResourceGroupName   string         `json:"resourceGroupName,omitempty"`
	ResourceProvider    LocalizedValue `json:"resourceProviderName"`
	ResourceID          string         `json:"resourceId"`
	ResourceType        LocalizedValue `json:"resourceType"`
	OperationID         string         `json:"operationId"`
	OperationName       LocalizedValue `json:"operationName"`
	Properties          map[string]any `json:"properties"`
	Status              LocalizedValue `json:"status"`
	SubStatus           LocalizedValue `json:"subStatus"`
	EventTimestamp      string         `json:"eventTimestamp"`
	SubmissionTimestamp string         `json:"submissionTimestamp"`
	SubscriptionID      string         `json:"subscriptionId"`
	TenantID            string         `json:"tenantId"`
}

// LocalizedValue mirrors Azure's {value, localizedValue}.
type LocalizedValue struct {
	Value          string `json:"value"`
	LocalizedValue string `json:"localizedValue"`
}

func lv(s string) LocalizedValue { return LocalizedValue{Value: s, LocalizedValue: s} }

// Log appends and queries events.
type Log struct{ st store.Store }

// New creates the activity log.
func New(st store.Store) *Log { return &Log{st: st} }

// Record stores an event.
func (l *Log) Record(e Input) {
	now := time.Now().UTC()
	status := "Succeeded"
	if e.StatusCode >= 400 {
		status = "Failed"
	} else if e.StatusCode == 202 {
		status = "Accepted"
	}
	id := entra.NewGUID()
	ev := Event{
		Authorization:       map[string]any{"action": e.Action, "scope": e.ResourceID},
		Caller:              e.Caller,
		Channels:            "Operation",
		CorrelationID:       e.CorrelationID,
		EventDataID:         id,
		EventName:           lv("EndRequest"),
		Category:            lv("Administrative"),
		HTTPRequest:         map[string]any{"clientRequestId": e.ClientRequestID, "clientIpAddress": e.ClientIP, "method": e.Method},
		ID:                  e.ResourceID + "/events/" + id + "/ticks/" + fmt.Sprint(now.UnixNano()/100),
		Level:               "Informational",
		ResourceGroupName:   e.ResourceGroup,
		ResourceProvider:    lv(e.Namespace),
		ResourceID:          e.ResourceID,
		ResourceType:        lv(e.ResourceType),
		OperationID:         e.CorrelationID,
		OperationName:       lv(e.Action),
		Properties:          map[string]any{"statusCode": fmt.Sprint(e.StatusCode), "serviceRequestId": e.RequestID},
		Status:              lv(status),
		SubStatus:           lv(fmt.Sprint(e.StatusCode)),
		EventTimestamp:      now.Format(time.RFC3339Nano),
		SubmissionTimestamp: now.Format(time.RFC3339Nano),
		SubscriptionID:      e.SubscriptionID,
		TenantID:            e.TenantID,
	}
	if e.StatusCode >= 400 {
		ev.Level = "Error"
	}
	key := fmt.Sprintf("%s/%020d/%s", strings.ToLower(e.SubscriptionID), now.UnixNano(), id)
	_ = store.PutJSON(l.st, store.BucketActivity, key, ev)
}

// Input carries the data for a new event.
type Input struct {
	Action, Caller, CorrelationID, ClientRequestID, ClientIP, Method string
	ResourceID, ResourceGroup, Namespace, ResourceType               string
	SubscriptionID, TenantID, RequestID                              string
	StatusCode                                                       int
}

// Query returns events for a subscription matching the simple filter.
func (l *Log) Query(subscriptionID string, f Filter) []Event {
	var out []Event
	_ = l.st.Scan(store.BucketActivity, strings.ToLower(subscriptionID)+"/", func(_ string, v []byte) bool {
		var e Event
		if json.Unmarshal(v, &e) != nil {
			return true
		}
		if f.matches(e) {
			out = append(out, e)
		}
		return true
	})
	// newest first, as Azure returns
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Filter is the parsed $filter of an activity log query.
type Filter struct {
	From, To       time.Time
	ResourceGroup  string
	ResourceURI    string
	ResourceProv   string
	CorrelationID  string
	Caller         string
	StatusFailures bool
}

func (f Filter) matches(e Event) bool {
	t, _ := time.Parse(time.RFC3339Nano, e.EventTimestamp)
	if !f.From.IsZero() && t.Before(f.From) {
		return false
	}
	if !f.To.IsZero() && t.After(f.To) {
		return false
	}
	if f.ResourceGroup != "" && !strings.EqualFold(f.ResourceGroup, e.ResourceGroupName) {
		return false
	}
	if f.ResourceURI != "" && !strings.EqualFold(f.ResourceURI, e.ResourceID) {
		return false
	}
	if f.ResourceProv != "" && !strings.EqualFold(f.ResourceProv, e.ResourceProvider.Value) {
		return false
	}
	if f.CorrelationID != "" && !strings.EqualFold(f.CorrelationID, e.CorrelationID) {
		return false
	}
	if f.Caller != "" && !strings.EqualFold(f.Caller, e.Caller) {
		return false
	}
	return true
}

// ParseFilter parses expressions like
// "eventTimestamp ge '2024-01-01T00:00:00Z' and resourceGroupName eq 'rg'".
func ParseFilter(s string) Filter {
	var f Filter
	for _, clause := range splitAnd(s) {
		parts := strings.SplitN(strings.TrimSpace(clause), " ", 3)
		if len(parts) != 3 {
			continue
		}
		field, op, val := strings.ToLower(parts[0]), strings.ToLower(parts[1]), strings.Trim(strings.TrimSpace(parts[2]), "'")
		switch field {
		case "eventtimestamp":
			t, err := time.Parse(time.RFC3339Nano, val)
			if err != nil {
				continue
			}
			if op == "ge" || op == "gt" {
				f.From = t
			} else {
				f.To = t
			}
		case "resourcegroupname":
			f.ResourceGroup = val
		case "resourceuri", "resourceid":
			f.ResourceURI = val
		case "resourceprovider":
			f.ResourceProv = val
		case "correlationid":
			f.CorrelationID = val
		case "caller":
			f.Caller = val
		}
	}
	return f
}

func splitAnd(s string) []string {
	var out []string
	lower := strings.ToLower(s)
	for {
		i := strings.Index(lower, " and ")
		if i < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:i])
		s, lower = s[i+5:], lower[i+5:]
	}
}
