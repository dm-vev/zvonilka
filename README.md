# Zvonilka

Self-hosted messaging and calling platform built as a service-oriented monorepo.

Zvonilka combines Go domain services, protobuf API contracts, PostgreSQL-backed persistence, local infrastructure manifests, and Rust crates reserved for realtime/media workloads. The project is designed for private deployments, federation experiments, bot integrations, encrypted conversations, calls, notifications, and constrained transport bridges.

Languages: [Русский](#русский) | [English](#english)

## Русский

### Что это

Zvonilka - это каркас self-hosted платформы для мессенджера и звонков. Репозиторий хранит серверные сервисы, доменную логику, protobuf-контракты, миграции, локальную инфраструктуру и экспериментальные компоненты для federation/mesh-сценариев.

Проект полезен, если нужно:

- запускать собственный сервер сообщений и звонков;
- развивать API для мобильных, веб- и bot-клиентов;
- проверять архитектуру federated messaging;
- тестировать доставку сообщений через обычные сети и constrained transports;
- держать бизнес-логику, контракты и deployment-артефакты в одном monorepo.

### Возможности

- Аккаунты, устройства, сессии, password/bot/debug login flows.
- Диалоги, группы, каналы, сообщения, реакции, pin/unpin, forwarding и sync events.
- E2EE-aware message payload validation и хранение opaque encryption metadata.
- Bot API boundary для внешней автоматизации.
- Call domain, call hooks, RTC runtime и TURN/local infrastructure support.
- Notification worker и delivery retry model.
- Federation core для peer/link/bundle модели.
- Bridge-подход для Meshtastic, MeshCore и других delay-tolerant transports.
- Protobuf-контракты как source of truth для публичных и внутренних API.
- Rust workspace для realtime gateway, presence service и media worker.

### Статус проекта

Проект находится в активной разработке. API, миграции, конфигурация и границы сервисов могут меняться. Для production-развертывания обязательно проведите security review, настройте секреты, TLS, observability, backup и политику миграций.

### Архитектура

Высокоуровневая модель:

- `cmd/` - entrypoints для запуска Go-сервисов.
- `internal/app/` - wiring сервисов, transports и runtime boundaries.
- `internal/domain/` - бизнес-правила и доменные сервисы.
- `internal/platform/` - конфигурация, storage, logging, federation transports и технические адаптеры.
- `proto/contracts/` - protobuf-контракты API.
- `gen/proto/` - сгенерированный Go-код из protobuf.
- `deploy/` - локальная инфраструктура и SQL-миграции.
- `rust/` - Rust workspace для realtime/media компонентов.
- `docs/` - архитектурные заметки.
- `tests/` - интеграционные сценарии и заготовки.

Основные Go-сервисы:

- `controlplane` - аккаунты, directory, chats, groups, channels, moderation и admin workflows.
- `gateway` - client-facing gRPC/API edge, chat/call/sync/media/users endpoints.
- `botapi` - внешняя bot-интеграция.
- `callworker` и `callhooks` - обработка call lifecycle и webhook delivery.
- `notificationworker` - доставка уведомлений и retry.
- `federationworker` - outbox/inbox federation flow.
- `federationbridge`, `federationmeshtastic`, `federationmeshcore` - bridge adapters.
- `devseed` - создание тестовых данных для локальной разработки.

Подробнее: [`docs/architecture.md`](docs/architecture.md), [`docs/layout.md`](docs/layout.md), [`FEDERATION.md`](FEDERATION.md).

### Требования

- Go `1.25` или совместимая версия из `go.mod`.
- Rust toolchain с `cargo` для crates в `rust/`.
- Docker и Docker Compose для локальных зависимостей.
- `buf` для lint/breaking checks protobuf-контрактов.
- PostgreSQL, Redis, MinIO и coturn для полного локального окружения.

### Быстрый старт

1. Склонируйте репозиторий.

```bash
git clone https://github.com/dm-vev/zvonilka.git
cd zvonilka
```

2. Запустите локальную инфраструктуру.

```bash
make local-up
```

3. Соберите проект.

```bash
make build
```

4. Запустите нужный сервис.

```bash
make run-controlplane
make run-gateway
make run-botapi
```

5. Остановите локальные зависимости, когда они больше не нужны.

```bash
make local-down
```

### Конфигурация

Конфигурация читается из переменных окружения с префиксом `ZVONILKA_`. Наиболее часто используемые группы:

- `ZVONILKA_HTTP_ADDR`, `ZVONILKA_GRPC_ADDR` - адреса базовых HTTP/gRPC endpoints.
- `ZVONILKA_GATEWAY_HTTP_ADDR`, `ZVONILKA_GATEWAY_GRPC_ADDR` - адреса gateway.
- `ZVONILKA_FEATURE_FEDERATION_ENABLED` - включение federation-функций.
- `ZVONILKA_FEDERATION_*` - peer, bridge, worker и trust-настройки federation.
- `ZVONILKA_MESHTASTIC_*`, `ZVONILKA_MESHCORE_*` - настройки mesh bridges.
- `ZVONILKA_NOTIFICATION_*` - retry, polling и delivery webhook уведомлений.
- `ZVONILKA_CALL_*`, `ZVONILKA_RTC_*` - звонки, hooks, TURN/RTC runtime.
- `ZVONILKA_OBJECT_STORAGE_*` - S3/MinIO object storage.
- `ZVONILKA_IDENTITY_*` - настройки identity lifecycle и debug login.

Локальный Compose по умолчанию поднимает PostgreSQL `17`, Redis `8`, coturn и MinIO. Пароли из `deploy/local/docker-compose.yml` предназначены только для локальной разработки.

### Команды разработки

```bash
make fmt
make test
make build
make proto-lint
make proto-breaking
```

Отдельные сервисы можно запускать напрямую:

```bash
go run ./cmd/controlplane
go run ./cmd/gateway
go run ./cmd/botapi
go run ./cmd/notificationworker
go run ./cmd/federationworker
```

### Тестирование

Основная команда:

```bash
make test
```

Она запускает `go test ./...` и `cargo test --workspace`. Для тестов, которым нужны внешние зависимости, сначала поднимите локальную инфраструктуру через `make local-up`.

### Безопасность

- Не используйте локальные Docker Compose секреты в production.
- Включайте TLS и корректную network policy для публичных endpoints.
- Храните реальные токены, TURN secrets, webhook secrets и S3 credentials вне git.
- Проверяйте federation peers и bridge transports как недоверенные каналы.
- Перед публикацией production-сборки запускайте тесты, protobuf checks и security review.

### Вклад в проект

Перед изменениями проверьте актуальность ветки, запустите форматирование и тесты. Для крупных изменений сначала согласуйте архитектуру или API-контракты.

Рекомендуемый порядок:

1. Создать отдельную ветку.
2. Обновить protobuf и generated code, если меняются API.
3. Добавить или обновить тесты.
4. Запустить `make fmt`, `make test`, `make proto-lint`.
5. Описать изменение и риски в pull request.

### Лицензия

Проект распространяется по лицензии BSD 3-Clause.

## English

### What It Is

Zvonilka is a self-hosted messaging and calling platform skeleton. The repository keeps server services, domain logic, protobuf contracts, migrations, local infrastructure, and experimental components for federation and mesh-style delivery in one monorepo.

Use it when you need to:

- run a private messaging and calling backend;
- build APIs for mobile, web, and bot clients;
- explore federated messaging architecture;
- test message delivery over normal networks and constrained transports;
- keep domain code, contracts, and deployment assets together.

### Features

- Accounts, devices, sessions, password/bot/debug login flows.
- Direct chats, groups, channels, messages, reactions, pin/unpin, forwarding, and sync events.
- E2EE-aware message payload validation and opaque encryption metadata storage.
- Bot API boundary for external automation.
- Call domain, call hooks, RTC runtime, and TURN/local infrastructure support.
- Notification worker with retry-oriented delivery.
- Federation core built around peer/link/bundle concepts.
- Bridge model for Meshtastic, MeshCore, and other delay-tolerant transports.
- Protobuf contracts as the source of truth for public and internal APIs.
- Rust workspace for realtime gateway, presence service, and media worker workloads.

### Project Status

The project is under active development. APIs, migrations, configuration, and service boundaries may change. Before production deployment, run a dedicated security review and configure secrets, TLS, observability, backups, and migration policy.

### Architecture

High-level layout:

- `cmd/` - runnable Go service entrypoints.
- `internal/app/` - service wiring, transports, and runtime boundaries.
- `internal/domain/` - business rules and domain services.
- `internal/platform/` - configuration, storage, logging, federation transports, and technical adapters.
- `proto/contracts/` - protobuf API contracts.
- `gen/proto/` - generated Go protobuf output.
- `deploy/` - local infrastructure and SQL migrations.
- `rust/` - Rust workspace for realtime/media components.
- `docs/` - architecture notes.
- `tests/` - integration scenarios and placeholders.

Main Go services:

- `controlplane` - accounts, directory, chats, groups, channels, moderation, and admin workflows.
- `gateway` - client-facing gRPC/API edge for chat, calls, sync, media, and users.
- `botapi` - external bot integration boundary.
- `callworker` and `callhooks` - call lifecycle and webhook delivery.
- `notificationworker` - notification delivery and retries.
- `federationworker` - federation outbox/inbox processing.
- `federationbridge`, `federationmeshtastic`, `federationmeshcore` - bridge adapters.
- `devseed` - local development seed data.

More details: [`docs/architecture.md`](docs/architecture.md), [`docs/layout.md`](docs/layout.md), [`FEDERATION.md`](FEDERATION.md).

### Requirements

- Go `1.25` or a compatible version from `go.mod`.
- Rust toolchain with `cargo` for the crates under `rust/`.
- Docker and Docker Compose for local dependencies.
- `buf` for protobuf linting and breaking-change checks.
- PostgreSQL, Redis, MinIO, and coturn for the full local stack.

### Quick Start

1. Clone the repository.

```bash
git clone https://github.com/dm-vev/zvonilka.git
cd zvonilka
```

2. Start local infrastructure.

```bash
make local-up
```

3. Build the project.

```bash
make build
```

4. Run a service.

```bash
make run-controlplane
make run-gateway
make run-botapi
```

5. Stop local dependencies when they are no longer needed.

```bash
make local-down
```

### Configuration

Configuration is read from environment variables with the `ZVONILKA_` prefix. Common groups include:

- `ZVONILKA_HTTP_ADDR`, `ZVONILKA_GRPC_ADDR` - base HTTP/gRPC bind addresses.
- `ZVONILKA_GATEWAY_HTTP_ADDR`, `ZVONILKA_GATEWAY_GRPC_ADDR` - gateway bind addresses.
- `ZVONILKA_FEATURE_FEDERATION_ENABLED` - federation feature switch.
- `ZVONILKA_FEDERATION_*` - federation peer, bridge, worker, and trust settings.
- `ZVONILKA_MESHTASTIC_*`, `ZVONILKA_MESHCORE_*` - mesh bridge settings.
- `ZVONILKA_NOTIFICATION_*` - notification polling, retry, and delivery webhook settings.
- `ZVONILKA_CALL_*`, `ZVONILKA_RTC_*` - calls, hooks, TURN, and RTC runtime settings.
- `ZVONILKA_OBJECT_STORAGE_*` - S3/MinIO object storage settings.
- `ZVONILKA_IDENTITY_*` - identity lifecycle and debug login settings.

The local Compose file starts PostgreSQL `17`, Redis `8`, coturn, and MinIO. Credentials in `deploy/local/docker-compose.yml` are for local development only.

### Development Commands

```bash
make fmt
make test
make build
make proto-lint
make proto-breaking
```

You can also run individual services directly:

```bash
go run ./cmd/controlplane
go run ./cmd/gateway
go run ./cmd/botapi
go run ./cmd/notificationworker
go run ./cmd/federationworker
```

### Testing

Main command:

```bash
make test
```

It runs `go test ./...` and `cargo test --workspace`. For tests that need external dependencies, start the local stack first with `make local-up`.

### Security

- Do not use local Docker Compose credentials in production.
- Enable TLS and proper network policy for public endpoints.
- Keep real tokens, TURN secrets, webhook secrets, and S3 credentials out of git.
- Treat federation peers and bridge transports as untrusted channels.
- Run tests, protobuf checks, and a security review before publishing production builds.

### Contributing

Before changing the project, update your branch and run formatting and tests. For large changes, discuss architecture or API contracts first.

Recommended flow:

1. Create a dedicated branch.
2. Update protobuf and generated code if APIs change.
3. Add or update tests.
4. Run `make fmt`, `make test`, and `make proto-lint`.
5. Describe the change and risks in the pull request.

### License

This project is distributed under the BSD 3-Clause License.
