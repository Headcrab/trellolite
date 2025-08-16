<div align="center">

# Trellolite

Минималистичный и быстрый клон Trello: доски → списки → карточки, live‑обновления через SSE, drag‑and‑drop и настраиваемые цвета.

<p>
   <a href="https://www.postgresql.org/" target="_blank"><img alt="Postgres" src="https://img.shields.io/badge/Postgres-16-4169E1?logo=postgresql&logoColor=white"></a>
   <img alt="Go" src="https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white">
   <img alt="License" src="https://img.shields.io/badge/License-MIT-green">
   <img alt="Docker Compose" src="https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white">
</p>

</div>

## Демо

Готовая развернутая версия: https://trellolite.xtrim.work/

## Обзор ✨

Trellolite — учебный и референс‑проект, демонстрирующий:
- Чистый backend на Go + Postgres с простым хранилищем и миграциями
- Реальное‑время через Server‑Sent Events (SSE) без внешних брокеров
- Ванильный frontend (без фреймворков): HTML5 DnD, <dialog>, тёмная/светлая темы, i18n (EN/RU)
- Позиционирование с разрежённой сеткой (sparse ints) и авто‑перепаковкой
- Опциональные цвета для досок/списков/карточек с быстрым диалогом выбора
- Базовая аутентификация (email+пароль) и OAuth (GitHub)
 - Базовая аутентификация (email+пароль) и OAuth (GitHub, Google)

## Возможности 🚀

- Доски, списки, карточки, комментарии к карточкам
- Drag‑and‑drop:
   - Карточки внутри списка и между списками
   - Карточки и списки между окнами браузера (cross‑window DnD)
   - Списки внутри доски и между досками (drag заголовка списка на доску в сайдбаре)
   - Доски в сайдбаре (перестановка)
- SSE‑события для синхронизации во всех открытых клиентах
- Выбор цвета для: доски, списка, карточки (иконка палитры → диалог)
- Темная/светлая/авто тема (переключатель в заголовке)
- Группы и доступ к доскам: владелец доски может открыть её для своих групп; участники могут покидать группы
- Фильтр видимости досок: две отжимаемые кнопки «Мои» и «Группы» с запоминанием состояния
- Сворачиваемый сайдбар: в свернутом виде остаётся только кнопка разворота (гамбургер)
- Визуальная подсказка для «чужих» досок из групп — иконка пользователей вместо иконки доски

## Стек 🧰

- Backend: Go 1.21+ (net/http, database/sql, pgx v5), логирование через slog
- DB: Postgres 16
- Frontend: ванильные HTML/CSS/JS + EventSource (SSE), HTML5 Drag and Drop, i18n
- Runtime: Docker Compose, multi‑stage сборка

### Технологии и языки

- Языки: Go, SQL, HTML5, CSS3, JavaScript (ES6+)
- Протоколы/веб‑API: HTTP/1.1, Server‑Sent Events (EventSource), HTML5 Drag and Drop, DOM, Fetch API
- БД и драйверы: PostgreSQL 16, pgx v5, database/sql
- UI и доступность: нативные элементы (dialog, details/summary), aria‑атрибуты, клавиатурная навигация контекстного меню
- i18n: собственный лёгкий рантайм (`web/i18n/i18n.js`) + словари (`en.json`, `ru.json`)
- Сборка/контейнеры: Docker multi‑stage, distroless runtime image

## Структура 🗂️

```
docker-compose.yml
Dockerfile
go.mod
README.md
server/
   api.go        // HTTP API и SSE
   main.go       // запуск сервера, статические файлы
   models.go     // структуры данных/JSON
   store.go      // SQL, миграции, позиционирование и перемещения
web/
   index.html    // разметка, диалоги, иконки
   styles.css    // темы, компоненты, DnD‑подсветки
   app.js        // логика UI, API, SSE, DnD, i18n и контекстные меню
   i18n/         // словари en.json, ru.json и рантайм i18n.js
```

## Быстрый старт (Docker) 🐳

Требуется установленный Docker (Desktop) и Docker Compose.

```powershell
docker compose up -d --build
```

Откройте: http://localhost:8080

Остановка:

```powershell
docker compose down
```

Альтернатива через Taskfile (установите go-task):

