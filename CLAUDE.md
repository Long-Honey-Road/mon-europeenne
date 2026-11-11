# mon-europeenne: AI-buddy for Avito interns (Hack&Change 2026)

All documentation lives in the `docs/` submodule (an "ochre" repo: a private docs repository attached as a git
submodule; this one is `Long-Honey-Road/ochre-stonecrop`). It is written in Russian.

Before any task:

```bash
git submodule update --init --remote docs
```

Then read `docs/agents/workflow.md` (reading order, code rules, SDD, tests, git, definition of done) and
`docs/plan.md` (current stage and what "done" means). The documentation map is `docs/README.md`.

Hard rules, never to be broken:

- **No `git push` without an explicit instruction from a human.** Commit locally, report "branch ready to push",
  wait. Applies to every repository, including `docs/`, in every circumstance.
- Work in a branch (`feat/`, `fix/`, `foundation/`, `docs/`); `main` only via PR after green CI.
- Tests first: a failing test before implementation code. Line coverage of application code above 80%.
- Spec before code for every agent scenario and tool (SDD).
- Code, comments, README and code-adjacent docs in English. Comments only where intent is not visible from code.
- Synthetic data only; test instructions are labelled as such, never presented as Avito regulations.
- Permissions are enforced in tools and search, not in prompts; what a role cannot see is neither shown nor executed.
- Agent actions only through MCP tools; tools only through mock services with an OpenAPI contract.
- Mocks stay minimal: no request portal, no Jira/Confluence clones. SDD specs use spec-kit.
- Permissions are RBAC plus ABAC (role and attributes), checked in tools and search before the model sees data.
- The agent never writes code for the intern; it explains the task, the repository and the team process.
- LLM and service calls never block the UI; status events and failure handling are mandatory.
- Secrets only in environment variables; `.env.example` in the repo.
- Documentation changes: commit inside `docs/` first, then commit the updated submodule pointer here.

Decisions that are not documented are not invented silently: ask, or record an assumption in
`docs/product/decisions.md`.
