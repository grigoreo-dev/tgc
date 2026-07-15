# tgc — Telegram CLI для агентов (v1) — Design Spec

**Дата:** 2026-07-13
**Статус:** утверждён (brainstorming завершён)
**Репозиторий:** `grigoreo-dev/tgc` (workspace path: `projects/tgc`)

## 1. Обзор и цели

`tgc` — терминальный Telegram-клиент на Go, спроектированный agent-first: основной
потребитель — AI-агенты (O‍penCode и автономные), вторичный — человек.

- Основа: **gotgproto** (обёртка над gotd/td) + прямой доступ к raw **gotd/td**
  API (`client.API()`) там, где обёртки не хватает (RichMessage, тонкости истории).
- Один статический бинарник `tgc`.
- Самостоятельный CLI-дизайн (не копирует `tg`/`tgbot`); в перспективе заменяет
  оба этих инструмента, skills обновляются после готовности.
- Серверные режимы — webhook-шлюз, MCP-сервер, Bot API-совместимый HTTP API —
  **вне scope v1**; это отдельные будущие спеки поверх того же ядра. Ядро
  проектируется как переиспользуемая Go-библиотека (`internal/` → выделение в
  публичные пакеты при необходимости), чтобы серверные режимы не требовали
  переписывания.

### Режимы работы

| Режим | Логин | Статус в v1 |
|---|---|---|
| user-bot (MTProto, личная сессия) | телефон + код + 2FA | первичен, полный функционал |
| telegram-bot | bot-токен | тот же CLI; команды, недоступные ботам, возвращают структурированную ошибку `{"error":"bot_unsupported",...}` |

## 2. Профили и аутентификация

### Хранение

```
~/.config/tgc/
├── config.toml              # default_profile = "...", опц. api_id/api_hash
└── profiles/
    └── <name>/
        └── session.db       # SQLite: сессия gotgproto + peer cache + кэш диалогов
```

- **Дефолтный профиль невидим**: с одним аккаунтом пользователь про профили не
  знает (`tgc chats` просто работает).
- Выбор профиля: флаг `--profile <name>` или env `TGC_PROFILE`.
- Модель как у kubectl contexts / aws profiles / gh accounts.

### API-креденшелы

Только собственные api_id/api_hash пользователя (my.telegram.org). Источники по
приоритету: env `TGC_API_ID`/`TGC_API_HASH` → конфиг профиля (сохраняются при
`auth login`). Никаких вшитых в бинарник креденшелов.

### Команды auth

```
tgc auth login [--profile X]              # интерактив: телефон → код → 2FA
tgc auth login --bot-token 123:ABC        # неинтерактивный бот-логин
tgc auth export [--profile X] [-o file]   # выгрузка сессии (gotd session JSON / base64)
tgc auth import [file]                    # подселение сессии; также env TGC_SESSION
tgc auth list                             # профили и их статус
tgc auth logout [profile]
```

Интерактивный логин выполняется человеком один раз; в headless-окружение сессия
переносится через export/import. Неинтерактивного степового логина по номеру в
v1 нет.

## 3. Команды v1

### Сводка

```
tgc auth      login / export / import / list / logout
tgc chats     [--limit 50] [--type user|group|channel] [--fresh]
tgc info      <selector>
tgc members   <selector> [--limit N]           # read-only список участников
tgc search    <query> [--messages] [--limit N]
tgc read      <selector> [фильтры]
tgc context   <selector> <msg_id> [--radius 10]
tgc send      <selector> <text|-> [опции]
tgc edit      <selector> <msg_id> <text>
tgc delete    <selector> <msg_id>... [--for-me]
tgc forward   <from> <msg_id> <to>
tgc download  <selector> <msg_id> [-o path] [--stdout]
```

### Селектор чата/пользователя

Универсальный, принимается всеми командами: `@username` | числовой ID | номер
телефона | fuzzy-поиск по имени (`tgc send "Вася Пупкин" "текст"`). При
неоднозначности — ошибка со списком кандидатов (id, username, тип). Резолюция
идёт сперва по локальному кэшу (peer cache + кэш диалогов), к API — только при
промахе.

`tgc search <query>` — явный поиск кандидатов по контактам/диалогам, возвращает
тех же кандидатов JSONL-ом. `--messages` переключает на глобальный серверный
поиск по сообщениям.

