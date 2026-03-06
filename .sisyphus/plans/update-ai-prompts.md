# Work Plan: Update AI Prompts for Mandatory Skill Invocation

## TL;DR

> **Quick Summary**: Add mandatory skill invocation requirements to two AI prompt files. Before writing code, AI must call `superpower` skill (backend/Agent) or `ui-ux-pro-max` skill (frontend).
> 
> **Deliverables**:
> - Updated `ai_implementation_prompt_v2.2_complete.md` with skill invocation requirements
> - Updated `test_implementation_prompt_v2.2_complete.md` with complete skill requirements
> 
> **Estimated Effort**: Quick
> **Parallel Execution**: YES - 2 independent tasks
> **Critical Path**: Task 1 (no dependencies) → Task 2 (no dependencies)

---

## Context

### Original Request
Update AI prompts to enforce:
1. **Mandatory `superpower` skill invocation** before writing ANY backend/Agent code
2. **Mandatory `ui-ux-pro-max` skill invocation** before writing ANY frontend code

### Interview Summary
**Key Discussions**:
- Two files need updating: main implementation prompt and test implementation prompt
- Test file already has `superpower` requirement but missing `ui-ux-pro-max` in core section
- Main implementation file has NO skill invocation requirements at all
- Use user-specified shorthand names (`superpower`, `ui-ux-pro-max`) for consistency

**Research Findings**:
- Main prompt (411 lines): Section 2 "核心指令" exists, no skill mentions
- Test prompt (417 lines): Section 3 "核心强制指令" has `superpower`, needs `ui-ux-pro-max`

### Metis Review
**Identified Gaps** (addressed):
- Skill name precision: Using user-specified shorthand names
- Section strategy: Rename main prompt's Section 2, add to test prompt's Section 3
- Guardrails: Only modify specified files, no section renumbering

---

## Work Objectives

### Core Objective
Add mandatory skill invocation requirements to AI prompt files so that AI coding assistants must call appropriate skills before writing code.

### Concrete Deliverables
- Modified `baseline_system_design_v2.2/ai_implementation_prompt_v2.2_complete.md`
- Modified `baseline_system_design_v2.2/test_implementation_prompt_v2.2_complete.md`

### Definition of Done
- [ ] Both files contain mandatory skill invocation requirements
- [ ] Main prompt has both `superpower` and `ui-ux-pro-max` requirements
- [ ] Test prompt has `ui-ux-pro-max` added to Section 3
- [ ] Section numbering unchanged
- [ ] Existing content preserved

### Must Have
- Skill invocation prominently placed in core instruction section
- Both skills mentioned for main prompt (backend + frontend tasks)
- Chinese text for skill requirements

### Must NOT Have (Guardrails)
- Do NOT modify other design documents
- Do NOT create or modify skill files
- Do NOT change section numbering
- Do NOT remove existing content

---

## Verification Strategy

### QA Policy
Every task will include verification commands to confirm changes are correct.

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately - Independent):
├── Task 1: Update test_implementation_prompt [quick]
└── Task 2: Update ai_implementation_prompt [quick]

