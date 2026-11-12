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
docker compose -f deploy/docker-compose.yml up -d --build --wait   # api, postgres, redis
```

```bash
deploy/smoke.sh   # test login, then the private profile
```

The API listens on `http://localhost:8080` (`API_PORT` to change). Paths follow
`/{version}/{public|private}/{resource}`: `/v1/public/...` needs no session, `/v1/private/...` needs
`Authorization: Bearer <token>` from `POST /v1/public/auth/login`. Contract: `backend/api/openapi/buddy-api.yaml`.

Test accounts (synthetic, from `backend/seed/people.json`):

| Login | Password | Role |
|---|---|---|
| `intern` | `intern-demo` | intern, backend, team Messenger |
| `mentor` | `mentor-demo` | mentor |
| `manager` | `manager-demo` | manager of the team |

Frontend dev server (from `frontend/`): `npm ci && npm run dev`, then http://localhost:5173.

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

Contracts live in `backend/api/proto` (source of truth). `backend/gen` and
`backend/api/openapi/buddy-api.yaml` are generated and committed; CI fails if they are stale.