```powershell
task up    # сборка и запуск
task logs  # логи
task down  # остановка
```

## Локальная разработка 💻

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

## Аутентификация 🔐

Trellolite использует cookie‑сессии. Неавторизованные пользователи видят только страницу входа `/web/login.html` (корень `/` отдает её до входа). После входа доступны API и UI досок.

Эндпоинты:
- POST /api/auth/register — создать пользователя (email+пароль)
- POST /api/auth/login — войти
- POST /api/auth/logout — выйти
- GET /api/auth/me — текущий пользователь (анонимно возвращает `{user:null}`)
- GET /api/auth/providers — доступные OAuth провайдеры (GitHub/Google)
- GET /api/auth/oauth/github/start — начало OAuth
- GET /api/auth/oauth/github/callback — коллбэк OAuth
 - GET /api/auth/oauth/google/start — начало OAuth
 - GET /api/auth/oauth/google/callback — коллбэк OAuth
 - POST /api/auth/reset — запрос на сброс пароля (dev: magic‑link пишется в логи)
 - POST /api/auth/reset/confirm — подтверждение сброса по токену

UI:
- `/web/login.html` содержит форму email+пароль и кнопку «Войти через GitHub» (появляется, если настроен OAuth). Кнопки оформлены единообразно; «Регистрация» и «Забыли пароль?» выглядят как ссылки.
- В основном интерфейсе слева — панель пользователя (имя/почта, инициалы) и «Выйти». Сайдбар можно свернуть; останется только кнопка‑гамбургер.
- Ссылка «Забыли пароль?» создаёт dev‑magic‑link: токен выводится в лог сервера; вставьте `#reset=...` в URL страницы входа, чтобы открыть форму ввода нового пароля.

Фильтр видимости досок:
- В шапке списка досок — две отжимаемые кнопки «Мои» и «Группы». Состояние запоминается в localStorage.
   - Мои — доски, созданные вами
   - Группы — доски, открытые группам, в которых вы состоите
   - Обе включены — показываются все доступные.
   - Для досок, доступных через группы и не вами созданных, в списке показывается иконка пользователей.

## API (кратко) 📡

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

- Groups (для текущего пользователя)
   - GET /api/my/groups — список групп пользователя и его роль
   - POST /api/groups {name} — создать свою группу (создатель — админ)
   - DELETE /api/groups/{id} — удалить свою группу (только админ группы)
   - GET /api/groups/{id}/users — участники группы
   - POST /api/groups/{id}/users {user_id} — добавить участника
   - DELETE /api/groups/{id}/users/{user_id} — убрать участника
   - POST /api/groups/{id}/leave — покинуть группу (для не‑админов)

- Boards ↔ Groups
   - GET /api/boards/{id}/groups — группы, у которых есть доступ к доске
   - POST /api/boards/{id}/groups {group_id} — дать доступ группе (только владелец доски)
   - DELETE /api/boards/{id}/groups/{group_id} — убрать доступ (только владелец доски)

Ответы — JSON. На ошибки — { ok:false, error:"..." } и соответствующий HTTP код.

## События SSE 🔔

Подписка клиента: EventSource(`/api/boards/{id}/events`).

Примеры типов событий: board.moved, board.updated, list.created|updated|deleted|moved, card.created|updated|deleted|moved, comment.created. Клиентская логика обновляет UI инкрементально либо перерисовывает разметку при сложных изменениях.

## DnD и позиционирование 🧲

- Позиции — разрежённые целые (шаг 1000). При нехватке промежутка — фоновая пере-нумерация и повтор попытки.
- DnD с ограничением «тянуть только за хэндл» для списков и досок, чтобы не ломать карточки.
- Перенос списка между досками — перетаскиванием заголовка списка на доску в сайдбаре.

## Цвета 🎨

Иконка палитры на досках, списках и карточках открывает диалог выбора цвета:
- Предустановленные свотчи + произвольный цвет
- Сброс (без цвета)
- Сохранение через PATCH color; UI раскрашивается, другие клиенты получают обновления по SSE.

## Проекты 📁

Доску можно привязать к проекту при создании (в диалоге). Список проектов доступен в выпадающем списке; можно создать новый проект из того же диалога. Это помогает группировать доски по контексту.

## Переменные окружения ⚙️