Wave FINAL (After ALL tasks):
├── Task F1: Verify skill mentions exist
└── Task F2: Verify section structure preserved
```

---

## TODOs

- [ ] 1. Update test_implementation_prompt_v2.2_complete.md

  **What to do**:
  - Add `ui-ux-pro-max` skill requirement to Section 3 "核心强制指令"
  - Insert as new item 2 (after existing `superpower` item)
  - Content: `2. **调用 `ui-ux-pro-max` skill**：在编写任何前端测试代码前，必须先调用 `ui-ux-pro-max` skill。`
  - Renumber subsequent items (2→3, 3→4, 4→5, 5→6, 6→7)

  **Must NOT do**:
  - Do NOT remove existing content
  - Do NOT change section numbering
  - Do NOT remove inline skill reminders already present

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple text addition to markdown file
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Task 2)
  - **Blocks**: None
  - **Blocked By**: None

  **References**:
  - `baseline_system_design_v2.2/test_implementation_prompt_v2.2_complete.md:21-29` - Current Section 3 content to modify
  - `baseline_system_design_v2.2/test_implementation_prompt_v2.2_complete.md:353` - Example of `ui-ux-pro-max` mention (for consistent phrasing)

  **Acceptance Criteria**:
  - [ ] Section 3 contains `ui-ux-pro-max` skill requirement
  - [ ] Item is numbered as item 2
  - [ ] Previous items 2-6 are renumbered to 3-7
  - [ ] File still has correct section count

  **QA Scenarios**:
  ```
  Scenario: Verify ui-ux-pro-max added to Section 3
    Tool: Bash
    Steps:
      1. grep -n "ui-ux-pro-max" baseline_system_design_v2.2/test_implementation_prompt_v2.2_complete.md | head -1
    Expected Result: Line number is between 21-30 (within Section 3)
    Evidence: .sisyphus/evidence/task-1-ui-ux-added.txt
  ```

  **Commit**: YES
  - Message: `docs(prompts): add ui-ux-pro-max skill requirement to test prompt`
  - Files: `baseline_system_design_v2.2/test_implementation_prompt_v2.2_complete.md`

- [ ] 2. Update ai_implementation_prompt_v2.2_complete.md

  **What to do**:
  - Rename Section 2 title from "核心指令" to "核心强制指令"
  - Prepend two new skill invocation items before existing items
  - New item 1: `1. **调用 `superpower` skill**：在编写任何后端或 Agent 代码前，必须先调用 `superpower` skill。`
  - New item 2: `2. **调用 `ui-ux-pro-max` skill**：在编写任何前端代码前，必须先调用 `ui-ux-pro-max` skill。`
  - Renumber existing items 1-4 to 3-6

  **Must NOT do**:
  - Do NOT remove existing 4 core instruction items
  - Do NOT change other sections
  - Do NOT modify file structure

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple text modification to markdown file
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Task 1)
  - **Blocks**: None
  - **Blocked By**: None

  **References**:
  - `baseline_system_design_v2.2/ai_implementation_prompt_v2.2_complete.md:11-16` - Current Section 2 content
  - `baseline_system_design_v2.2/test_implementation_prompt_v2.2_complete.md:21-29` - Reference for consistent phrasing

  **Acceptance Criteria**:
  - [ ] Section 2 title changed to "核心强制指令"
  - [ ] Contains both `superpower` and `ui-ux-pro-max` skill requirements
  - [ ] Original 4 items preserved and renumbered
  - [ ] File structure unchanged

  **QA Scenarios**:
  ```
  Scenario: Verify both skills added to main prompt
    Tool: Bash
    Steps:
      1. grep -c "superpower" baseline_system_design_v2.2/ai_implementation_prompt_v2.2_complete.md
      2. grep -c "ui-ux-pro-max" baseline_system_design_v2.2/ai_implementation_prompt_v2.2_complete.md
    Expected Result: Both counts >= 1
    Evidence: .sisyphus/evidence/task-2-skills-added.txt

  Scenario: Verify section title changed
    Tool: Bash
    Steps:
      1. grep -n "核心强制指令" baseline_system_design_v2.2/ai_implementation_prompt_v2.2_complete.md
    Expected Result: Line number around line 11 (Section 2)
    Evidence: .sisyphus/evidence/task-2-title-changed.txt
  ```

  **Commit**: YES
  - Message: `docs(prompts): add mandatory skill invocation to implementation prompt`
  - Files: `baseline_system_design_v2.2/ai_implementation_prompt_v2.2_complete.md`

---

## Final Verification Wave

- [ ] F1. **Verify skill mentions exist**
  Run verification commands on both files to confirm skill requirements are present.
  ```bash
  # Main prompt should have both skills
  grep -c "superpower" baseline_system_design_v2.2/ai_implementation_prompt_v2.2_complete.md
  grep -c "ui-ux-pro-max" baseline_system_design_v2.2/ai_implementation_prompt_v2.2_complete.md
  
  # Test prompt should have both skills in Section 3
  grep -n "ui-ux-pro-max" baseline_system_design_v2.2/test_implementation_prompt_v2.2_complete.md | head -1
  ```
  Output: Both counts >= 1, test prompt ui-ux-pro-max in Section 3 range

- [ ] F2. **Verify section structure preserved**
  Confirm section numbering is unchanged in both files.
  ```bash
  # Verify section headers unchanged
  grep -n "^## [0-9]" baseline_system_design_v2.2/ai_implementation_prompt_v2.2_complete.md
  grep -n "^## [0-9]" baseline_system_design_v2.2/test_implementation_prompt_v2.2_complete.md
  ```
  Output: Sections numbered 1-7 (main) and 1-6 (test) as expected

---

## Commit Strategy

- **Task 1**: `docs(prompts): add ui-ux-pro-max skill requirement to test prompt`
- **Task 2**: `docs(prompts): add mandatory skill invocation to implementation prompt`

---

## Success Criteria

### Verification Commands
```bash
# Verify both skills in main prompt
grep -c "superpower" baseline_system_design_v2.2/ai_implementation_prompt_v2.2_complete.md && echo "superpower OK"
grep -c "ui-ux-pro-max" baseline_system_design_v2.2/ai_implementation_prompt_v2.2_complete.md && echo "ui-ux-pro-max OK"

# Verify ui-ux-pro-max added to test prompt Section 3
grep -n "ui-ux-pro-max" baseline_system_design_v2.2/test_implementation_prompt_v2.2_complete.md | head -1
```

### Final Checklist
- [ ] All "Must Have" present (both skills in both files)
- [ ] All "Must NOT Have" absent (no section renumbering, no removed content)
- [ ] Both files modified correctly
- [ ] Commits created with proper messages