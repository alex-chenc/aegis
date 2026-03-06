# Draft: Update AI Prompts for Mandatory Skill Invocation

## Requirements (confirmed)

1. **Mandatory Superpower Invocation**: Before writing ANY code, must call `superpower` skill
2. **Frontend-Specific Requirement**: Before writing frontend code, must call `ui-ux-pro-max` skill
3. **Files to Update**:
   - `baseline_system_design_v2.2/ai_implementation_prompt_v2.2_complete.md` - Main AI implementation prompt (411 lines)
   - `baseline_system_design_v2.2/test_implementation_prompt_v2.2_complete.md` - Test implementation prompt (417 lines)

## Technical Decisions

### Decision 1: Add Prominent "核心强制指令" Section
Both files will have skill invocation requirements in a prominent section.

### Decision 2: Use User-Specified Shorthand Names
- Use `superpower` as specified (shorthand for `superpowers/using-superpowers`)
- Use `ui-ux-pro-max` as specified (refers to frontend skills)
- Keep consistent with test prompt's existing naming convention

### Decision 3: Section Strategy
- **Main prompt**: Rename Section 2 "核心指令" → "核心强制指令" and prepend skill items
- **Test prompt**: Add `ui-ux-pro-max` item to existing Section 3

## Research Findings

### Main Implementation Prompt (ai_implementation_prompt_v2.2_complete.md)
- 411 lines total
- Section 2 "核心指令" has 4 items (lines 11-16):
  1. 严格遵循设计
  2. 模块化实现
  3. 高质量代码
  4. 提供完整文件
- NO skill invocation mentions anywhere
- Has frontend tasks 6.1, 6.2 that would need `ui-ux-pro-max`

### Test Implementation Prompt (test_implementation_prompt_v2.2_complete.md)
- 417 lines total
- Section 3 "核心强制指令" (lines 21-29) HAS `superpower` skill:
  ```
  1. **调用 `superpower` skill**：在编写任何后端或 Agent 测试代码前，必须先调用 `superpower` skill。
  ```
- MISSING `ui-ux-pro-max` in core section (but mentioned inline at line 353)

## Metis Review Findings

### Critical Issues Identified
1. **Skill Name Precision**: User said `superpower`/`ui-ux-pro-max` but system has different exact names
   - `superpowers/using-superpowers` is the actual skill
   - `frontend-ui-ux` and `ui-ux-pro-max/CLAUDE` exist
   - **Decision**: Use user's shorthand names for consistency with existing test prompt

### Guardrails Applied
- ONLY modify the two specified files
- Do NOT create/modify skill files
- Do NOT update other design documents
- Do NOT change section numbering

### Scope Cream Prevention
- NOT updating AGENTS.md
- NOT adding to other v2.2 docs
- NOT creating skill documentation

## Open Questions (RESOLVED)

1. ~~Should we add to EVERY task or just once at top?~~
   - **RESOLVED**: Add prominently in core section ONCE; keep inline reminders per-task as bonus

2. ~~Exact skill names to use?~~
   - **RESOLVED**: Use user-specified shorthand (`superpower`, `ui-ux-pro-max`) to match test prompt convention

## Scope Boundaries

- INCLUDE: Updating both prompt files
- INCLUDE: Making skill invocation mandatory and prominent
- EXCLUDE: Other design documents
- EXCLUDE: AGENTS.md
- EXCLUDE: Modifying skill files