### Чтение

```
tgc read <chat>                          # последние 20, новые сверху
tgc read <chat> --limit 100
tgc read <chat> --before <id> | --after <id>       # пагинация по msg_id
tgc read <chat> --since 2026-07-01 --until 2026-07-10
tgc read <chat> --from @user
tgc read <chat> --search "запрос"                  # серверный поиск в чате
tgc context <chat> <msg_id> [--radius 10]
```

Поля сообщения (JSONL): `id, chat_id, sender_id, sender_name, sender_username,
date, text, reply_to, media{type,file_name,size,mime}, edited, fwd_from`.
`sender_*` берутся из ответа истории без дополнительных резолвов.

### Отправка и операции

```
tgc send <chat> "текст"                            # Markdown по умолчанию
tgc send <chat> "текст" --reply <msg_id>
tgc send <chat> -                                  # текст из stdin
tgc send <chat> --file report.pdf --caption "отчёт"
tgc send <chat> --file pic.png --as-photo          # форсировать фото vs документ
tgc send <chat> --file a.jpg --file b.jpg --file c.mp4 --caption "альбом"
tgc edit <chat> <msg_id> "новый текст"
tgc delete <chat> <msg_id> [<msg_id>...]           # дефолт: удалить у всех
tgc delete <chat> <msg_id> --for-me                # только у себя
tgc forward <from_chat> <msg_id> <to_chat>
```

- **Media group**: несколько `--file` → один альбом, максимум 10 (больше —
  ошибка с подсказкой разбить). `--caption` цепляется к первому элементу.
  Несовместимые типы в группе (документ+фото) — ошибка API пробрасывается
  честно. Ответ — JSONL: строка на каждое сообщение группы (свой `message_id`,
  общий `grouped_id`).
- Тип файла определяется по mime; `--as-photo` форсирует отправку картинки как
  фото (со сжатием), иначе картинка уходит документом.
- Ответ send/edit — JSON с `message_id`, `chat_id`, `date` (для цепочек
  reply/edit).

### Скачивание

```
tgc download <chat> <msg_id> [-o путь] [--stdout]
```

Дефолт — текущая директория, оригинальное имя файла. `-o` — файл или
директория. Успех — JSON в stdout: `{path, size, mime, file_name}`.
`--stdout` — сырые байты файла в stdout вместо JSON (для пайпов); успех/ошибка
определяются по exit code, ошибка — как обычно JSON в stderr.

### Инфо

```
tgc chats [--limit 50] [--type user|group|channel] [--fresh]
tgc info <selector>        # тип, id, title/имя, username, число участников, описание
tgc members <group>        # участники: id, имя, username, статус (admin/member)
```

## 4. Формат вывода (контракт)

Инструмент agent-first, вывод — API-контракт:

- **stdout**: только полезный результат. Компактный JSON (без пробелов); списки
  — **JSONL** (одна строка = один объект): стримится и режется `head`-ом.
- **stderr**: всё остальное — ошибки, логи, прогресс.
- **Ошибки**: структурированный JSON в stderr
  (`{"error":"<code>","message":"...","...детали"}`) + ненулевой exit code.
- **`--pretty`**: человекочитаемый вид (таблицы, цвета); цвета подавляются при
  non-TTY и `NO_COLOR`.
- Все list-команды имеют `--limit` и пагинацию.

## 5. Форматирование исходящих сообщений

- **Markdown по умолчанию** — агентам нативно. `--plain` — отправить как есть.
- Транслятор Markdown работает в два уровня:
  - инлайн-разметка (bold, italic, code, ссылки) → Telegram entities;
  - блочная разметка (заголовки, таблицы, списки, цитаты, код-блоки) →
    **RichMessage** (Bot API 10.1, июнь 2026: RichBlock*/RichText*-структуры),
    где отправитель/слой это поддерживает.
- **Фолбэк**: если RichMessage недоступен (user-режим без поддержки на текущем
  слое MTProto, старые серверы) — деградация до плоских entities (заголовки →
  жирный, таблицы → моноширинный текст).
- `--rich <json>` — экспертный режим: готовая RichMessage-структура JSON-ом,
  без Markdown-транслятора.
