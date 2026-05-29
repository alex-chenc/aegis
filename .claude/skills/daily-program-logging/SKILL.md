---
name: daily-program-logging
description: Use this skill when writing or modifying application code that needs normal operational logging, especially when the code lacks logs or a unified logging library.
---

# Daily Program Logging

## When to Use

Use this skill when the task involves writing, modifying, or reviewing application code and the code should contain normal operational logs.

This skill applies to:

- New feature development
- Code refactoring
- Bug fixing
- Background task implementation
- API development
- CLI tool development
- Service startup and shutdown logic
- External dependency calls
- Important business process handling

Do not use this skill to add random debug prints everywhere.

## Purpose

AI often writes code that only focuses on whether the function works, but ignores runtime logs.

This causes problems after the program is used in real environments:

- Humans cannot easily troubleshoot failures.
- Operators cannot understand what the program is doing.
- Important business behavior is not observable.
- External dependency failures are hard to locate.
- Production issues require temporary log patches to investigate.

This skill ensures that logs are treated as part of maintainable and operable software, not as an afterthought.

## Core Rules

### 1. Prefer existing project logging

Before adding logs, check whether the project already has a logging library, logging utility, or logging convention.

If it exists, use the existing logging style.

Do not introduce a new logging framework when the project already has one.

### 2. Create a unified logging library when missing

If this is a new project, or the project does not have a unified logging library, implement one first.

After the logging library is created, all future code must use it.

Do not scatter `print`, `console.log`, or custom one-off wrappers across the codebase.

The logging library must support:

- DEBUG
- INFO
- WARN
- ERROR

The logging library must support configurable options, including:

- Log storage path
- Maximum size of a single log file
- Maximum number of retained log files
- Log level
- Console output switch
- File output switch
- Log format
- Log rotation

### 3. Add logs at meaningful points

Add logs around important runtime behavior, not every line of code.

Good log points include:

- Service startup
- Service shutdown
- Important business operation started
- Important business operation completed
- Important business operation failed
- External API request started
- External API request failed
- Database operation failed
- Background task started
- Background task completed
- Configuration loaded
- Recoverable abnormal condition
- Permission or validation failure
- Retry, fallback, or degraded behavior

### 4. Use correct log levels

Use log levels consistently.

- DEBUG: detailed diagnostic information for development or troubleshooting.
- INFO: important normal events and business progress.
- WARN: abnormal but recoverable situations.
- ERROR: failed operations that require attention.

Do not use ERROR for normal business failures that are expected and handled.

Do not use INFO for noisy loop-level details.

### 5. Include useful context

Logs should help humans understand what happened.

When useful, include context such as:

- Request ID
- Trace ID
- User ID
- Resource ID
- Task ID
- Operation name
- Input summary
- Result status
- Error reason
- Duration
- Retry count
- External dependency name

### 6. Protect sensitive data

Never log:

- Passwords
- Tokens
- Secrets
- Private keys
- Full cookies
- Authorization headers
- Sensitive personal data
- Large raw request or response bodies
- Binary content

If context is needed, log masked, shortened, or summarized values.

### 7. Control log noise

Do not add logs that make normal operation hard to read.

Avoid:

- Logging every loop iteration
- Logging huge objects
- Duplicating the same log in multiple layers
- Logging both caller and callee without clear value
- Temporary debug logs left in production code

### 8. Keep logs stable and searchable

Log messages should be stable and easy to search.

Prefer clear event-style messages, for example:

```text
user_login_started
user_login_failed
task_execution_completed
external_api_request_failed
config_loaded