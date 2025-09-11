# Log Backup Retry Monitoring and Alerting

This document provides monitoring and alerting configurations for the TiDB log backup retry feature.

## Prometheus Metrics

The log backup retry feature exposes the following metrics through the TiDB Operator controller:

### Controller Metrics

```prometheus
# Log backup retry attempts counter
tidb_operator_log_backup_retry_total{namespace="default",backup="demo-backup",reason="JobFailed"}

# Log backup retry success/failure counter  
tidb_operator_log_backup_retry_result_total{namespace="default",backup="demo-backup",result="success"}

# Log backup retry duration histogram
tidb_operator_log_backup_retry_duration_seconds{namespace="default",backup="demo-backup"}

# Current retry attempts gauge
tidb_operator_log_backup_retry_current{namespace="default",backup="demo-backup"}

# Log backup retry exhausted counter
tidb_operator_log_backup_retry_exhausted_total{namespace="default",backup="demo-backup"}

# Log backup retry timeout counter
tidb_operator_log_backup_retry_timeout_total{namespace="default",backup="demo-backup"}
```

### Kubernetes Events Metrics

Monitor retry events through kube-state-metrics:

```prometheus
# Retry started events
kube_event_count{reason="RetryStarted",object="backup"}

# Retry exhausted events  
kube_event_count{reason="RetryExhausted",object="backup"}

# Backup failed events
kube_event_count{reason="BackupFailed",object="backup"}
```

## Grafana Dashboard Configuration

### Log Backup Retry Overview Panel

```json
{
  "dashboard": {
    "title": "TiDB Log Backup Retry",
    "panels": [
      {
        "title": "Retry Rate",
        "type": "stat",
        "targets": [
          {
            "expr": "rate(tidb_operator_log_backup_retry_total[5m])",
            "legendFormat": "{{backup}}"
          }
        ]
      },
      {
        "title": "Success Rate",
        "type": "stat", 
        "targets": [
          {
            "expr": "rate(tidb_operator_log_backup_retry_result_total{result=\"success\"}[5m]) / rate(tidb_operator_log_backup_retry_total[5m])",
            "legendFormat": "Success Rate"
          }
        ]
      },
      {
        "title": "Current Retries by Backup",
        "type": "table",
        "targets": [
          {
            "expr": "tidb_operator_log_backup_retry_current",
            "legendFormat": "{{namespace}}/{{backup}}"
          }
        ]
      },
      {
        "title": "Retry Duration Distribution",
        "type": "heatmap",
        "targets": [
          {
            "expr": "rate(tidb_operator_log_backup_retry_duration_seconds_bucket[5m])",
            "legendFormat": "{{le}}"
          }
        ]
      },
      {
        "title": "Exhausted Retries",
        "type": "stat",
        "targets": [
          {
            "expr": "increase(tidb_operator_log_backup_retry_exhausted_total[1h])",
            "legendFormat": "Exhausted (1h)"
          }
        ]
      }
    ]
  }
}
```

### Log Backup Health Panel

```json
{
  "title": "Log Backup Health",
  "panels": [
    {
      "title": "Failed Backups Requiring Attention",
      "type": "table",
      "targets": [
        {
          "expr": "tidb_operator_log_backup_retry_current > 3",
          "legendFormat": "{{namespace}}/{{backup}}"
        }
      ]
    },
    {
      "title": "Backup Events Timeline", 
      "type": "logs",
      "targets": [
        {
          "expr": "kube_event_count{reason=~\"RetryStarted|RetryExhausted|BackupFailed\",object=\"backup\"}"
        }
      ]
    }
  ]
}
```

## Alerting Rules

### Prometheus AlertManager Rules

