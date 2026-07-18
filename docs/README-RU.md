# Документация Paseka

Документы сгруппированы по глубине погружения. Спеки фич (`specs/`) **не** публикуются на сайте; см. [индекс спек](plans/specs-index.md).

## Идея и принципы

| Документ | Описание |
| -------- | -------- |
| [Principles](idea/principles.md) | Хореография, контракты, honey, HITL, colony vs machine |
| [Glossary](idea/glossary.md) | Bee-глоссарий и доменный словарь |
| [Brief (RU)](idea/brief.md) | Исторический продуктовый бриф |

## Использование

| Документ | Описание |
| -------- | -------- |
| [Colony layout](guide/colony-layout.md) | `.paseka/` и machine-local конфиг, slug, `paseka init` |
| [CLI](guide/cli.md) | Справочник Queen Shell (`paseka`) |
| [Bee config](guide/bee-config.md) | YAML роли пчелы, адаптеры, routing |
| [Prompt templates](guide/prompt-templates.md) | `.paseka/prompts/`, `text/template`, partials |
| [Interactive sessions](guide/interactive-sessions.md) | HITL `bee chat`, SessionAdapter, Ghostty |
| [Nuc packs](guide/nuc.md) | Переносимые пакеты bees + prompts |

## Справочник

| Документ | Описание |
| -------- | -------- |
| [Bee routing](reference/bee-routing.md) | `subscribes` / `publishes`, Reactor, task vs direct |
| [Insight kinds](reference/insight-kinds.md) | Таксономия `INSIGHT` и `{{.Insights}}` |
| [Task ledger](reference/task-ledger.md) | `traceId` → `taskId` → `agentId`, жизненный цикл |

## Архитектура

| Документ | Описание |
| -------- | -------- |
| [Overview](architecture/overview.md) | Адаптеры, run IPC, worktrees, раскладка пакетов |

## Планы

| Документ | Описание |
| -------- | -------- |
| [Changelog](plans/changelog.md) | Сделанное: ссылки на specs и канонические docs |
| [Specs index](plans/specs-index.md) | Краткая карта `docs/specs/` (тела только в репо) |
| [Backlog](plans/backlog.md) | Отложенные идеи и follow-up'ы |

Английский индекс: [README.md](README.md).

Индекс для агентов: [llms.txt](llms.txt) (полный корпус: [llms-full.txt](https://russ-p.github.io/paseka/llms-full.txt), генерируется `scripts/gen-llms-full.sh`).
