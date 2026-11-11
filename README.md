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

Правила в [CLAUDE.md](CLAUDE.md) (дубликат в `AGENTS.md`) и в [docs/agents/workflow.md](docs/agents/workflow.md).

## Запуск

Появится вместе с кодом.
