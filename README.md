# A2A/MCP System Orchestrator

Центральный оркестратор интеллектуальной системы автоматизации обработки обращений. Координирует работу AI-агентов, планирует цепочки вызовов через LLM и обеспечивает взаимодействие с клиентскими диспетчерскими.

## 📋 Состав репозитория

```
orchestrator/
├── cmd/
│ └── orchestrator/
│ └── main.go # Точка входа
├── config/
│ └── agents.json # Статическая конфигурация агентов
├── internal/
│ ├── config/ # Загрузка конфигурации
│ ├── db/ # Работа с PostgreSQL
│ ├── discovery/ # Discovery агентов
│ ├── handlers/ # HTTP обработчики
│ └── services/ # Бизнес-логика
├── migrations/ # SQL миграции
├── scripts/ # Вспомогательные скрипты
├── .air.toml # Hot-reload для разработки
├── .env # Конфигурация
├── docker-compose.yml # Локальный запуск
├── Dockerfile # Сборка образа
├── go.mod # Зависимости Go
└── README.md
```

## 🏗️ Архитектура

Оркестратор реализует **LLM-центричную оркестрацию**:

1. **API Server** (Go + Gin) — приём запросов
2. **PostgreSQL** — хранение конфигураций диспетчерских
3. **Redis** — кэширование и rate limiting
4. **Ollama** — запуск LLM для планирования
5. **A2A Client** — вызов агентов по протоколу A2A
6. **Discovery** — обнаружение доступных агентов

## 🚀 Быстрый старт

### Предварительные требования
- Go 1.21+
- Docker и Docker Compose
- Git

### Установка и запуск

```bash
# Клонирование репозитория
git clone https://github.com/your-org/a2a-mcp-system-orchestrator.git
cd a2a-mcp-system-orchestrator

# Настройка окружения
cp .env.example .env
# Отредактируйте .env под ваши параметры

# Запуск всех сервисов
docker-compose up -d

# Проверка логов
docker-compose logs -f orchestrator
```

### Проверка работоспособности

```bash
# Health check
curl http://localhost:8080/health

# Отправка тестового запроса
curl -X POST http://localhost:8080/process-ticket \
  -H "Content-Type: application/json" \
  -d '{
    "text": "У меня не работает интернет",
    "dispatcher_id": "520e3b2d-b187-4581-9eea-a8dd26b5d737"
  }'
```

🔧 Конфигурация
Основные параметры в .env:

| Параметр | Описание | Значение по умолчанию |
| --- | --- | --- |
| `DB_HOST` | Хост PostgreSQL | `postgres` |
| `DB_NAME` | Имя БД | `orchestrator` |
| `LLM_ENDPOINT` | URL Ollama | `http://ollama:11434` |
| `LLM_MODEL` | Модель для планирования | `llama3.2` |
| `DISCOVERY_TYPE` | Тип discovery | `static` |

## 🧠 Процесс обработки запроса

1. Получение запроса — через /process-ticket

2. Загрузка конфигурации — диспетчерской из БД

3. LLM планирование — создание цепочки шагов

4. Поиск агентов — через discovery по capabilities

5. Выполнение плана — вызов агентов по A2A

6. Сбор результатов — формирование ответа

## 📡 API Эндпоинты

### `POST /process-ticket`

Request:

```json
{
  "text": "string",
  "dispatcher_id": "uuid",
  "metadata": {}
}
```

Response:

```json
{
  "ticket_id": "uuid",
  "classification": {},
  "suggested_team": "string",
  "status": "processed",
  "timestamp": "2026-03-19T10:00:00Z",
  "plan": "string",
  "execution_log": []
}
```

### `GET /health`

Проверка работоспособности сервиса.

## 🛠️ Разработка

### Hot-reload с Air

```bash
# Установка air
go install github.com/air-verse/air@latest

# Запуск с hot-reload
air -c .air.toml
```

### Добавление нового агента в discovery

1. Уточнить новый известный адрес вещания агента.

2. Удостовериться в совершении вызова на адрес и подтягивании /well-known.json от нового агента.

### 📊 Мониторинг

- Метрики Prometheus — на /metrics

- Логи в JSON — для интеграции с ELK

- Grafana дашборды — визуализация метрик

### 📄 Лицензия

Boris Larkin 2026

### 📞 Контакты

По вопросам: borislarkin18@mail.ru
