# AI Rule Auto Update Fix V5.6

## Problem

In intelligent anomaly detection, AI rule auto update can be enabled and the
high-frequency alert threshold can be reached, but alerts keep increasing after
the system claims to generate a conservative rule update.

## Root Cause Hypotheses

1. The background rule generation scheduler currently exits at service startup
   when the config is disabled. If an operator enables AI rule update later from
   the UI/API, no scheduler is running to evaluate the new config.
2. The scheduler uses fixed 10m/30m/60m windows instead of the configured
   `high_frequency_hours` window, so thresholds outside those windows are not
   evaluated as configured.
3. Automatic rule tightening updates the database rule content and status, but
   it does not push the updated rule to connected agents. Existing agents keep
   using the old broad rule, so alert volume does not drop.

## Design

1. Keep the AI rule auto-update scheduler alive after API server startup. Each
   tick reads the latest config and skips work while disabled.
2. Evaluate high-frequency alerts with the configured
   `thresholds.high_frequency_hours` and `thresholds.high_frequency_count`.
3. When `/api/v1/detection/rules/ai-rule-config` enables AI rule update, trigger one
   immediate configured-window scan in the background. Periodic scans continue
   to run every 10 minutes.
4. After AI tightening updates a rule to `experimental`, broadcast an
   incremental rule update through the existing server-to-agent path.

## Scope

Backend only:

- `api-server/internal/service/rule_generation_service.go`
- `api-server/internal/api/handler/detection_handler.go`
- focused Go tests for scheduler/config-window behavior and rule tightening

No frontend changes are required for this fix.

## Verification

1. Unit tests first:
   - configured scan uses `high_frequency_hours`;
   - disabled config returns no configured scan stats;
   - rule tightening persists a stricter condition and experimental status.
2. Build/test using the `aegis-build-test` skill:
   - `cd api-server && go test ./internal/service ./internal/api/handler`
   - `cd api-server && make build`
3. Service/API smoke with curl:
   - `curl -s http://localhost:8082/health`
   - `curl -s http://localhost:8082/api/v1/detection/rules/ai-rule-config`
   - `curl -s -X PUT http://localhost:8082/api/v1/detection/rules/ai-rule-config ...`
4. Short monitoring window:
   - inspect `api-server` logs for configured-window scan messages;
   - query alert counts by MITRE before/after the scan when data is available.