- ADDR — адрес сервера, по умолчанию `:8080`
- DATABASE_URL — строка подключения к Postgres; по умолчанию для docker `postgres://postgres:postgres@db:5432/trellolite?sslmode=disable`

### Аутентификация
- SESSION_COOKIE_NAME — имя cookie сессии (по умолчанию trellolite_sess)
- SESSION_TTL — срок жизни сессии (например, `336h` для 14 дней)
- COOKIE_SAMESITE — `lax` (по умолчанию), `strict`, `none`
- COOKIE_SECURE — `true` в проде (HTTPS), `false` в dev

OAuth (GitHub):
- OAUTH_GITHUB_CLIENT_ID
- OAUTH_GITHUB_CLIENT_SECRET
- OAUTH_GITHUB_REDIRECT_URL (например, `http://localhost:8080/api/auth/oauth/github/callback`)

OAuth (Google):
SMTP (для писем подтверждения/сброса):
- SMTP_HOST / SMTP_PORT
- SMTP_FROM
- SMTP_USERNAME / SMTP_PASSWORD (опционально)

- OAUTH_GOOGLE_CLIENT_ID
- OAUTH_GOOGLE_CLIENT_SECRET
- OAUTH_GOOGLE_REDIRECT_URL (например, `http://localhost:8080/api/auth/oauth/google/callback`)

При наличии настроенных значений GitHub‑провайдера на странице входа появится кнопка «Войти через GitHub». Сессии — cookie httpOnly.

### Настройка OAuth (GitHub)

1) Создайте OAuth App в GitHub: Settings → Developer settings → OAuth Apps → New OAuth App.
   - Homepage URL: `http://localhost:8080`
   - Authorization callback URL: `http://localhost:8080/api/auth/oauth/github/callback`
2) Скопируйте `.env.example` в `.env` и заполните идентификатор и секрет:

```env
OAUTH_GITHUB_CLIENT_ID=...
OAUTH_GITHUB_CLIENT_SECRET=...
OAUTH_GITHUB_REDIRECT_URL=http://localhost:8080/api/auth/oauth/github/callback
```

3) Перезапустите контейнеры:

```powershell
docker compose up -d --build
```

Compose автоматически подхватит `.env`. После этого кнопка GitHub появится на странице входа, а полный OAuth‑флоу заработает.

### Настройка OAuth (Google)

1) Создайте OAuth‑клиент в Google Cloud Console: APIs & Services → Credentials → Create Credentials → OAuth client ID (Application type: Web application).
   - Authorized JavaScript origins: `http://localhost:8080`
   - Authorized redirect URIs: `http://localhost:8080/api/auth/oauth/google/callback`
2) Добавьте переменные в `.env`:

```env
OAUTH_GOOGLE_CLIENT_ID=...
OAUTH_GOOGLE_CLIENT_SECRET=...
OAUTH_GOOGLE_REDIRECT_URL=http://localhost:8080/api/auth/oauth/google/callback
```

3) Перезапустите контейнеры:

```powershell
docker compose up -d --build
```

Кнопка Google появится на странице входа, если провайдер сконфигурирован.

### Rate limiting и dev‑сброс пароля

Для снижения brute‑force на `/api/auth/register|login|reset|reset/confirm` действует простая in‑memory квота на IP (на dev сервере). В проде замените на внешний middleware/прокси.

Dev‑сброс пароля: POST `/api/auth/reset` логирует magic‑link (токен) в stdout; откройте `/web/login.html#reset={token}` и укажите новый пароль. Для приватности ответ всегда «ok», даже если email не существует.

## Тёмная/светлая тема 🌗

Переключатель в левом верхнем углу (light/dark/auto). Хранится в localStorage.

## Тестирование и качество ✅

- При сборке используется multi‑stage Docker; бинарник на distroless image
- Логи HTTP с длительностью (slog JSON)
- Миграции запускаются на старте (idempotent)

## Траблшутинг 🛠️

- Ошибка вида `column "color" does not exist` — перезапустите контейнеры после обновления кода: миграция добавит недостающие колонки.

```powershell
docker compose up -d --build
```

- Порт 8080 занят — задайте `ADDR`, например `:8081`.

## Лицензия 📄

MIT. Используйте свободно в учебных и демонстрационных целях.

