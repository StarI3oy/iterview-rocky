# Планка 400к+

Локальный тренажёр к собеседованиям: Flutter (телефон и широкий Web) и один Go-сервер. Карточки квизов, открытых вопросов и кодовых задач читаются из SQLite. Код на ревью уходит в OpenRouter, локальной песочницы компиляции нет.

Исходный дамп лежит в `prep_app/data/prep_app_knowledge_export.json` и читаемой книгой в `prep_app/data/prep_app_knowledge_base.md`. При старте сервер ищет `/home/shaman/prep_app_knowledge_export.json` (или `KNOWLEDGE_JSON`) и обновляет карточки. Повторный импорт не сбрасывает прогресс: `times_asked`, `last_result`, `closed`, `status`.

## Запуск

```bash
export OPENROUTER_API_KEY=...          # необязательно: без ключа ревью и пояснение поиска выключены
export OPENROUTER_MODEL=openai/gpt-4o-mini
./prep_app/run.sh
```

Сервер слушает `:8080` и отдаёт API вместе со статикой `prep_app/app/build/web`. С телефона в той же сети откройте `http://<ip-компьютера>:8080`. Узкая ширина (меньше 768px) — мобильный режим, широкая — две панели для кода.

Базы по умолчанию:

- `INTERVIEW_DB=/home/shaman/interview_questions.db`
- `HARD_CONCEPTS_DB=/home/shaman/hard_concepts.db`

На телефоне как отдельное приложение: `flutter run --dart-define=API_BASE=http://<ip>:8080` из `prep_app/app`.

## Контракт карточки

Таблица `prep_items` в базе вопросов. Полный DDL — `prep_app/backend/schema.sql`.

Утренний спринт берёт одну дорожку по протоколу: чётный день — секции компаний, нечётный — алгоритмические паттерны (код), воскресенье — system design или легенда.

- `item_type`: `quiz`, `flashcard` или `code`
- `track`: `dumps`, `patterns`, `system_design`, `legend`; вопросы библиотеки — `library`, разобранные концепты — `concept`
- `block`: `go`, `db_kafka`, `algo`, `system_design` (понимаются и короткие алиасы вроде `kafka`, `postgres`, `sd`)
- `quiz`: `options` — JSON-массив строк, `correct_index`, `explanation`
- `flashcard`: `answer_skeleton`, `checklist` — JSON-массив тезисов
- `code`: `starter_code`, `reference_code` (эталон остаётся на сервере), `review_criteria`, `language` (`go` или `sql`)

Если в уже существующей таблице есть колонки `question` / `body` и `topic` / `category`, при старте они копируются в `prompt` и `block`, только пока целевые поля пустые.

Прогресс планки: устный вопрос зачтён при последнем верном ответе, кодовая задача — когда ревью вернуло `passed: true`.

## API

- `GET /api/bar` — общий процент и 4 блока
- `GET /api/feed?mode=mobile|web&kind=sprint|code|hard`
- `POST /api/answer` — `{question_id, passed, user_answer, selected_index?}`. Для квиза вердикт считается по `selected_index`. Провал пишет строку в `hard_concepts`
- `POST /api/review` — `{question_id, code}` → `{passed, score, bugs, style_notes, follow_up_question}`
- `POST /api/search` — `{query}` ищет по вопросам и незакрытым hard concepts, при наличии ключа добавляет короткое пояснение
- `POST /api/queue` — отложить кодовую задачу с телефона
