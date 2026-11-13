# AI-buddy Constitution

AI-buddy guides Avito interns through adaptation: it answers from team sources, leads to the next step and
acts through corporate services. Full product and design docs live in the `docs/` submodule; this file holds
the rules every spec, plan and task must respect.

## Core Principles

### I. Sources, not memory
Facts come from documents, services and the team repository, never from model memory. Every factual answer
carries the sources it used; an answer without sources is a defect (small talk excepted). When nothing is
found, the agent says so and offers the escalation chain: mentor, manager, HR BP, open sources.

### II. Permissions before the model
Permissions are RBAC plus ABAC (role and attributes: own object, own team, subordinates, cluster) and are
enforced in tools and search, never in prompts. The model never sees data the caller may not see. A refused
call returns `forbidden` with no content and the agent answers with a refusal.

### III. Actions only through tools
The agent acts only through registered MCP tools; tools reach data through service adapters: real Avito
sources directly (with a snapshot for offline runs), unavailable systems through mock services with a protobuf
contract. Thread type limits the tool set. Submitting a request to a service is a human
click on our API, never a model tool.

### IV. Test-first (non-negotiable)
Every feature and fix starts with a failing test. Golden dialogs in a spec are test cases run against the
scripted fake model. Line coverage of application code stays above 80%, mocks excluded.

### V. The agent does not do the intern's work
The agent explains the task, the repository and the team process. It never writes code for the intern.

### VI. Nothing blocks the UI
Model and service calls run in background jobs; the UI gets status events. Timeouts and failures turn into a
clear message with the escalation chain.

## Data and Security
- No real personal or internal data. Public Avito sources (checklist, Playbook) only with attribution. Test
  instructions are labelled as such and never presented as Avito regulations.
- No sensitive personal data and no financial operations.
- Secrets only in environment variables; `.env.example` in the repository.

## Development Workflow
- A spec exists before code for every agent scenario and tool; behaviour, input, output, refusal and golden
  dialogs are mandatory sections.
- Specs live in `specs/NNN-name/`. Branches follow the repository rules (`feat/agent-loop`), not spec-kit
  feature numbers.
- Code, comments and specs in English; user-facing text and dialog examples in Russian.

## Governance
This constitution follows `docs/CLAUDE.md` and `docs/agents/workflow.md`; on conflict those win and this file
is updated. Changes to contracts go through `docs/design/system-design.md` first.

**Version**: 1.1.0 | **Ratified**: 2026-11-12 | **Last Amended**: 2026-11-13
