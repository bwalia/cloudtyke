import json, sys
P = {"type": "prometheus", "uid": "prometheus"}
panels, y = [], 0
def nid(): return len(panels) + 1
def stat(title, expr, x, w=4, unit="none", decimals=None, thresholds=None):
    p = {"id": nid(), "type": "stat", "title": title, "datasource": P, "gridPos": {"x": x, "y": y, "w": w, "h": 4},
         "targets": [{"refId": "A", "expr": expr, "datasource": P}],
         "options": {"reduceOptions": {"calcs": ["lastNotNull"]}, "colorMode": "background", "graphMode": "area"},
         "fieldConfig": {"defaults": {"unit": unit, "color": {"mode": "thresholds"},
             "thresholds": {"mode": "absolute", "steps": thresholds or [{"color": "blue", "value": None}]}}, "overrides": []}}
    if decimals is not None: p["fieldConfig"]["defaults"]["decimals"] = decimals
    panels.append(p)
def ts(title, targets, x, w=12, h=8, unit="none", stack=False, desc=None):
    p = {"id": nid(), "type": "timeseries", "title": title, "datasource": P, "gridPos": {"x": x, "y": y, "w": w, "h": h},
         "targets": [{"refId": chr(65+i), "expr": e, "legendFormat": l, "datasource": P, "exemplar": True} for i, (e, l) in enumerate(targets)],
         "fieldConfig": {"defaults": {"unit": unit, "custom": {"fillOpacity": 18 if stack else 6, "lineWidth": 2,
             "stacking": {"mode": "normal" if stack else "none"}}}, "overrides": []},
         "options": {"legend": {"displayMode": "table", "placement": "right", "calcs": ["lastNotNull"]}, "tooltip": {"mode": "multi"}}}
    if desc: p["description"] = desc
    panels.append(p)
def row(title):
    global y
    panels.append({"id": nid(), "type": "row", "title": title, "collapsed": False, "gridPos": {"x": 0, "y": y, "w": 24, "h": 1}})
    y += 1

R = "[2m]"  # exports and scrapes both run every 5s; a 2-minute window smooths their aliasing
row("Payments · business KPIs"); 
stat("Payments / min", f"sum(rate(cpay_payments_total{R}))*60", 0, decimals=1)
stat("Authorisation rate", f'sum(rate(cpay_payments_total{{status!="declined"}}{R})) / sum(rate(cpay_payments_total{R}))', 4, unit="percentunit", decimals=1,
     thresholds=[{"color": "red", "value": None}, {"color": "orange", "value": 0.8}, {"color": "green", "value": 0.88}])
stat("GBP volume / hour", f'sum(rate(cpay_payment_volume_total{{currency="GBP"}}{R}))*3600/100', 8, unit="currencyGBP", decimals=0)
stat("Blocked by risk / min", f'sum(rate(cpay_payments_total{{decline_reason="risk_blocked"}}{R}))*60', 12, decimals=1,
     thresholds=[{"color": "green", "value": None}, {"color": "orange", "value": 1}, {"color": "red", "value": 3}])
stat("Acquirer p95", f"histogram_quantile(0.95, sum by (le) (rate(cpay_acquirer_latency_milliseconds_bucket{R})))", 16, unit="ms", decimals=0,
     thresholds=[{"color": "green", "value": None}, {"color": "orange", "value": 400}, {"color": "red", "value": 1000}])
stat("Chargebacks (1h)", "sum(increase(cpay_chargebacks_total[1h]))", 20, decimals=0,
     thresholds=[{"color": "green", "value": None}, {"color": "purple", "value": 1}])
y += 4
ts("Payments by outcome", [(f"sum by (status) (rate(cpay_payments_total{R}))*60", "{{status}}")], 0, unit="short", stack=True)
ts("Payments by merchant", [(f"sum by (merchant) (rate(cpay_payments_total{R}))*60", "{{merchant}}")], 12, unit="short", stack=True)
y += 8
ts("Decline reasons", [(f'sum by (decline_reason) (rate(cpay_payments_total{{status="declined"}}{R}))*60', "{{decline_reason}}")], 0, w=8, unit="short", stack=True)
ts("Risk decisions", [(f"sum by (decision) (rate(cpay_risk_decisions_total{R}))*60", "{{decision}}")], 8, w=8, unit="short", stack=True,
   desc="The card-testing attack against Tailspin Toys every few minutes shows up as a burst of 'block'.")
