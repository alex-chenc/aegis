# Aegis Intelligent Host Security System

[中文](README.md) | English

![AI Analysis](docs/screenshots/ui-refresh/ai_analysis.png)

![AI Trace](docs/screenshots/ui-refresh/ai_trace.png)

## Overview

Aegis is a next-generation AI-native host security platform. The system deeply integrates LLM technology, using a natural-language AI assistant to orchestrate end-to-end security operations across baselines, vulnerabilities, assets, alerts, and weak passwords, achieving dynamic audit management of host configurations, vulnerabilities, and weak passwords. Through continuous AI noise reduction and automated judgment, it builds a closed loop from precise protection to automated response. We are committed to creating a minimalist, intelligent server baseline automation management platform for DevOps and security engineers through the forward-looking technology of "model against model".

## Core Features

### AI Security Assistant (V6.1)

Takes security-operation goals in natural language: the LLM understands intent while the backend decides tools and parameters, driving end-to-end workflows across hosts, assets, vulnerabilities, baselines, weak passwords and detection via a deterministic plan, with final answers bounded by real tool evidence.

- Describe targets and objects in natural language (host names, IPs, IP fragments, the current page object, etc.); the assistant resolves them to a unique real entity before any write, asks when ambiguous or offline, and never guesses a write target for you
- Decomposes each request into business goal, action, object, scope and missing info, automatically forming a multi-step plan for complex tasks instead of stopping at the first matching tool
- Backend capability contracts let the LLM only suggest candidate capabilities; final tool selection, argument binding and execution order are decided by the backend, so the model cannot bypass authorization to call or invent tools
- Multi-step closures are wrapped as high-level capabilities (baseline compliance, vulnerability assessment and remediation, asset inventory refresh, alert investigation and response, weak-password assessment, etc.); the model picks the goal and the backend orchestrates the internal steps
- Generates an immutable single-tool-bound execution plan; the runtime exposes only the tools allowed for the current step and rejects any guessed call outside the live tool catalog
- Async and long-running tasks (vulnerability scans, weak-password scans, etc.) use bounded polling with backoff, reporting honestly when still running instead of looping on "executing" forever
- All async high-level operations are tracked through a unified operation reference, with accepted, running, awaiting approval, awaiting input, partial and terminal states kept consistent across chat, events and UI
- Write operations count as success only when they create a real side effect and meet coverage; "all" requests report expected, covered, failed and uncovered counts, and zero tasks created means failure
- Final answers are bound to real tool evidence; answers that contradict or omit evidence fall back to a conservative summary, never fabricating "done"
- High-risk and write operations require explicit user intent and approval bound to scope and parameters, voided if the scope changes
- Recoverable blockers (e.g., a detection hook not yet allowlisted) produce a persistent decision card listing impacts and actions (extend allowlist, view suggestion only, pause, cancel, etc.), still actionable after refresh or restart

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
- A new auto-verify loop for baseline: a failed check auto-triggers a fix, and a successful fix auto-re-runs the check, looping until it passes or hits the max round limit, with an auto-verify toggle and max-rounds option at dispatch
- Pass-rate is split by type (detection pass-rate = passed checks / total, fix pass-rate = successful fixes / total), and compliance report export is simplified to XLS with per-task detail rows

### Intelligent Vulnerability Check and Fix

- Collects host software inventory and calls LLMs to identify known CVE vulnerabilities
- Vulnerability list supports severity filtering, search, and time-based sorting
- Supports manually entering custom CVEs and calling LLMs to complete vulnerability details
- Automatically calls LLMs to generate POC verification scripts and confirm whether vulnerabilities truly exist
- Automatically calls LLMs to generate targeted remediation scripts, with batch dispatch and progress tracking
- Script generation no longer depends on host selection; the batch-execute entry is consolidated into the one-click fix bar, and executing a generated script opens a host-selection dialog (online hosts with the script already generated) with max-rounds and auto-verify options
- After POC verification confirms a vulnerability, a fix is triggered automatically; after a successful fix, POC re-verification runs automatically, looping until it passes, hits the max round limit, or fails
- On script execution errors the LLM auto-regenerates and retries via the same self-healing mechanism as baseline, with task details showing per-round status and error summaries
- The vulnerability workbench is sorted by shell generation status (generated > generating > none) with a new "Shell Status" column, and the same ordering is applied on both frontend and backend

### Weak Password Detection (V6.1)

Takes an AI-native approach: based on collected asset results, an LLM identifies which hosts, applications, and configuration files may contain collectable credential material; the agent's standard weak-password tool reads the target fields and returns them; the server and LLM then jointly complete dictionary matching and result explanation.

- Analyzes collected asset results in one click to identify which hosts and applications may hold collectable account/password configurations, so users never need to specify config paths manually
- Detects weak passwords across Linux local accounts (`/etc/passwd`, `/etc/shadow`), Redis, MySQL/MariaDB, PostgreSQL, Nginx/Apache Basic Auth, OpenSSH, and AI Agents, MCP Servers, LLM gateways, and more
- The agent uses a standard weak-password collection tool to read account/password material from server-specified file paths and field selectors; `find`, recursive full-disk search, and arbitrary shell execution are forbidden
- For containerized apps, the agent reads in-container config files via the read-only `/proc/<pid>/root` mapping of the related process PID, adapting automatically to container environments
- Plaintext passwords are matched directly against dictionaries on the server; encrypted passwords/hashes are analyzed by the LLM and re-verified by a real server-side verifier, with proprietary algorithms that cannot be locally verified flagged as "AI-inferred, pending confirmation"
- Ships a built-in 1000-entry default weak-password dictionary, supports user-uploaded dictionaries, and offers one-click AI dictionary generation from natural-language descriptions with automatic deduplication
- Provides one-click batch detection that validates each host's online status before scanning and automatically skips offline hosts without creating invalid tasks
- Defines a fixed detection skill per application type with a generic fallback skill, and supports configuring detection rounds (10-50) plus a cap of 10 agent auxiliary-tool calls per application
- The task-detail progress bar tracks every stage (asset analysis -> tool dispatch -> config retrieval -> AI repair locator -> password matching -> result storage) with percentage, current host/application, and agent tool-call count
- Supports initiating, viewing, re-testing, and explaining weak-password tasks in natural language, with results showing the matched password, match method, AI explanation, failure reasons, and remediation guidance
- Passwords are displayed masked by default; revealing the full plaintext requires entering the system password / approval and is recorded in the audit log with a watermark

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

### Frontend Internationalization (V6.1)

- A Chinese/English switcher (zh-CN / en-US) is added next to the top-bar mode switcher; switching needs no refresh and preserves the current route, filters, forms, mode and assistant session
- Built on Vue I18n and the Element Plus ConfigProvider with a single global locale; translation happens at the display boundary while enums and raw data stay stable
- Two language resource sets split by business domain are bundled into the offline package with no external translation service or CDN; the language choice is persisted to local storage, synced across tabs, defaults to Simplified Chinese, and falls back on invalid values
- REST requests send Accept-Language and API errors are localized via stable error codes; assistant runs carry a locale snapshot, while commands, scripts, logs, evidence and identifiers are never translated

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
unzip aegis-v5.8-linux-amd64-release.zip
cd v5.8

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
