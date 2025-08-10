# Trellolite — план развития (пользователи, авторизация, группы, админка)

Документ описывает поэтапное внедрение пользовательской модели, аутентификации (регистрация и OAuth), назначений на карточки, групп пользователей, админ‑панели и сопутствующих изменений бэкенда/фронтенда и безопасности.

## Цели
- Войти в сервис под учетной записью (email+пароль) и через OAuth (GitHub → далее Google).
- Управлять проектами/досками/правами через роли и группы.
- Назначать исполнителя карточке (минимум 1 пользователь). В перспективе поддержать несколько назначенных.
- Предоставить простую админку: пользователи, группы, проекты, базовые операции.

## Не цели (пока)
- Полный SSO/SCIM, SAML — вне первой итерации.
- Продвинутые ACL на уровне поля — используем RBAC.
- Полнотекстовый поиск/файлы — пока не входит.

---

## Архитектура (ключевые решения)
- Сессии на сервере (cookie httpOnly + Secure + SameSite=Lax). JWT/refresh — опционально для API‑клиентов.
- OAuth через провайдеры (GitHub первым): классический Authorization Code Flow.
- RBAC на уровне проекта, досок и списков: Admin/Owner/Maintainer/Member/Viewer.
- SSE поток авторизуется cookie; доступ подтверждается по правам на запрошенную доску.
- UI без фреймворков сохраняется: добавим лайтовые страницы входа/регистрации/админки.

## Изменения модели данных (SQL)
Все изменения — idempotent (IF NOT EXISTS), не ломающие текущее поведение.

Новые таблицы:
- users
  - id bigserial PK
  - email text unique not null
  - password_hash text not null default '' (для OAuth‑только может быть пустым)
  - name text not null default ''
  - avatar_url text
  - is_active boolean not null default true
  - is_admin boolean not null default false
  - created_at timestamptz not null default now()
- oauth_accounts
  - id bigserial PK
  - user_id bigint FK users(id) on delete cascade
  - provider text not null (e.g. 'github','google')
  - provider_user_id text not null
  - access_token text, refresh_token text, expires_at timestamptz
  - unique(provider, provider_user_id)
- groups
  - id bigserial PK
  - name text unique not null
  - created_at timestamptz not null default now()
- user_groups
  - user_id bigint FK users(id) on delete cascade
  - group_id bigint FK groups(id) on delete cascade
  - PK(user_id, group_id)
- projects
  - id bigserial PK
  - name text not null
  - owner_user_id bigint FK users(id)
  - created_at timestamptz not null default now()
- project_members
  - project_id bigint FK projects(id) on delete cascade
  - user_id bigint FK users(id) on delete cascade
  - role smallint not null default 2  -- 0 Viewer, 1 Member, 2 Maintainer, 3 Owner
  - PK(project_id, user_id)
- sessions
  - id bigserial PK
  - user_id bigint FK users(id)
  - token text unique not null
  - created_at timestamptz not null default now()
  - expires_at timestamptz not null

Изменения существующих таблиц:
- boards: добавить project_id bigint FK projects(id) (nullable на 1‑й фазе), created_by bigint FK users(id) (nullable)
- lists: (без изменений на 1‑й фазе)
- cards: добавить assignee_user_id bigint FK users(id) NULL; (позже — таблица card_assignees для множества)
- comments: добавить user_id bigint FK users(id) NOT NULL (автор комментария), на 1‑й фазе можно допустить NULL и постепенно заполнить.

Миграции — отдельным блоком в store.go (ALTER TABLE IF NOT EXISTS…), затем постепенная адаптация выборок.

## API (черновик)
Аутентификация:
- POST /auth/register {email, password, name}
- POST /auth/login {email, password}
- POST /auth/logout
- GET  /auth/me → текущий пользователь
- GET  /oauth/:provider/start → редирект
- GET  /oauth/:provider/callback → устанавливает сессию, редиректит в /

Пользователи/группы/проекты (админка):
- GET/POST /api/users, GET/PATCH/DELETE /api/users/:id
- GET/POST /api/groups, PATCH/DELETE /api/groups/:id, POST /api/groups/:id/members
- GET/POST /api/projects, PATCH/DELETE /api/projects/:id, POST /api/projects/:id/members

