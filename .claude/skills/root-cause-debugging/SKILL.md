---
name: root-cause-debugging
description: Use this skill when the current task is essentially a bug, runtime error, failed test, CI failure, abnormal behavior, feature regression, or unexpected functionality. The AI must first trace the complete bug-related call chain, analyze the root cause based on business logic, and then apply the smallest safe fix.
---

# Root Cause Debugging Skill

Use this Skill whenever the current task is essentially a bug-related problem.

The bug may come from a user report, automated test failure, CI failure, runtime error, abnormal log, UI issue, API issue, unexpected behavior, feature regression, or a problem discovered by the AI during development.

The AI must not immediately modify code after seeing the error location, failed test, or problematic function.

The AI must first start from the failing function, failed test, or abnormal location, then trace upward along the call chain to identify all key functions on the bug path, including the directly failing function, upstream callers, entry functions, parameter construction functions, state initialization functions, business decision functions, and other relevant logic.

After identifying the key functions on the bug path, the AI must analyze them together with the business logic:

1. What is the original business goal of this execution path?
2. What is each key function responsible for in the business flow?
3. How are the key parameters or states created, passed, and changed?
4. Which function first starts to deviate from the expected business behavior?
5. Why does this eventually trigger the current bug?
6. Is the real root cause in the current failing function, or in upstream business logic, parameter construction, state initialization, configuration, or data source?

Before modifying code, the AI must briefly explain:

1. What the bug symptom is;
2. Where the problematic location is;
3. Which key functions are on the bug path;
4. How these functions call each other;
5. What business logic this path represents;
6. What the root cause is, or what the most likely root cause is;
7. Why fixing only the current error line is not enough;
8. What minimal fix will be applied;
9. How the fix will be verified.

The AI may modify code only after the root cause has been identified.

When modifying code, the AI must follow these rules:

- Make only the smallest change that addresses the root cause;
- Do not perform unrelated refactoring;
- Do not simply swallow exceptions;
- Do not hide the problem by bypassing logic;
- Do not casually change test expectations;
- Do not modify multiple unrelated modules at once;
- Do not break real business logic just to make tests pass.