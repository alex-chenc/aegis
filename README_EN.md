# Aegis Intelligent Host Security System

[中文](README.md) | English

![AI Analysis](docs/screenshots/ui-refresh/ai_analysis.png)

![AI Trace](docs/screenshots/ui-refresh/ai_trace.png)

![Agent Mode](docs/screenshots/ui-refresh/agent.png)

## Overview

Aegis is a next-generation AI-native host security platform. The system deeply integrates LLM technology to achieve dynamic audit management of host configurations and vulnerabilities. Through continuous AI noise reduction and automated judgment, it builds a closed loop from precise protection to automated response. We are committed to creating a minimalist, intelligent server baseline automation management platform for DevOps and security engineers through the forward-looking technology of "model against model".

## Core Features

### Dual-mode Intelligent Security Console (V6.0)

The core change in V6.0 is that all major platform capabilities, including host
management, baseline checks, vulnerability remediation, script audit,
self-healing, real-time detection, alert tracing, blocking policies, asset
collection, and dynamic detection packages, can be launched, orchestrated, and
tracked through Agent mode using natural language. The original operation mode
is still available for precise inspection, manual configuration, and fine-grained
control.

- Simpler operations: users no longer need to remember every page entry or form
  path. They can describe the goal directly, such as "check SSH baselines for
  these hosts", "analyze whether this host was attacked", or "generate and
  dispatch a remediation script".
- Smoother workflows: the agent can break down a task, select tools, and connect
  host data, assets, vulnerabilities, baselines, alerts, Agent runtime evidence,
  and DetectionPackage capabilities, reducing back-and-forth page switching.
- More complete analysis: the agent actively gathers context around the goal and
  turns asset details, vulnerability risks, baseline results, alert evidence,
  process/network behavior, and historical task results into understandable
  conclusions.
- Safer execution: high-risk actions are still governed by `request_approval`,
  `whitelist`, and `full_access` permission modes. Blocking, remediation,
  signing, and enabling detection packages can explain their impact first and run
  only after approval.
- Traceable results: plans, tool calls, approval records, execution results, and
  errors are preserved as audit trails, making it easier to review why a decision
  was made, which tools were called, and which hosts were affected.

### Host and Agent Management

- Host list, Agent online status, host metadata, and runtime status management
- One-click Agent installation command generation and distribution
- Task dispatch and result reporting across API Server, Server, and Agent through gRPC

### Intelligent Baseline Check and Fix

- Supports uploading baseline documents in PDF, Word, YAML, Excel, TXT, and other formats
- Uses LLMs to automatically parse check rules, detection methods, and remediation suggestions
- Document MD5 deduplication, parsing progress tracking, and parsing status display
- Batch LLM generation of check scripts and fix scripts, with online viewing, editing, and version traceability
- Batch dispatch check or fix tasks by selecting multiple rules and hosts
- Task status, execution progress, output logs, task type filtering, and sorting
- Failed tasks support re-dispatching checks, one-click fix task creation, and remediation suggestion viewing

### Intelligent Vulnerability Check and Fix

- Collects host software inventory and calls LLMs to identify known CVE vulnerabilities
- Vulnerability list supports severity filtering, search, and time-based sorting
- Supports manually entering custom CVEs and calling LLMs to complete vulnerability details
- Automatically calls LLMs to generate POC verification scripts and confirm whether vulnerabilities truly exist
- Automatically calls LLMs to generate targeted remediation scripts, with batch dispatch and progress tracking

### Script Security Audit

- Unified script audit service covers baseline checks, baseline fixes, vulnerability fixes, POC verification, and self-healing scripts
- Command audit blacklist supports create, edit, enable/disable, delete, search, and batch operations
- Blacklist rules support regex matching, exact command matching, category tags, severity levels, and applicable script types
- Preset high-risk command rules cover file system destruction, permission abuse, network egress, reverse shells, and other risks
- Generated scripts go through blacklist audit first, then AI security audit
- When an audit fails, the failure reason is fed back to the LLM for regeneration, with up to 3 retries
- A second blacklist validation runs before task dispatch, and a final validation runs on the Agent side before execution
- Audit logs record script content, matched rules, AI analysis results, risk levels, and audit timelines

### Intelligent Self-healing

- Automatically triggers the LLM self-healing flow when checks or fixes fail
- Automatically analyzes error causes and generates repaired scripts
- Automatically retries repaired scripts and tracks self-healing status
- Self-healing scripts are included in the unified script security audit flow

