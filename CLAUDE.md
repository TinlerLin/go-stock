# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

### One-Click Production Build

```bash
wails build
```

Compiles Go backend + Vue frontend → single portable executable at `build/bin/go-stock.exe`.

### Wails (Desktop App)
- `wails dev` — Start in development mode (hot-reload Go + Vite dev server on :5173)
- `wails build` — Production build for current platform
- `wails build --clean` — Clean build (clear cache first)
- `wails build --platform windows/amd64` — Cross-platform build
- `wails doctor` — Check Wails environment and dependencies
- `wails build -ldflags "-X main.Version=v1.2.3 -X main.VersionCommit=abc1234 -X main.BuildKey=<hex-aes-key> -X main.OFFICIAL_STATEMENT=..."` — Production build with injected variables

### Platform Build Scripts
- `bash scripts/build-windows.sh` — Windows amd64 build
- `bash scripts/build-macos.sh` — macOS (Intel + Apple Silicon)
- `bash scripts/build-linux.sh` — Linux build

### Frontend (Vue 3 + Vite)
- `cd frontend && npm install` — Install dependencies
- `cd frontend && npm run dev` — Vite dev server (port 5173)
- `cd frontend && npm run build` — Production build to `frontend/dist/`

### Go
- `go test ./...` — Run all tests
- `go test -v -run TestName ./backend/...` — Run a single test
- `go build ./backend/...` — Compile backend packages only

### AI Assistant Web (standalone mode)
- `cd ai-assistant-web/cmd/ai-assistant-web && go run main.go` — Run the web server standalone (port from `AI_ASSISTANT_WEB_ADDR` env, default `:18888`)

## Architecture

### Project Structure

```
go-stock/                  # Wails root — `main.go` is the desktop app entry point
├── main.go                # Wails app bootstrap, DB init, AutoMigrate, embedded assets
├── app.go                 # App struct — all Wails-bound methods exposed to frontend
├── app_common.go          # Cross-platform shared logic (trading time, sentiment, agent chat, etc.)
├── app_windows.go         # Windows-specific (notifications, toast, tray)
├── app_darwin.go          # macOS-specific
├── app_linux.go           # Linux-specific
├── backend/
│   ├── agent/             # AI agent system built on CloudWeGo Eino framework
│   │   ├── agent.go       # React & PlanExecute agent modes, complexity classification
│   │   ├── agent_api.go   # ChatWithAgent, StockAiAgent API
│   │   ├── chat_memory.go # Conversation memory (SQLite-backed)
│   │   ├── chat_model_factory.go  # Multi-provider LLM model factory
│   │   ├── tools/         # Agent tool functions (stock lookup, K-line, news, MCP, etc.)
│   │   └── cron_task_api.go  # Scheduled AI analysis tasks
│   ├── data/              # ~60+ files: API clients for financial data sources
│   │   ├── settings_api.go    # Settings CRUD, AI configs (stores API keys in SQLite)
│   │   ├── openai_api.go      # OpenAI-compatible client (masks keys in String())
│   │   ├── openai_stream.go   # SSE streaming for LLM chat
│   │   ├── openai_tools.go    # AI tool-calling integration
│   │   ├── stock_data_api.go  # Sina/TDX/Tushare stock data fetchers
│   │   ├── eastmoney_kline_api.go  # EastMoney K-line data
│   │   ├── market_news_api.go # News scraping (uses Otto JS engine for callback parsing)
│   │   ├── tool_registry.go   # Tool registration for AI agent
│   │   ├── tool_kline.go, tool_*.go  # Individual tool definitions
│   │   ├── sponsor_vip.go     # AES-decrypt sponsor codes to determine VIP level
│   │   └── crawler_api.go     # Chromedp-based web scraping
│   ├── db/
│   │   └── db.go           # GORM + SQLite init (WAL mode, busy_timeout, 5 max conns)
│   ├── models/
│   │   └── models.go       # All GORM models (AIResponseResult, CronTask, MCPServer, etc.)
│   ├── logger/
│   │   └── lgo.go          # Zap logger with lumberjack rotation (./logs/)
│   └── util/               # HTML→Markdown, struct→Markdown converters
├── frontend/               # Vue 3 SPA (Naive UI + TDesign Chat + ECharts)
│   └── src/
│       ├── router/router.js   # Hash-mode router, 9 routes
│       └── components/        # ~45 Vue components (stock, fund, market, agent-chat, etc.)
├── ai-assistant-web/       # Standalone Go HTTP server + Vue frontend (AI assistant feature)
│   ├── server.go           # HTTP server (port 18888), no auth on most endpoints, wildcard CORS
│   └── frontend/           # Separate Vue frontend for the AI assistant web UI
├── build/                  # Icons, screenshots, NSIS installer scripts, platform plists
├── scripts/                # Platform-specific build shell scripts (wails build wrappers)
├── data/dict/              # Chinese word segmentation dictionaries (gse/结巴)
├── go.mod                  # Go 1.26, Wails v2.11.0, GORM, Chromedp, Eino, resty
└── wails.json              # Wails project config (app name, build commands)
```

### Key Architectural Patterns

**Desktop App Flow**: The `App` struct in `app.go` + `app_common.go` exposes exported methods that are automatically bound to the frontend via Wails. The frontend calls `window.go.main.App.MethodName()` to invoke them. Results are returned synchronously or emitted as events via `runtime.EventsEmit()`.

**AI Agent Flow**: `ChatWithAgent()` in `app_common.go` calls `agent.StockAiAgentApi.ChatWithContext()`, which creates a React or PlanExecute agent (Eino framework). The agent has access to registered tools (stock data, K-line, news, MCP). Results stream back via Go channels → frontend events.

**Data Flow**: Frontend calls Wails-bound App methods → methods delegate to `backend/data` API clients → external financial APIs (Sina, EastMoney, TDX, Tushare) or local SQLite. HTTP clients are `go-resty/resty`. Headless browser scraping uses `chromedp`.

**Database**: SQLite via GORM (`github.com/glebarez/sqlite`). DB file at `data/stock.db` with WAL journal mode. All models auto-migrated in `main.go:AutoMigrate()`. API keys and settings stored as JSON in the `ai_config` and `settings` tables.

**VIP System**: Sponsor codes are AES-ECB encrypted hex strings. The `BuildKey` (injected at build time) is the AES key. `sponsor_vip.go` decrypts and validates sponsor info to determine VIP level (0/1/2).

### Platform-Specific Code
- Files with `_darwin.go` / `_windows.go` / `_linux.go` suffixes contain platform-specific implementations.
- macOS notifications use `osascript` (AppleScript). Windows notifications use `go-toast/toast`.
- `stock_data_api_*.go` files have platform-specific Chromedp browser detection logic.

### Key Dependencies
- **Wails v2.11.0**: Desktop framework binding Go ↔ WebView
- **CloudWeGo Eino v0.8.11**: AI agent orchestration (React/PlanExecute, multi-model support)
- **go-resty/resty v2**: HTTP client for external API calls
- **chromedp**: Headless Chrome/Edge for web scraping (market news, K-line data)
- **GORM + glebarez/sqlite**: ORM with pure-Go SQLite driver
- **Naive UI 2.x + TDesign Vue Next**: Vue 3 UI component libraries
- **ECharts 5 + lightweight-charts**: Financial charting
