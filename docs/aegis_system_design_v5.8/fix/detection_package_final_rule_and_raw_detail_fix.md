# Detection Package Final Rule And Raw Detail Fix

## Bug Description And Symptoms

Dynamic detection packages generated multiple similar Sigma rules in rule management for one CVE detection chain. For CVE-2026-31431, AF_ALG socket, AF_ALG bind, and splice-call steps appeared as separate rule-management entries even though they are intermediate detection steps. Block policies also showed extra entries because policies were reconciled from every visible Sigma rule. Package detail pages only exposed the editor for mutable draft states, so users could not inspect original HookPlan, eBPF, Sigma, and Correlation content after a package moved to build/sign/enable states.

## Reproduction Steps

1. Create or enable a CVE-2026-31431 dynamic detection package with multiple atomic Sigma steps and one Correlation final rule.
2. Open rule management and observe the atomic step rules listed as separate final rules.
3. Open block policies and observe policies/counts derived from the intermediate step rules.
4. Open a signed or enabled package detail page and observe that there is no stable place to inspect the original generated content.

## Root Cause Analysis

`DetectionPackageService.syncDetectionPackageRules` synchronized both atomic Sigma rules and Correlation rules into `sigma_rules`. `DetectionHandler.reconcileRulePolicyBindings` then listed all Sigma rules and created/kept block policies by MITRE ID, so intermediate detection steps polluted both rule management and blocking policy reconciliation. The package detail frontend only fetched draft-specific raw fields when navigating through editable states.

## Fix Design

- Treat Correlation as the only final detection rule that belongs in rule management.
- Keep atomic Sigma steps as package raw content, visible in detection package detail, not as standalone managed rules.
- Ignore `source = detection_package` atomic rules in default Sigma rule listing and block-policy rule-title/count queries.
- During package rule sync, delete old atomic package rules for the same package and upsert only final Correlation rules.
- During block-policy reconciliation, delete policies whose MITRE IDs no longer have a visible managed rule.
- Add a read-only raw information tab to package detail for HookPlan, eBPF source, Sigma atomic rules, and Correlation final rules.

## Code Changes

- `api-server/internal/service/detection_package_service.go`
  - Syncs only Correlation final rules into `sigma_rules`.
  - Deletes stale package atomic rules with `source = detection_package`.
  - Logs missing-final-rule and stale-atomic cleanup events.
- `api-server/internal/repository/sigma_rule_repo.go`
  - Default rule listing excludes package atomic step rules.
- `api-server/internal/repository/block_policy_repo.go`
  - Rule title, count, and search ignore package atomic step rules.
- `api-server/internal/api/handler/detection_handler.go`
  - Reconciliation now removes orphan block policies.
- `frontend/src/views/detection/DetectionPackages/PackageDetail.vue`
  - Adds the read-only raw information tab.
- `frontend/src/views/detection/DetectionPackages/components/CodeEditorPanel.vue`
  - Adds read-only support.

## Verification Steps

- Run focused api-server tests:
  - `go test ./internal/service ./internal/api/handler`
- Run frontend build:
  - `npm run build`

## Affected Components

- `api-server`: dynamic detection package rule sync, Sigma listing, block-policy reconciliation.
- `frontend`: dynamic detection package detail page.

## Risk And Rollback Plan

Risk is limited to dynamic-package atomic rules no longer appearing in rule management. The original atomic YAML remains in package drafts and is shown in package detail. Rollback by reverting the listed files; old behavior will again sync atomic package rules as managed Sigma rules.