- Открытый вопрос реализации (проверяется в первом спайке): доступность
  RichMessage с user-аккаунта через MTProto; gotd layer 227 (v0.160.0,
  2026-07-07) свежее Bot API 10.1, типы ожидаются в наличии. Если для
  user-режима недоступно — RichMessage работает только в bot-режиме, user
  получает фолбэк.
- Стриминг-драфты (`sendRichMessageDraft`) — не в v1; архитектура транслятора
  не должна им противоречить (блоки генерируются последовательно).

## 6. Бережное отношение к Telegram API

- **Peer cache** (gotgproto, SQLite): резолв `@username` → API только при
  первом обращении, далее из базы. `contacts.resolveUsername` — самый
  лимитируемый метод, это главная защита.
- **Кэш диалогов**: `tgc chats` кэширует список с TTL ~5 минут в session.db;
  `--fresh` форсирует обновление. Fuzzy-селекторы ищут сперва по кэшу.
- **FLOOD_WAIT**: middleware `floodwait.Waiter` (gotd). Если `retry_after` ≤
  30с — ждём и повторяем прозрачно; больше — структурированная ошибка
  `{"error":"flood_wait","retry_after":X}`, решение за агентом.
- **Минимум вызовов по дизайну**: `read` не резолвит отправителей отдельно,
  `info` отвечает из кэша при возможности.
- Сообщения **не кэшируются** (мутабельны, инвалидация не окупается).

## 7. Вне scope v1

Управление группами (создание, добавление/удаление участников, инвайт-ссылки),
реакции, опросы, стикеры, голосовые, админка каналов, топики форумов, контакты
(управление), папки, архив, realtime-подписка (`listen`), стриминг-драфты,
webhook-режим, MCP-сервер, Bot API-совместимый HTTP-шлюз.

## 8. Структура проекта

```
projects/tgc/
├── cmd/tgc/                 # main, CLI-обвязка (cobra)
├── internal/
│   ├── client/              # инициализация gotgproto, профили, floodwait
│   ├── resolve/             # универсальный селектор + кэш кандидатов
│   ├── markup/              # Markdown → entities / RichMessage + фолбэк
│   ├── output/              # JSONL / pretty, контракт ошибок
│   └── commands/            # реализация команд
├── go.mod
└── .github/workflows/ci.yml
```

## 9. Тестирование

- **Юнит**: парсер селекторов, Markdown-транслятор (entities и RichMessage,
  фолбэк), кэш-логика (TTL, инвалидация), контракт вывода/ошибок.
- **Интеграционные**: против реального тестового аккаунта и тестового бота,
  отдельный профиль, ручной запуск (`go test -tags integration`); MTProto
  полноценно не мокается.
- **CI** (GitHub Actions): build + `go vet` + юнит-тесты на push в main и PR.

## 10. Решения и их обоснование (лог брейншторма)

| Решение | Выбор | Почему |
|---|---|---|
| Декомпозиция | ядро+CLI сначала, серверные режимы потом | один спек не должен покрывать 5 подсистем |
| Режимы | оба, user-bot первичен | gotgproto даёт бот-логин почти бесплатно |
| Отношение к tg/tgbot | самостоятельный, будущая замена | свобода дизайна, skills обновим позже |
| Объём v1 | auth+чаты+сообщения+файлы (+members read-only), без управления группами | баланс ценность/срок |
| Вывод | компактный JSONL, `--pretty`, ошибки в stderr | agent-first стандарт 2025–26 (подтверждено ресёрчем) |
| Селекторы | универсальные + `tgc search` | минимум трения для агента |
| Креденшелы | только свои, env/конфиг | легально и просто |
| Форматирование | Markdown → entities/RichMessage, `--plain`, `--rich` | RichMessage (Bot API 10.1) создан под AI-ботов |
| Библиотека | gotgproto + raw gotd | сессии/peer cache из коробки, raw-доступ есть |
| Имя | `tgc` | коротко, не конфликтует |
| Логин | интерактив + export/import сессии | степовый headless-логин не нужен в v1 |
| Download | CWD + оригинальное имя, `-o`, `--stdout` | предсказуемо для агента |
| Delete | дефолт «у всех», `--for-me` | агентский сценарий |
| Rate limits | peer cache + кэш диалогов TTL 5м + floodwait.Waiter | «не обижать телегу» |