### Real-time Threat Detection and Response

- Uses eBPF to collect process execution, file access, and network connection events in real time
- File events support sensitive path access monitoring; network events support IPv4, IPv6, source address, and destination port collection
- eBPF automatically adapts to ringbuf or perf buffer based on kernel capabilities
- Built-in Sigma matching on the Agent side supports process, file, and network event pre-filtering
- Alerts are automatically aggregated and deduplicated by host, process, rule, and MITRE technique
- Alert center supports filtering, search, detail view, and real-time WebSocket push
- Blocking policies support manual blocking, rule-hit auto blocking, and AI-analysis-based auto blocking by MITRE ATT&CK technique

### Dynamic eBPF Detection Package (V5.8)

- Dynamically generate eBPF detection programs for specific CVEs, extending Agent runtime detection capabilities
- Detection package management page: full visual workflow from draft editing, building, signing, to enabling and deployment
- Packages can only be built and enabled after admin review, ensuring detection code is safe and controllable
- Supports single-event rule matching and multi-event correlation detection, covering complex attack chain scenarios
- Plugin loading failures do not affect the Agent main process, ensuring host stability

### Intelligent Asset Collection (V5.8)

- Automatically collects installed software packages and running application processes on hosts
- Uses LLM to identify application types (databases, web services, web frameworks, web sites) and versions
- Provides asset classification views under the host list for quick understanding of each host's software assets
- Provides accurate asset context for vulnerability scanning, improving vulnerability matching accuracy
- Supports scheduled automatic collection and manual triggered collection

### AI Alert Analysis and Attack Trace

- Supports multi-alert AI analysis by time range, host, and selected alerts
- Uses a ReAct agent for execution planning, tool calls, reflection, auditing, correction, and summarization
- Streams reasoning, tool calls, observations, execution plans, and final conclusions through SSE
- Historical sessions can restore analysis process, conclusions, disposal suggestions, and execution results
- Supports attack trace graphs, attack flowchart images, and structured disposal suggestions
- Supports context compression, batch event analysis, and observability for large-context analysis

### System Configuration and Observability

- Model configuration supports separate text model and image model settings, connection tests, and secure saving
- When no model has been configured for the first time, the page shows an editable empty state instead of reporting it as a system error
- Supports OpenAI-compatible models, DashScope, and other LLM services
- Audit log page displays total audit count, pass rate, failure count, retry distribution, and detail drawer
- Agent analysis records iteration count, tool call count, failure rate, session duration, and token usage


## Quick Start

### Requirements

- Docker 20.10+
- Docker Compose 2.0+
- 2GB+ available memory

### Source Code Deployment

```bash
# 1. Clone the repository
git clone <repository-url>
cd aegis

# 2. Configure environment variables
cp .env.example .env
vim .env

# 3. Build and start
docker compose up -d --build

# 4. Verify service
curl http://localhost:8082/health
```

Visit http://localhost:8081 in your browser.

### One-click Deployment (Offline Package)

```bash
# 1. Extract the offline release package
unzip aegis-v6.0-linux-amd64-release.zip
cd v6.0

# 2. Run the deployment script (auto-detects IP, loads images, starts services)
bash start.sh
```

Visit http://<host-ip>:8081 in your browser.

### Configure LLM

For first-time use, configure the LLM service on the "System Configuration / Model Configuration" page:

1. Go to "System Configuration / Model Configuration"
2. Enter API Key and Base URL (supports Alibaba Cloud DashScope, OpenAI, etc.)
3. Click "Connection Test" to verify configuration
4. Save configuration

### Deploy Agent

Get the Agent installation command from "System Configuration / Agent Installation" and execute it on the target server:

```bash
curl -sSL http://<SERVER_IP>:8082/api/v1/agent/install.sh | sudo bash
```


## Port Reference

| Service | Port |
| :------ | :--- |
| Frontend | 8081 |
| API Server HTTP | 8082 |
| API Server gRPC | 19093 |
| Server gRPC | 19090 / 19094 |
| DC gRPC | 19092 |
| Builder gRPC | 19096 |
| PostgreSQL | 5432 |
| Redis | 6379 |
| MinIO API | 9000 |
| MinIO Console | 9001 |
| Kafka | 29092 |


## Fully AI-designed, developed, and tested project
