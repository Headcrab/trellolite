<div align="center">

# Trellolite

Минималистичный и быстрый клон Trello: доски → списки → карточки, live‑обновления через SSE, drag‑and‑drop и настраиваемые цвета.

</div>

## Обзор

Trellolite — учебный и референс‑проект, демонстрирующий:
- Чистый backend на Go + Postgres с простым хранилищем и миграциями
- Реальное‑время через Server‑Sent Events (SSE) без внешних брокеров
- Ванильный frontend (без фреймворков): HTML5 DnD, <dialog>, тёмная/светлая темы
- Позиционирование с разрежённой сеткой (sparse ints) и авто‑перепаковкой
- Опциональные цвета для досок/списков/карточек с быстрым диалогом выбора

## Возможности

- Доски, списки, карточки, комментарии к карточкам
- Drag‑and‑drop:
   - Карточки внутри списка и между списками
   - Списки внутри доски и между досками (drag заголовка списка на доску в сайдбаре)
   - Доски в сайдбаре (перестановка)
- SSE‑события для синхронизации во всех открытых клиентах
- Выбор цвета для: доски, списка, карточки (иконка палитры → диалог)
- Темная/светлая/авто тема (переключатель в заголовке)

## Стек

- Backend: Go 1.21+ (net/http, database/sql, pgx v5), логирование через slog
- DB: Postgres 16
- Frontend: ванильные HTML/CSS/JS + EventSource, HTML5 Drag and Drop
- Runtime: Docker Compose, multi‑stage сборка

## Структура

```
docker-compose.yml
Dockerfile
go.mod
README.md
server/
   api.go        // HTTP API и SSE
   events.go     // простой EventBus (SSE)
   main.go       // запуск сервера, статические файлы
   models.go     // структуры данных/JSON
   store.go      // SQL, миграции, позиционирование и перемещения
web/
   index.html    // разметка, диалоги, иконки
   styles.css    // темы, компоненты, DnD‑подсветки
   app.js        // логика UI, API, SSE, DnD
```

## Быстрый старт (Docker)

Требуется установленный Docker (Desktop) и Docker Compose.

```powershell
docker compose up -d --build
```

Откройте: http://localhost:8080

Остановка:

```powershell
docker compose down
```

## Локальная разработка

1) Поднимите Postgres из compose или локально. Значение по умолчанию в контейнере:

```
postgres://postgres:postgres@localhost:5432/trellolite?sslmode=disable
```

2) Запустите сервер локально (нужен Go):

```powershell
$env:DATABASE_URL = "postgres://postgres:postgres@localhost:5432/trellolite?sslmode=disable"
go run ./server
```

3) Откройте http://localhost:8080

Миграции применяются автоматически на старте (создание таблиц и недостающих колонок, включая color).

## API (кратко)

- Boards
   - GET /api/boards — список
   - POST /api/boards {title}
   - GET /api/boards/{id}, GET /api/boards/{id}/full
   - PATCH /api/boards/{id} {title?, color?}
   - POST /api/boards/{id}/move {new_index}
   - DELETE /api/boards/{id}
   - GET /api/boards/{id}/events — SSE поток
- Lists
   - GET /api/boards/{id}/lists
   - POST /api/boards/{id}/lists {title}
   - PATCH /api/lists/{id} {title?, pos?, color?}
   - POST /api/lists/{id}/move {new_index, target_board_id?}
   - DELETE /api/lists/{id}
- Cards
   - GET /api/lists/{id}/cards
   - POST /api/lists/{id}/cards {title, description}
   - PATCH /api/cards/{id} {title?, description?, pos?, due_at?, color?}
   - POST /api/cards/{id}/move {target_list_id, new_index}
   - DELETE /api/cards/{id}
- Comments
   - GET /api/cards/{id}/comments
   - POST /api/cards/{id}/comments {body}

Ответы — JSON. На ошибки — { ok:false, error:"..." } и соответствующий HTTP код.

## События SSE

Подписка клиента: EventSource(`/api/boards/{id}/events`).

Примеры типов событий: board.moved, board.updated, list.created|updated|deleted|moved, card.created|updated|deleted|moved, comment.created. Клиентская логика обновляет UI инкрементально либо перерисовывает разметку при сложных изменениях.

## DnD и позиционирование

- Позиции — разрежённые целые (шаг 1000). При нехватке промежутка — фоновая пере-нумерация и повтор попытки.
- DnD с ограничением «тянуть только за хэндл» для списков и досок, чтобы не ломать карточки.
- Перенос списка между досками — перетаскиванием заголовка списка на доску в сайдбаре.

## Цвета

Иконка палитры на досках, списках и карточках открывает диалог выбора цвета:
- Предустановленные свотчи + произвольный цвет
- Сброс (без цвета)
- Сохранение через PATCH color; UI раскрашивается, другие клиенты получают обновления по SSE.

## Переменные окружения

- ADDR — адрес сервера, по умолчанию `:8080`
- DATABASE_URL — строка подключения к Postgres; по умолчанию для docker `postgres://postgres:postgres@db:5432/trellolite?sslmode=disable`

## Тёмная/светлая тема

Переключатель в левом верхнем углу (light/dark/auto). Хранится в localStorage.

## Тестирование и качество

- При сборке используется multi‑stage Docker; бинарник на distroless image
- Логи HTTP с длительностью (slog JSON)
- Миграции запускаются на старте (idempotent)

## Траблшутинг

- Ошибка вида `column "color" does not exist` — перезапустите контейнеры после обновления кода: миграция добавит недостающие колонки.

```powershell
docker compose up -d --build
```

- Порт 8080 занят — задайте `ADDR`, например `:8081`.

## Лицензия

MIT. Используйте свободно в учебных и демонстрационных целях.

