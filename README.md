# A2A/MCP System Orchestrator

Central SaaS platform for a multi-agent intelligent system that automates customer service request processing in dispatch centers. This repository contains the LLM orchestrator, API gateway, discovery service, and agent registry — the "brain" of the entire system.

## 🔗 Related Repositories

This is one of three repositories that make up the complete system:

| Repository | Purpose |
|-----------|---------|
| **a2a-mcp-system-orchestrator** (this repo) | Central SaaS platform with LLM-orchestrator, discovery service, and agent registry |
| [a2a-mcp-system-agents](https://github.com/BorisLarkin/a2a-mcp-system-agents) | AI agents (classifier, researcher, generator) and MCP tools (encoder, Qdrant, Ollama) |
| [a2a-mcp-system-client](https://github.com/BorisLarkin/a2a-mcp-system-client) | Client infrastructure: local proxy, database, web dashboard, web widget |

## 🎯 Overview

This repository implements the **orchestration layer** — the central component that coordinates the entire request processing pipeline. It receives requests from client infrastructure, dynamically plans the processing chain using an LLM, and executes it by calling specialized AI agents through the A2A protocol.

The orchestrator is built on a **multi-agent paradigm with LLM-centric orchestration**. Unlike traditional systems with hard-coded processing logic, the orchestrator does not follow a fixed route. Instead, it asks the language model to analyze each request and generate an optimal plan — a sequence of agent calls tailored to the specific content of the request.

## ✨ Key Features

- **LLM-centric dynamic planning** — the processing chain is generated per-request, not predefined
- **Hybrid discovery service** — agents are registered in PostgreSQL and periodically polled for their current status and skills
- **Schema-driven payload construction** — the orchestrator builds input data for each agent based on its Agent Card (input/output schemas)
- **Multi-tenant agent registry** — system agents (available to all dispatch centers) and client-specific agents (isolated per dispatcher)
- **Fallback chain** — if an agent is unavailable, the orchestrator retries, then falls back to local LLM or predefined default behavior
- **Full audit logging** — every A2A call is recorded with complete request/response bodies
- **Knowledge base API** — allows the client to add documents to the vector database for RAG-based search

## 🧩 Core Components

### LLM Orchestrator (`internal/services/orchestrator.go`)

The heart of the system. Its `Process` method:

1. Receives the request text and dispatcher configuration
2. Queries the discovery service for available agents
3. Forms a prompt for the LLM with the agent list and their skills
4. Receives a dynamic plan (e.g., "1. classifier:classify → category; 2. researcher:search → solutions; 3. generator:generate_response → answer")
5. Passes the plan to the Plan Executor
6. Compares the classification confidence with the threshold from the dispatcher configuration
7. Decides: auto-response or escalation to operator

### Plan Executor (`internal/services/plan_executor.go`)

Executes the LLM-generated plan step by step:

- Parses the plan text into structured steps
- For each step, finds the appropriate agent through discovery
- Constructs the payload based on the agent's input schema and the context of previous steps
- Calls the agent via A2A
- Collects results and passes them as context to subsequent agents
- Handles errors with retries and fallback strategies

### Hybrid Discovery Service (`internal/discovery/hybrid.go`)

Manages the agent registry:

- Loads agents from PostgreSQL on startup
- Periodically polls each agent's `/.well-known/agent.json` to update skills and status
- Marks agents as online/offline based on health checks
- Filters agents by dispatcher ID: system agents (NULL dispatcher) are visible to all, client-specific agents only to their owner

### A2A Client (`internal/services/a2a_client.go`)

Custom implementation of the A2A protocol for agent communication:

- Sends tasks via `POST /tasks/send` to agents
- Receives responses with `task_id` and `status`
- Supports retry logic and timeouts
- Logs all calls to the database

### API Handlers

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/process-ticket` | POST | Main endpoint for request processing (called by client infrastructure) |
| `/api/v1/agents` | GET | List all agents for the current dispatcher |
| `/api/v1/agents` | POST | Register a new agent (validates the agent's card first) |
| `/api/v1/agents/:id` | DELETE | Remove an agent |
| `/api/v1/dispatchers/validate` | POST | Validate API key and dispatcher ID |
| `/api/v1/admin/dispatchers` | POST | Register a new dispatch center (SaaS admin) |
| `/api/v1/knowledge` | POST | Add a document to the knowledge base |

## 🔌 Protocols

### A2A (Agent-to-Agent)

**This is a custom lightweight implementation** inspired by the open A2A specification. It uses the same core concepts — task-based interaction, agent cards, standard endpoints — but is adapted for synchronous processing:

- Agents expose `POST /tasks/send` with `{skill_id, input}` → `{task_id, status, output}`
- Agent cards are served at `/.well-known/agent.json` with capabilities and skills
- The orchestrator constructs payloads based on agent input schemas (schema-driven)
- No streaming, no asynchronous task polling — processing is synchronous with a 30-second timeout

This design choice keeps the orchestrator simple and fast while preserving the key benefit of A2A: **the orchestrator does not need to know the internal implementation of agents**.

### MCP (Model Context Protocol)

**Also a custom implementation**, inspired by the open MCP specification from Anthropic. The orchestrator does not call MCP tools directly — that is the role of agents. However, the orchestrator provides a Knowledge API that internally invokes MCP tools (`sentence-encoder` for embeddings, `qdrant-search` for vector storage) when the client adds documents to the knowledge base.

The implementation follows JSON-RPC 2.0 with `tools/list` and `tools/call` methods, sufficient for the system's needs without the complexity of the full specification.

## 🏗️ Why Multi-Agent?

Traditional approaches to customer service automation face a fundamental trade-off:

- **Rule-based chatbots** are fast but cannot handle complex or unexpected requests
- **Monolithic AI platforms** are powerful but inflexible and expensive
- **Custom integrations** are flexible but require significant engineering effort

The multi-agent paradigm with LLM-centric orchestration breaks this trade-off:

- **Specialization** — each agent solves a narrow problem exceptionally well (classification, search, generation)
- **Dynamic composition** — the orchestrator combines agents into a chain tailored to each request
- **Extensibility** — new agents can be added without modifying the orchestrator
- **Isolation** — agents can be deployed independently, even on different servers

## 📂 Repository Structure

```
a2a-mcp-system-orchestrator/
├── cmd/
│   └── orchestrator/
│       └── main.go               # Entry point
├── internal/
│   ├── config/
│   │   └── config.go             # Configuration loading
│   ├── db/
│   │   ├── models.go             # GORM models (Dispatcher, Agent, Ticket, A2ACall)
│   │   └── postgres.go           # Database connection
│   ├── discovery/
│   │   ├── client.go             # Discovery interface
│   │   ├── hybrid.go             # Hybrid discovery (DB + agent cards)
│   │   ├── models.go             # Agent and Skill models
│   │   └── static.go             # Static discovery (fallback)
│   ├── handlers/
│   │   ├── agent.go              # Agent registration/list/deletion
│   │   ├── dispatchers.go        # Dispatch center registration/validation
│   │   ├── knowledge.go          # Knowledge base API
│   │   └── ticket.go             # Main process-ticket endpoint
│   ├── models/
│   │   └── a2a.go                # A2A protocol models
│   └── services/
│       ├── a2a_client.go         # A2A client for agent calls
│       ├── agent_client.go       # Legacy agent client (fallback)
│       ├── agent_repository.go   # Agent database operations
│       ├── dispatcher_service.go # Dispatcher database operations
│       ├── llm_client.go         # LLM client (Ollama)
│       ├── orchestrator.go       # Main orchestrator logic
│       ├── plan_executor.go      # Plan parsing and execution
│       └── ticket_repository.go  # Ticket and A2A call logging
├── migrations/
│   └── 001_full_schema.sql       # Database schema
├── config/
│   └── agents.json               # Static agent list (fallback)
├── scripts/
│   ├── prewarm.sh                # Model warm-up script
│   └── test_connection.py        # Connection test script
├── Dockerfile
└── docker-compose.yml
```

## 🚀 Quick Start

### Prerequisites

- Docker 24+ and Docker Compose 2.20+
- NVIDIA GPU with at least 16 GB VRAM (for LLM inference)
- NVIDIA Container Toolkit
- Tailscale (for secure connection to agents and client infrastructure)

### Installation

```bash
# 1. Clone the repository
git clone https://github.com/BorisLarkin/a2a-mcp-system-orchestrator.git
cd a2a-mcp-system-orchestrator

# 2. Configure environment
cp .env.example .env
nano .env  # Set database password, API keys, model name

# 3. Start the orchestrator with dependencies
docker-compose up -d --build

# 4. Check status
docker-compose ps
```

### Testing the Connection

```bash
# Check orchestrator health
curl http://localhost:8080/health

# Test request processing (requires valid API key)
curl -X POST http://localhost:8080/process-ticket \
  -H "Content-Type: application/json" \
  -H "X-API-Key: sk_live_your_key" \
  -d '{
    "text": "У меня не работает интернет",
    "dispatcher_id": "uuid-диспетчерской"
  }'

# Test agent listing
curl http://localhost:8080/api/v1/agents \
  -H "X-API-Key: sk_live_your_key"
```

## 🧠 LLM for Planning

The orchestrator uses **Saiga Llama 3 8B** (`ilyagusev/saiga_llama3:latest`) for dynamic planning. The model receives:

- The request text
- The dispatcher configuration (communication style, threshold, company context)
- The list of available agents with their skills

And returns a plan in the format:

```
1. classifier:classify → category
2. researcher:search → solutions
3. generator:generate_response → answer
```

For corporate-related queries (e.g., "What are your tariffs?"), the LLM might generate a different plan:

```
1. corporate:company_info → info
2. generator:generate_response → answer
```

This demonstrates the flexibility of dynamic planning — the same system handles both technical and business queries without any code changes.

## ⚙️ Configuration

Key environment variables (`.env`):

| Parameter | Description | Default |
|-----------|-------------|---------|
| `DB_HOST` | PostgreSQL host | `postgres` |
| `DB_NAME` | Database name | `orchestrator` |
| `DB_USER` | Database user | `orchestrator` |
| `DB_PASSWORD` | Database password | — |
| `LLM_MODEL` | LLM model for planning | `ilyagusev/saiga_llama3:latest` |
| `LLM_ENDPOINT` | Ollama API endpoint | `http://ollama:11434/api/chat` |
| `DISCOVERY_TYPE` | Discovery type (`hybrid` or `static`) | `hybrid` |
| `SAAS_ADMIN_KEY` | API key for SaaS admin endpoints | — |
| `ENCODER_MCP_URL` | MCP sentence encoder URL | `http://sentence-encoder:9102` |
| `QDRANT_MCP_URL` | MCP Qdrant search URL | `http://qdrant-search:9103` |

## 🔒 Performance Considerations

- **Concurrency**: Go goroutines handle multiple client requests in parallel, each processing chain runs in a separate goroutine
- **Timeouts**: A2A calls have a 30-second timeout; LLM planning has a configurable timeout
- **Retries**: Agent calls are retried up to 3 times with exponential backoff
- **Fallbacks**: If the LLM is unavailable, a default plan is used; if an agent is unavailable, the orchestrator falls back to direct LLM calls or predefined defaults
- **GPU utilization**: The Ollama container uses NVIDIA GPU with 16 GB VRAM, loading the model into memory once and keeping it warm

## 📝 License

This project is part of a bachelor's thesis. All rights reserved by Boris Larkin, 2026.

## 📞 Contacts

- **Author:** Boris Larkin
- **Email:** borislarkin18@mail.ru
- **GitHub:** [BorisLarkin](https://github.com/BorisLarkin)
