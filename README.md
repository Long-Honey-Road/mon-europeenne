# mon-europeenne

AI-buddy: ИИ-наставник для стажёров Авито. Прототип для хакатона Hack&Change 2026.

## Документация

Требования кейса, продуктовые решения, архитектура и правила работы лежат в сабмодуле [`docs/`](docs/)
(репозиторий `Long-Honey-Road/ochre-stonecrop`). Начало: [docs/README.md](docs/README.md).

```bash
git clone --recurse-submodules git@github.com:Long-Honey-Road/mon-europeenne.git
```

Если репозиторий уже склонирован без сабмодулей:

```bash
git submodule update --init
```

Обновить документацию до последней версии (делать перед началом любой работы):

```bash
git submodule update --init --remote docs
```

`docs/` это ochre-репо: приватный репозиторий с документацией, подключённый сабмодулем.

## Для ИИ-агентов

Правила в [CLAUDE.md](docs/CLAUDE.md) (дубликат в `docs/AGENTS.md`) и в [docs/agents/workflow.md](docs/agents/workflow.md).

## Development

Requirements: Go 1.27, Docker (also for tests: they start Postgres via testcontainers). Dev tools (golangci-lint, buf, protoc plugins) are pinned in
`backend/tools.mod` and run via `go tool`, nothing to install.

```bash
cp deploy/.env.example deploy/.env
docker compose -f deploy/docker-compose.yml up -d --build --wait
```

```bash
deploy/smoke.sh   # login, profile, then one chat turn over SSE
```

The stack: `postgres`, `redis`, one-shot `migrate` and `seed`, `api` (HTTP and SSE), `worker` (agent turns
from the queue) and `frontend`. The worker calls the model from `LLM_*` in `deploy/.env`: by default Ollama on the host
(`http://host.docker.internal:11434/v1`, model `qwen3.6:35b-a3b`). With `LLM_PROVIDER=fake` it answers every
question with a fixed text: for CI and frontend work without a model.

`frontend` serves the built app on http://localhost:3000 (`WEB_PORT` to change) and proxies `/v1` and `/healthz`
to the API, so the browser talks to one origin; SSE goes through unbuffered.

Chat flow for clients: create a thread, open `GET /v1/private/threads/{id}/events` (SSE) first, then
`POST /v1/private/threads/{id}/messages` (202). Events: `status` (what the agent is doing), `delta` (answer text
as it is generated), `reset` (drop the streamed text), `block` (sources, person, not_found), then `done` with
`message_id` and the final `text` (show it instead of the stream: the server may remove invented links) or
`error` with `escalation`.

Access requests (`FEATURE_REQUESTS=true` by default): the agent prepares a request with `prepare_request` and
the answer gets an `action` block with the filled form; the user sends it with `POST /v1/private/requests`.
Until the corp-requests mock exists, a simulated request system in Redis approves or rejects requests after a
delay from `backend/seed/request_types.json`; the worker checks every 15 s and sends `request_status` to the
thread. `GET /v1/private/today` shows missing, waiting and rejected requests as blockers.

The API listens on `http://localhost:8080` (`API_PORT` to change). Paths follow
`/{version}/{public|private}/{resource}`: `/v1/public/...` needs no session, `/v1/private/...` needs
`Authorization: Bearer <token>` from `POST /v1/public/auth/login`. Contract: `backend/api/openapi/buddy-api.yaml`.

Test accounts (synthetic, from `backend/seed/people.json`):

| Login | Password | Role |
|---|---|---|
| `intern` | `intern-demo` | intern, backend, team Messenger |
| `mentor` | `mentor-demo` | mentor |
| `manager` | `manager-demo` | manager of the team |

Frontend (from `frontend/`):

| Command | What it does |
|---|---|
| `npm ci && npm run dev` | dev server on http://localhost:5173; `/v1` is proxied to the API on `localhost:8080` |
| `npm test` / `npm run test:coverage` | Vitest with the API mocked by msw |
| `npm run typecheck` | TypeScript check |
| `npm run gen:api` | regenerate `src/api/schema.gen.ts` from `backend/api/openapi/buddy-api.yaml` |

Sign in with a test account below; the login page also has buttons that fill them in.

From `backend/`:

| Command | What it does |
|---|---|
| `make test` | unit tests |
| `make cover` | tests with coverage, generated code excluded |
| `make lint` | golangci-lint, buf lint and proto format check |
| `make gen` | regenerate Go, gRPC, grpc-gateway and OpenAPI from `api/proto` |
| `make run CMD=api` | run a `buddy` subcommand |
| `make run CMD="migrate up"` | apply migrations to `DATABASE_URL` (`down` reverts all) |
| `make run CMD=llm-check` | probe the model from `LLM_*` env: plain answer, tool call, JSON by schema, stream |
| `make run CMD='ask -v Кто поможет с деплоем?'` | one agent turn as `intern` (`-as`, `-thread`, `-detailed`); `-v` prints tool calls |
| `make run CMD='trace -n 5 -as intern'` | the latest saved agent turns: prompt (`-prompt`), tool calls with arguments and results, text before cleanup; in compose `docker compose -f deploy/docker-compose.yml exec api buddy trace` |

The agent's tools over MCP (stdio), as a test user in one thread type; with `DATABASE_URL` the plan and
request tools are there too. For Claude Code, from the repo root:

```bash
claude mcp add buddy -- go -C backend run ./cmd/buddy mcp -as intern -thread plan
```

Or MCP Inspector: `npx @modelcontextprotocol/inspector go -C backend run ./cmd/buddy mcp`.

Contracts live in `backend/api/proto` (source of truth). `backend/gen` and
`backend/api/openapi/buddy-api.yaml` are generated and committed; CI fails if they are stale.