ts("Acquirer latency", [(f"histogram_quantile(0.5, sum by (le, acquirer) (rate(cpay_acquirer_latency_milliseconds_bucket{R})))", "p50 {{acquirer}}"),
                        (f"histogram_quantile(0.99, sum by (le, acquirer) (rate(cpay_acquirer_latency_milliseconds_bucket{R})))", "p99 {{acquirer}}")], 16, w=8, unit="ms")
y += 8
row("Services · RED metrics from traces (spanmetrics)")
ts("Requests / s by service", [(f'sum by (service_name) (rate(traces_span_metrics_duration_milliseconds_count{{span_kind="SPAN_KIND_SERVER"}}{R}))', "{{service_name}}")], 0, w=8, unit="reqps")
ts("Error rate by service", [(f'(sum by (service_name) (rate(traces_span_metrics_duration_milliseconds_count{{span_kind="SPAN_KIND_SERVER",status_code="STATUS_CODE_ERROR"}}{R})) or 0 * sum by (service_name) (rate(traces_span_metrics_duration_milliseconds_count{{span_kind="SPAN_KIND_SERVER"}}{R}))) / sum by (service_name) (rate(traces_span_metrics_duration_milliseconds_count{{span_kind="SPAN_KIND_SERVER"}}{R}))', "{{service_name}}")], 8, w=8, unit="percentunit")
ts("p95 latency by service", [(f'histogram_quantile(0.95, sum by (le, service_name) (rate(traces_span_metrics_duration_milliseconds_bucket{{span_kind="SPAN_KIND_SERVER"}}{R})))', "{{service_name}}")], 16, w=8, unit="ms")
y += 8
ts("Calls between services and to Azure / databases (service graph)", [(f"sum by (client, server) (rate(traces_service_graph_request_total{R}))", "{{client}} → {{server}}")], 0, w=12, unit="reqps")
ts("Outgoing calls to Azure data planes and databases", [(f'sum by (service_name, span_name) (rate(traces_span_metrics_duration_milliseconds_count{{span_kind="SPAN_KIND_CLIENT"}}{R}))', "{{service_name}} · {{span_name}}")], 12, w=12, unit="reqps",
   desc="Key Vault, Blob and Files calls (HTTP client spans) and PostgreSQL / MongoDB operations.")
y += 8
row("Money movement")
ts("Refunds and chargebacks / min", [(f"sum by (reason) (rate(cpay_refunds_total{R}))*60", "refund · {{reason}}"), (f"sum by (reason_code) (rate(cpay_chargebacks_total{R}))*60", "chargeback · {{reason_code}}")], 0, w=8, unit="short")
ts("Net settled to merchants (per batch run)", [("sum by (merchant, currency) (increase(cpay_settlement_net_total[5m]))/100", "{{merchant}} ({{currency}})")], 8, w=8, unit="none")
ts("Webhook deliveries", [(f"sum by (merchant, status) (rate(cpay_webhooks_total{R}))*60", "{{merchant}} · {{status}}")], 16, w=8, unit="short")
y += 8
row("Logs (Loki) · warnings and errors, linked to traces")
panels.append({"id": nid(), "type": "logs", "title": "Warnings and errors across Contoso Pay", "datasource": {"type": "loki", "uid": "loki"},
               "gridPos": {"x": 0, "y": y, "w": 24, "h": 10},
               "targets": [{"refId": "A", "datasource": {"type": "loki", "uid": "loki"}, "expr": '{service_namespace=~"contoso-pay|northwind-airways"} | severity_text=~"WARN|ERROR|FATAL|warn|error|fatal"'}],
               "options": {"showTime": True, "wrapLogMessage": True, "sortOrder": "Descending", "enableLogDetails": True}})
dash = {"uid": "contoso-pay", "title": "Contoso Pay · Payments (live)", "tags": ["contoso-pay", "opentelemetry"], "timezone": "browser",
        "refresh": "5s", "time": {"from": "now-15m", "to": "now"}, "schemaVersion": 39, "editable": True, "panels": panels,
        "links": [{"title": "Jaeger", "url": "http://localhost:4731", "type": "link", "targetBlank": True},
                  {"title": "Processor console", "url": "http://localhost:4703", "type": "link", "targetBlank": True}]}
json.dump(dash, open(sys.argv[1], "w"), indent=1)
for p in panels:
    if p["title"] == "Error rate by service":
        p["fieldConfig"]["defaults"].update({"min": 0, "softMax": 0.05})
print(len(panels), "panels")