Борды/листы/карточки:
- Перевести борды под проекты: GET /api/projects/:id/boards … (сохранить старые маршруты временно, объявив устаревшими)
- PATCH /api/cards/:id { assignee_id? } — назначение/сброс

SSE:
- /api/boards/:id/events — проверка прав: член проекта с ролью ≥ Viewer.

## Middleware/безопасность
- Cookie: httpOnly, Secure (в проде), SameSite=Lax.
- CSRF: для форм с изменениями — токен (кроме JSON с SameSite=Lax можно отложить до продовой фазы).
- Пароли: bcrypt (cost 12). Ограничение частоты попыток входа (rate‑limit).
- Валидация входных данных (email, длины, роли).
- Логи входа/выхода (минимум в slog).

## Фронтенд
Страницы/диалоги:
- /login — вход + ссылка на регистрацию и кнопка OAuth.
- /register — регистрация.
- Админка /admin: пользователи, группы, проекты (список, создать, изменить, удалить, добавить в группу/проект). Простые таблицы + диалоги на <dialog>.
- В карточке: поле «Исполнитель» (select с участниками проекта) + быстрый сброс. SSE обновляет исполнителя у других клиентов.

Хранение сессии:
- Cookie от сервера; фронту достаточно GET /auth/me для получения текущего пользователя при загрузке.
- Перенаправление неаутентифицированных на /login.

## Пошаговый план (итерации)
1) Фаза A — каркас и миграции
- Добавить миграции для users, oauth_accounts, sessions, groups, user_groups, projects, project_members, изменения boards/cards/comments.
- Не подключать их в выборки, чтобы не ломать существующий функционал.
- Добавить в README раздел «Аутентификация (в разработке)».

2) Фаза B — базовая аутентификация (email+пароль)
- Эндпоинты /auth/register, /auth/login, /auth/logout, /auth/me.
- Middleware проверки сессии; закрыть изменения (POST/PATCH/DELETE) авторизацией.
- Простые страницы /login, /register.

3) Фаза C — OAuth (GitHub)
- Провайдер GitHub: настройка client_id/secret через env.
- Маршруты start/callback, связывание с user (по email, либо новый user).

4) Фаза D — проекты и роли
- Ввести projects + members + роли; привязать boards к project_id.
- Миграция существующих досок в единый «Default Project».
- Ограничить доступ к бордам/спискам/карточкам по членству.

5) Фаза E — назначение исполнителя
- Поле assignee_id у карточки; PATCH обновление; отображение в UI.
- В админке — просмотр пользователей/статусов.

6) Фаза F — группы пользователей
- CRUD групп, назначение пользователей в группы.
- Привязка групп к проектам (наследуемые роли) — опционально, можно сначала использовать прямых членов.

7) Фаза G — админка
- UI /admin: список пользователей, групп, проектов; добавление/удаление/редактирование; смена ролей членов проекта.
- Только для is_admin=true.

8) Фаза H — безопасность и UX‑полировка
- Rate limiting на /auth/*.
- Сброс пароля (email — в dev: выводить magic‑link в логи).
- UX: аватар/имя пользователя в хедере, выход.

9) Фаза I — многоисполнителей на карточке (опционально)
- card_assignees(user_id, card_id) и UI с несколькими аватарками.

## Переменные окружения (новые)
- AUTH_SESSION_SECRET — ключ подписи/генерации токенов сессии.
- OAUTH_GITHUB_CLIENT_ID, OAUTH_GITHUB_CLIENT_SECRET, OAUTH_GITHUB_REDIRECT_URL
- (доп.) OAUTH_GOOGLE_CLIENT_ID/SECRET/REDIRECT_URL

## Тестирование
- Unit: хэширование пароля, создание сессии, middleware.
- Integration: register/login/logout flow, OAuth callback.
- Permission tests: доступ к /api/boards/:id и SSE.

## Риски и смягчение
- Ломающие изменения API/схемы — ввести флаги совместимости и поэтапные миграции (Default Project).
- Безопасность cookie в dev (без HTTPS) — SameSite=Lax, Secure=off.
- OAuth callback URIs — документировать настройки в README.

## Критерии готовности фазы
- Фаза B: можно создать пользователя, войти/выйти, видеть свои борды.
- Фаза D: доски изолированы по проектам и ролям.
- Фаза E: исполнитель карточки отображается и меняется, события по SSE.
- Фаза G: админ видит и управляет пользователями/проектами.
