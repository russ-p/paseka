# Paseka — Bee Glossary

## Брендинг

| Техническое             | Bee Language        |
| ----------------------- | ------------------- |
| Product                 | **Paseka**          |
| Runtime                 | Hive Runtime        |
| Instance                | Hive                |
| Workspace               | Apiary              |
| Project                 | Colony              |
| Cluster (если появится) | Federation of Hives |

---

# Пользователь

| Техническое   | Bee Language   |
| ------------- | -------------- |
| Developer     | Beekeeper      |
| Human Gateway | Queen Console  |
| CLI           | Queen Shell    |
| Dashboard     | Hive Dashboard |
| Web UI        | Queen Console  |

> Queen — это не агент и не оркестратор. Это персона интерфейса, через которую пасечник общается с ульем.

---

# Агенты

| Техническое        | Bee Language  |
| ------------------ | ------------- |
| Agent              | Bee           |
| Worker Agent       | Worker Bee    |
| Review Agent       | Guard Bee     |
| Planner Agent      | Scout Bee     |
| Knowledge Agent    | Archivist Bee |
| Commit Agent       | Builder Bee   |
| Notification Agent | Messenger Bee |
| Diagnostic Agent   | Medic Bee     |
| Observer           | Watch Bee     |

При необходимости пользователь может создавать собственные виды пчёл.

---

# Архитектура

| Technical Core | Bee Language       |
| -------------- | ------------------ |
| Event Bus      | Air                |
| Event Stream   | Flight Path        |
| Subject        | Flight Route       |
| Message        | Waggle Dance       |
| Broadcast      | Swarm Call         |
| Subscription   | Listening to Dance |
| Event Store    | Hive Memory        |
| Knowledge Base | Honeycomb          |
| Object Store   | Wax Storage        |
| KV Store       | Cell Storage       |

Важно: это исключительно язык интерфейса. API продолжает использовать `subject`, `stream`, `message`.

---

# Доменная модель

| Техническое  | Bee Language  |
| ------------ | ------------- |
| Signal       | Scent         |
| Insight      | Discovery     |
| Mutation     | Comb Proposal |
| Verification | Inspection    |
| Decision     | Hive Decision |
| Task         | Nectar        |
| Goal         | Bloom         |
| Context      | Hive Memory   |
| Artifact     | Wax Cell      |

Идея проходит естественный жизненный цикл:

Bloom → Nectar → Comb → Honey

---

# Метрики

| Technical     | Bee Language    |
| ------------- | --------------- |
| energyToken   | Honey Reserve   |
| confidence    | Pollen Quality  |
| priority      | Nectar Richness |
| traceId       | Flight Trail    |
| correlationId | Swarm Trail     |
| retries       | Return Flights  |
| timeout       | Sunset          |
| dead-letter   | Lost Bee        |

---

# Жизненный цикл задачи

| Technical      | Bee Language     |
| -------------- | ---------------- |
| Created        | New Bloom Found  |
| Accepted       | Nectar Collected |
| In Progress    | Building Comb    |
| Waiting Review | Guard Inspection |
| Verified       | Hive Approved    |
| Rejected       | Comb Rejected    |
| Completed      | Honey Stored     |
| Archived       | Winter Storage   |

---

# Состояние улья

| Technical              | Bee Language           |
| ---------------------- | ---------------------- |
| Healthy                | Calm Hive              |
| Busy                   | Active Foraging        |
| High Load              | Swarming               |
| Resource Exhausted     | Low Honey              |
| Infinite Loop Detected | Bees Flying in Circles |
| Idle                   | Resting Hive           |

---

# Диагностика

| Technical | Bee Language      |
| --------- | ----------------- |
| Logs      | Hive Chronicle    |
| Metrics   | Hive Health       |
| Trace     | Flight Trail      |
| Timeline  | Season Timeline   |
| Replay    | Replay the Season |
| Debug     | Inspect the Hive  |
| Audit     | Hive Inspection   |

---

# Команды CLI

```bash
paseka init
paseka run
paseka status
paseka doctor
paseka inspect
paseka replay
paseka hive list
paseka hive create
paseka bee list
paseka bee inspect
paseka queen
```

---

# UI-примеры

Вместо:

> Task accepted by planner.

Пользователь увидит:

> 🐝 Scout Bee discovered fresh nectar.

---

Вместо:

> Verification failed.

> 🛡 Guard Bee rejected the comb.

---

Вместо:

> Waiting for human approval.

> 👑 The Queen awaits your decision.

---

Вместо:

> Context updated.

> 🍯 New honey has been stored in the comb.

---

Вместо:

> Agent terminated.

> 🐝 A bee has returned to the hive.

---

# Принцип трансляции

Система имеет два независимых языка.

## Technical Layer

Используется в API, SDK, протоколах, логах и внутренней реализации.

```text
SIGNAL
MUTATION
VERIFICATION
TRACE
ENERGY
CONFIDENCE
```

## Experience Layer (Bee Language)

Используется исключительно в интерфейсах пользователя, документации, визуализации, системных промптах и повествовании.

```text
Scent
Nectar
Comb
Honey
Hive
Queen
Worker Bee
Guard Bee
Scout Bee
```

Оба слоя описывают одну и ту же систему. Bee Language не заменяет техническую модель, а служит её художественной интерпретацией, делая взаимодействие с Paseka более живым и запоминающимся без ущерба инженерной строгости.
