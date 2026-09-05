# Документация Paseka

Документы сгруппированы по глубине погружения. Спеки фич (`specs/`) **не** публикуются на сайте; см. [индекс спек](plans/specs-index.md).

## Если вы здесь впервые

Кратчайший путь от идеи к работающей колонии:

1. [Principles](idea/principles.md) — модель хореографии и идентичность Flight Trail.
2. [Colony layout](guide/colony-layout.md) — конфигурация проекта и машины.
3. [Bee config](guide/bee-config.md) и [prompt templates](guide/prompt-templates.md) — настройка ролей.
4. [Первый запуск CLI](guide/cli.md#first-time-setup) — NATS, диагностика и запуск работы.

## Идея и принципы

| Документ | Описание |
| -------- | -------- |
| [Principles](idea/principles.md) | Хореография, контракты, honey, HITL, colony vs machine |
| [Glossary](idea/glossary.md) | Bee-глоссарий и доменный словарь |
| [Brief (RU)](idea/brief.md) | Исторический продуктовый бриф |

## Использование

| Документ | Описание |
| -------- | -------- |
| [Начать здесь — Colony layout](guide/colony-layout.md) | `.paseka/`, machine-local конфиг, локальный NATS и `paseka init` |
| [Bee config](guide/bee-config.md) | YAML роли пчелы, адаптеры, routing |
| [Prompt templates](guide/prompt-templates.md) | `.paseka/prompts/`, `text/template`, partials |
| [CLI](guide/cli.md) | Справочник Queen Shell (`paseka`) и диагностика проблем |
| [Interactive sessions](guide/interactive-sessions.md) | HITL `bee chat`, SessionAdapter, Ghostty |
| [Queen Console](guide/queen-console.md) | Операторский обзор локального UI, review, sessions, System и Git |
| [Forage Cues](guide/cues.md) | Именованные точки входа (`.paseka/cues/`); опциональная Standing Trail; CLI, Console, Telegram; таймеры/webhook вызывают `cue run` |
| [Telegram gateway](guide/telegram-gateway.md) | Настройка и запуск `paseka gate telegram` |
| [Nuc packs](guide/nuc.md) | Переносимые пакеты bees + prompts |
| [Homelab deployment](guide/homelab-deployment.md) | Сервер/apiary в контейнере, Queen Console, внешний NATS |

## Справочник

| Документ | Описание |
| -------- | -------- |
| [Bee routing](reference/bee-routing.md) | `subscribes` / `publishes`, Reactor, task vs direct |
| [Event contracts](reference/event-contracts.md) | Виды `SIGNAL`, `INSIGHT`, `MUTATION`, `VERIFICATION` и правила авторинга |
| [Insight kinds](reference/insight-kinds.md) | Таксономия `INSIGHT` и `{{.Insights}}` |
| [Task ledger](reference/task-ledger.md) | `traceId` → `taskId` → `agentId`, жизненный цикл |

## Архитектура

| Документ | Описание |
| -------- | -------- |
| [Overview](architecture/overview.md) | Адаптеры, run IPC, worktrees, раскладка пакетов |
| [Hive Runtime map](architecture/paseka-runtime.html) | Интерактивная Archify-диаграмма (собирается в docs CI) |

## Планы

| Документ | Описание |
| -------- | -------- |
| [Changelog](plans/changelog.md) | Сделанное: ссылки на specs и канонические docs |
| [Specs index](plans/specs-index.md) | Краткая карта `docs/specs/` (тела только в репо) |
| [Backlog](plans/backlog.md) | Отложенная работа и допущения реализации |

Английский индекс: [README.md](README.md).

Индекс для агентов: [llms.txt](llms.txt) (полный корпус: [llms-full.txt](https://russ-p.github.io/paseka/llms-full.txt), генерируется `scripts/gen-llms-full.sh`).

Для контрибьюторов и coding agents: [AGENTS.md](https://github.com/russ-p/paseka/blob/main/AGENTS.md).