```yaml
# /etc/prometheus/rules/tidb-log-backup-retry.yml
groups:
- name: tidb-log-backup-retry
  rules:
  
  # High retry rate alert
  - alert: LogBackupHighRetryRate
    expr: rate(tidb_operator_log_backup_retry_total[5m]) > 0.1
    for: 2m
    labels:
      severity: warning
      component: tidb-operator
      feature: log-backup-retry
    annotations:
      summary: "High log backup retry rate detected"
      description: "Log backup {{ $labels.namespace }}/{{ $labels.backup }} is retrying at {{ $value }} attempts per second for the last 5 minutes."
      runbook_url: "https://docs.pingcap.com/tidb-in-kubernetes/stable/backup-restore-troubleshooting"

  # Low retry success rate alert  
  - alert: LogBackupLowRetrySuccessRate
    expr: |
      (
        rate(tidb_operator_log_backup_retry_result_total{result="success"}[10m]) /
        rate(tidb_operator_log_backup_retry_total[10m])
      ) < 0.5
    for: 5m
    labels:
      severity: warning
      component: tidb-operator  
      feature: log-backup-retry
    annotations:
      summary: "Low log backup retry success rate"
      description: "Log backup {{ $labels.namespace }}/{{ $labels.backup }} has a retry success rate of {{ $value | humanizePercentage }} over the last 10 minutes."

  # Retry exhaustion alert
  - alert: LogBackupRetriesExhausted
    expr: increase(tidb_operator_log_backup_retry_exhausted_total[1h]) > 0
    for: 0m
    labels:
      severity: critical
      component: tidb-operator
      feature: log-backup-retry
    annotations:
      summary: "Log backup retries exhausted"
      description: "Log backup {{ $labels.namespace }}/{{ $labels.backup }} has exhausted all retry attempts. Manual intervention required."
      runbook_url: "https://docs.pingcap.com/tidb-in-kubernetes/stable/backup-restore-troubleshooting"

  # Persistent retry failures
  - alert: LogBackupPersistentRetryFailure
    expr: tidb_operator_log_backup_retry_current > 3
    for: 15m
    labels:
      severity: warning
      component: tidb-operator
      feature: log-backup-retry
    annotations:
      summary: "Log backup persistent retry failure"
      description: "Log backup {{ $labels.namespace }}/{{ $labels.backup }} has been retrying for over 15 minutes ({{ $value }} attempts). Check for underlying issues."

  # Retry timeout alert
  - alert: LogBackupRetryTimeout
    expr: increase(tidb_operator_log_backup_retry_timeout_total[1h]) > 0
    for: 0m
    labels:
      severity: critical
      component: tidb-operator
      feature: log-backup-retry  
    annotations:
      summary: "Log backup retry timeout"
      description: "Log backup {{ $labels.namespace }}/{{ $labels.backup }} has timed out after exceeding the configured retry timeout duration."

  # No retry progress alert
  - alert: LogBackupNoRetryProgress
    expr: |
      (
        tidb_operator_log_backup_retry_current > 0
      ) and on(namespace,backup) (
        increase(tidb_operator_log_backup_retry_total[30m]) == 0
      )
    for: 30m
    labels:
      severity: warning
      component: tidb-operator
      feature: log-backup-retry
    annotations:
      summary: "Log backup retry stalled"  
      description: "Log backup {{ $labels.namespace }}/{{ $labels.backup }} has pending retries but no retry attempts in the last 30 minutes. Controller may be stuck."
```

### AlertManager Configuration

```yaml
# /etc/alertmanager/config.yml
route:
  group_by: ['alertname', 'component', 'feature']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 1h
  receiver: 'tidb-backup-alerts'
  routes:
  - match:
      component: tidb-operator
      feature: log-backup-retry
    receiver: 'tidb-backup-retry-alerts'

receivers:
- name: 'tidb-backup-retry-alerts'
  slack_configs:
  - api_url: 'YOUR_SLACK_WEBHOOK_URL'
    channel: '#tidb-alerts'
    title: 'TiDB Log Backup Retry Alert'
    text: |
      {{ range .Alerts }}
      *Alert:* {{ .Annotations.summary }}
      *Backup:* {{ .Labels.namespace }}/{{ .Labels.backup }}
      *Severity:* {{ .Labels.severity }}
      *Description:* {{ .Annotations.description }}
      {{ if .Annotations.runbook_url }}*Runbook:* {{ .Annotations.runbook_url }}{{ end }}
      {{ end }}
  email_configs:
  - to: 'tidb-ops@company.com'
    subject: 'TiDB Log Backup Retry Alert - {{ .GroupLabels.alertname }}'
    body: |
      {{ range .Alerts }}
      Alert: {{ .Annotations.summary }}
      Backup: {{ .Labels.namespace }}/{{ .Labels.backup }}
      Severity: {{ .Labels.severity }}
      Description: {{ .Annotations.description }}
      {{ end }}

- name: 'tidb-backup-alerts'
  # Default backup alert handling
```

## Monitoring Best Practices

### 1. Set Appropriate Thresholds

