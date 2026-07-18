# Документация Paseka

Индекс основных документов. Спецификации фич (`specs/`) сюда не входят.

| Документ | Описание |
| -------- | -------- |
| [001-brief.md](001-brief.md) | Продуктовый бриф: концепция хореографического AI-роя, EDA-контракты, `traceId` / `energyToken`, стек (NATS + JetStream), HITL и шаги MVP |
| [002-paseka-glossary.md](002-paseka-glossary.md) | Bee-глоссарий: брендинг, пользовательские термины, роли агентов и словарь доменной модели |
| [003-architecture.md](003-architecture.md) | Архитектура колонии: `.paseka/` и machine-local конфиг, `paseka init`, адаптеры, worktree-поток, раскладка пакетов |
| [004-prompt-templates.md](004-prompt-templates.md) | Шаблоны промптов в `.paseka/prompts/`: `text/template`, partials, переменные контекста и рендер при диспатче |
| [005-task-ledger.md](005-task-ledger.md) | Task Ledger: связь `traceId` → `taskId` → `agentId`, жизненный цикл задач и проекция состояния трейса |
| [006-interactive-sessions.md](006-interactive-sessions.md) | Интерактивные HITL-сессии: `bee chat`, SessionAdapter + PTY, реестр сессий и attach через Ghostty |
| [007-cli.md](007-cli.md) | Справочник Queen Shell (`paseka`): команды init/run/status, bee run/chat, console, energy и прочий CLI |
| [008-bee-routing.md](008-bee-routing.md) | Маршрутизация пчёл: `subscribes` / `publishes`, Reactor, task vs direct dispatch |
| [009-insight-kinds.md](009-insight-kinds.md) | Таксономия `INSIGHT`: отличие от `VERIFICATION`, виды payload и проекция в `{{.Insights}}` |
| [010-bee-config.md](010-bee-config.md) | Конфиг роли пчелы (`.paseka/bees/<role>.yaml`): схема, адаптеры, `command` / `post_exec`, params, контракты |
| [011-nuc.md](011-nuc.md) | Nuc — переносимые пакеты пчёл: export/import bees и prompts между Colony |
| [999-backlog.md](999-backlog.md) | Отложенные идеи и follow-up'ы вне текущего MVP |

Английский индекс: [README.md](README.md).

Индекс для агентов: [llms.txt](llms.txt) (полный корпус: [llms-full.txt](https://russ-p.github.io/paseka/llms-full.txt), генерируется `scripts/gen-llms-full.sh`).
