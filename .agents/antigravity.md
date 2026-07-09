# LapangGo Antigravity Setup

Use this file when starting or configuring Antigravity for this repository.

## Local Project Rules

Antigravity should follow:

- `AGENTS.md`
- `.agents/agent_workflow.md`
- `.agents/definition_of_done.md`

These files are the source of truth for project-specific behavior. They apply even when the external `agent-skills` plugin is not installed.

## Recommended First Message

Use this prompt when starting a new Antigravity session in this repo:

```text
Ikuti AGENTS.md, .agents/agent_workflow.md, dan .agents/definition_of_done.md.
Ikuti juga .agents/antigravity_staff_roles_v1_3_stabilization_plan.md untuk pekerjaan Staff Roles v1.3/v1.3.1.
Untuk task non-trivial, buat plan dulu dan jangan implement sebelum disetujui.
Jaga perubahan agent lain; cek git status sebelum edit.
```

## Mandatory Agent-Skills Protocol

For Staff Roles v1.3/v1.3.1 stabilization, Antigravity must apply the agent-skills lifecycle even if the upstream plugin is unavailable.

Required sequence:

1. `/review` or manual `code-review-and-quality`: inspect the current staff/audit implementation against the plan before changing code.
2. `/planning` or manual `planning-and-task-breakdown`: produce a small ordered task list and wait for approval.
3. `/build` or manual `incremental-implementation`: implement one thin slice at a time.
4. `/test` or manual `test-driven-development`: add/update targeted tests for each changed behavior.
5. `/review` again: verify the diff for IDOR, permission, migration, API contract, and UI regressions.
6. `/ship` or manual `shipping-and-launch`: run release-readiness checks and summarize blockers.

Do not skip directly to implementation for auth, staff access, audit logs, finance, refund, promo, booking, or migration work.

## Optional Agent-Skills Plugin

If you want Antigravity to use the upstream `agent-skills` plugin natively, install it from outside this repo:

```bash
agy plugin install https://github.com/addyosmani/agent-skills.git
agy plugin list
```

The plugin provides slash commands and automatic skill discovery. The local `AGENTS.md` still remains the project-specific authority for LapangGo.

## Slash Command Mapping

When the plugin is installed:

- `/spec`: define feature requirements before code.
- `/planning`: break work into small verified tasks.
- `/build`: implement the next slice.
- `/test`: run test-driven verification.
- `/review`: review code quality and regressions.
- `/code-simplify`: simplify without behavior change.
- `/ship`: run release-readiness checks.
- `/webperf`: audit browser-facing performance.

Use `/planning`, not `/plan`, because Antigravity may reserve `/plan` for its own planner.

## LapangGo-Specific Guardrails

For Antigravity tasks touching auth, owner access, staff roles, finance, refunds, promos, audit logs, booking status, or migrations:

- Require a short plan before implementation.
- Validate server-side permissions.
- Add tests or a manual QA path.
- Run targeted verification before broad checks.
- Report any skipped verification plainly.

## Current Priority

The next Antigravity task should use:

- `.agents/antigravity_staff_roles_v1_3_stabilization_plan.md`
- `staff_roles_v1_3_fix_implementation_plan.md`
- `staff_roles_v1_3_1_audit_trail_implementation_plan.md`

The goal is to stabilize the existing Staff Roles and Audit Trail work before starting any new feature.