```yaml
# Adjust thresholds based on your environment
# Development: Lower thresholds for early detection
# Production: Higher thresholds to avoid alert noise

# High retry rate threshold (per second)
high_retry_rate_threshold: 0.1      # Dev: 0.05, Prod: 0.2

# Low success rate threshold (percentage)  
low_success_rate_threshold: 0.5     # Dev: 0.7, Prod: 0.3

# Persistent retry duration threshold
persistent_retry_duration: 15m      # Dev: 5m, Prod: 30m
```

### 2. Dashboard Layout Recommendations

```
┌─────────────────┬─────────────────┐
│   Retry Rate    │  Success Rate   │
│     (Stat)      │     (Stat)      │  
├─────────────────┼─────────────────┤
│  Current Retries by Backup        │
│           (Table)                 │
├───────────────────────────────────┤  
│     Retry Duration Distribution   │
│           (Heatmap)              │
├─────────────────┬─────────────────┤
│ Exhausted (1h)  │  Timeouts (1h)  │
│     (Stat)      │     (Stat)      │
└─────────────────┴─────────────────┘
```

### 3. Log Aggregation Queries

```bash
# Kubectl queries for troubleshooting
kubectl logs -l app.kubernetes.io/component=controller-manager -n tidb-operator \
  | grep "log backup.*retry" \
  | tail -50

# Event monitoring  
kubectl get events --sort-by='.lastTimestamp' -A \
  | grep -E "(RetryStarted|RetryExhausted|BackupFailed)"

# Failed backup status
kubectl get backups -A -o json \
  | jq -r '.items[] | select(.status.backoffRetryStatus != null) | "\(.metadata.namespace)/\(.metadata.name): \(.status.backoffRetryStatus | length) retries"'
```

## Runbook Procedures

### 1. High Retry Rate Investigation

```bash
#!/bin/bash
# investigate-high-retry-rate.sh

NAMESPACE=$1
BACKUP_NAME=$2

echo "=== Investigating high retry rate for $NAMESPACE/$BACKUP_NAME ==="

# Check current backup status
kubectl get backup $BACKUP_NAME -n $NAMESPACE -o yaml

# Check recent events  
kubectl get events -n $NAMESPACE --field-selector involvedObject.name=$BACKUP_NAME \
  --sort-by='.lastTimestamp' | tail -20

# Check job and pod status
kubectl get jobs,pods -n $NAMESPACE -l backup=$BACKUP_NAME

# Check controller logs
kubectl logs -l app.kubernetes.io/component=controller-manager -n tidb-operator \
  | grep "$NAMESPACE/$BACKUP_NAME" | tail -20
```

### 2. Retry Exhaustion Response

```bash  
#!/bin/bash
# handle-retry-exhaustion.sh

NAMESPACE=$1
BACKUP_NAME=$2

echo "=== Handling retry exhaustion for $NAMESPACE/$BACKUP_NAME ==="

# Check failure reason
kubectl get backup $BACKUP_NAME -n $NAMESPACE \
  -o jsonpath='{.status.backoffRetryStatus[-1].originalReason}'

# Check storage connectivity
kubectl run test-storage --rm -it --image=busybox -- /bin/sh

# Reset retry counter (if issue is resolved)
kubectl patch backup $BACKUP_NAME -n $NAMESPACE --type='json' \
  -p='[{"op": "remove", "path": "/status/backoffRetryStatus"}]'
```

## Integration Examples

### PagerDuty Integration

```yaml
# AlertManager PagerDuty configuration
receivers:
- name: 'pagerduty-critical'
  pagerduty_configs:
  - service_key: 'YOUR_PAGERDUTY_SERVICE_KEY'
    description: 'TiDB Log Backup Critical: {{ .GroupLabels.alertname }}'
    details:
      backup: '{{ .Labels.namespace }}/{{ .Labels.backup }}'
      severity: '{{ .Labels.severity }}'
      description: '{{ .Annotations.description }}'
```

### Custom Webhook Integration

```yaml
# AlertManager webhook configuration
receivers:
- name: 'custom-webhook'
  webhook_configs:
  - url: 'https://your-webhook-endpoint.com/tidb-alerts'
    send_resolved: true
    http_config:
      bearer_token: 'YOUR_WEBHOOK_TOKEN'
```

This comprehensive monitoring setup provides:

- **Real-time visibility** into retry operations
- **Proactive alerting** for issues requiring attention  
- **Detailed dashboards** for operational insights
- **Automated runbooks** for common scenarios
- **Integration options** for existing monitoring stacks

Adjust thresholds and configurations based on your specific environment and operational requirements.