# Complete System Flow Documentation

This document describes **every layer** of the Real-Time Personalized Engagement Platform: infrastructure, data models, Kafka pipelines, each microservice’s internal logic, caching, WebSockets, frontend behavior, AI features, and the end-to-end demo sequence.

---

## Table of Contents

1. [System Overview](#1-system-overview)
2. [Infrastructure Layer](#2-infrastructure-layer)
3. [Shared Packages (`pkg/`)](#3-shared-packages-pkg)
4. [Database Design](#4-database-design)
5. [Kafka Design](#5-kafka-design)
6. [Event & Message Schemas](#6-event--message-schemas)
7. [Service 1: Dataset Replay](#7-service-1-dataset-replay)
8. [Service 2: Recommendation Engine](#8-service-2-recommendation-engine)
9. [Service 3: Engagement Orchestrator](#9-service-3-engagement-orchestrator)
10. [Service 4: Retention Analytics](#10-service-4-retention-analytics)
11. [Service 5: AI Insights](#11-service-5-ai-insights)
12. [Frontend Dashboard](#12-frontend-dashboard)
13. [End-to-End Sequence (One User Event)](#13-end-to-end-sequence-one-user-event)
14. [Consumer Groups & Parallelism](#14-consumer-groups--parallelism)
15. [Configuration Reference](#15-configuration-reference)
16. [Docker Networking](#16-docker-networking)
17. [Demo Walkthrough (Step by Step)](#17-demo-walkthrough-step-by-step)
18. [Failure Modes & Design Decisions](#18-failure-modes--design-decisions)

---

## 1. System Overview

### Purpose

Simulate a Netflix/Amazon-style **real-time personalization platform** that:

- Replays historical e-commerce behavior (Retailrocket dataset)
- Updates recommendations as events arrive
- Pushes live UI updates via WebSocket
- Computes retention and ROI metrics
- Surfaces lightweight AI insights

### Architectural Style

- **Event-driven**: Kafka is the nervous system; services are loosely coupled consumers/producers.
- **Microservice-style**: Five independent Go binaries + one React frontend.
- **Monorepo**: Single `go.mod`, shared `pkg/`, one `Dockerfile.go` parameterized by `SERVICE` build arg.

### High-Level Diagram

```
┌─────────────┐     user-events      ┌────────────────────┐
│   Dataset   │ ──────────────────►  │   Recommendation   │
│   Replay    │                      │     Service        │
└─────────────┘                      └─────────┬──────────┘
       │                                       │ recommendation-events
       │ user-events                           ▼
       ├──────────────────────────► ┌────────────────────┐
       │                            │    Engagement      │
       │                            │   Orchestrator     │──► Redis (dashboard cache)
       │                            │  + WebSocket Hub   │──► Browser (WS)
       │                            └─────────┬──────────┘
       │ user-events                           │ dashboard-events
       ▼                                       ▼
┌────────────────────┐               ┌────────────────────┐
│ Retention Analytics│──analytics──► │   AI Insights      │
└────────────────────┘    events     └────────────────────┘
       │                                       │
       ▼                                       ▼
  PostgreSQL                              PostgreSQL (SQL AI)
```

---

## 2. Infrastructure Layer

### 2.1 Zookeeper (`:2181`)

- **Image**: `confluentinc/cp-zookeeper:7.5.0`
- **Role**: Cluster coordination for Kafka (broker registration, controller election).
- **Not used directly** by application code.

### 2.2 Kafka (`:9092` host / `kafka:29092` internal)

- **Image**: `confluentinc/cp-kafka:7.5.0`
- **Listeners**:
  - `PLAINTEXT://kafka:29092` — used by containers on the Docker network
  - `PLAINTEXT_HOST://localhost:9092` — used by host-machine clients
- **Health check**: `kafka-broker-api-versions` against `localhost:9092`
- **Auto-create topics**: enabled (backup if `EnsureTopics` fails)
- **Replication factor**: 1 (single-broker dev setup)

### 2.3 PostgreSQL (`:5432`)

- **Image**: `postgres:16-alpine`
- **Credentials**: user `pep`, password `pep123`, database `pep`
- **Init on first boot** (via `/docker-entrypoint-initdb.d/`):
  1. `migrations/schema.sql` — tables + indexes
  2. `migrations/seed.sql` — categories, demo users, 15 catalog items
- **Used by**: recommendation-service (event inserts), retention-analytics (metric history), ai-insights (SQL assistant queries)

### 2.4 Redis (`:6379`)

- **Image**: `redis:7-alpine`
- **Used by**: engagement-orchestrator, retention-analytics, ai-insights
- **Keys** (see [Redis Keys](#redis-keys))

### 2.5 Frontend (`:3000` → nginx `:80`)

- **Build**: Vite production build served by nginx
- **Proxies** (in `nginx.conf`): API paths to backend containers via Docker DNS `127.0.0.11`
- **Direct API calls**: Frontend also calls `localhost:808x` via Vite env vars baked at build time

---

## 3. Shared Packages (`pkg/`)

### 3.1 `pkg/config`

Loads configuration via **Viper** with prefix `PEP_`:

| Env Variable | Default | Used By |
|--------------|---------|---------|
| `PEP_PORT` | `8080` | All HTTP services |
| `PEP_KAFKA_BROKERS` | `kafka:9092` | All Kafka services |
| `PEP_POSTGRES_DSN` | postgres connection string | recommendation, retention, ai |
| `PEP_REDIS_ADDR` | `redis:6379` | engagement, retention, ai |
| `PEP_EVENTS_CSV_PATH` | `/data/events.csv` | dataset-replay |
| `PEP_USE_MOCK_AI` | `true` | ai-insights |
| `OPENAI_API_KEY` | empty | ai-insights (optional) |

### 3.2 `pkg/logger`

- **Zap** production JSON logger
- Every log line includes `"service": "<name>"` field

### 3.3 `pkg/kafka`

| Function | Behavior |
|----------|----------|
| `NewWriter(topic)` | Sync writer, `Hash` partition balancer, `RequireOne` ack |
| `NewReader(topic, groupID)` | Consumer group reader, commits every 1s, starts at `FirstOffset` |
| `PublishJSON` | Marshal struct → Kafka message with string key |
| `PublishWithRetry` | Up to 3 attempts, exponential backoff 200ms × attempt |
| `EnsureTopics` | Creates topics with **4 partitions**, replication factor 1 |

### 3.4 `pkg/redis`

| Method | Key Pattern | TTL |
|--------|-------------|-----|
| `SetDashboard(userID, payload)` | `dashboard:user:{userId}` | 5 minutes |
| `GetDashboard(userID)` | same | — |
| `SetJSON(key, v)` | arbitrary | caller-defined |
| `GetJSON(key, dest)` | arbitrary | — |

### 3.5 `pkg/db`

- GORM + PostgreSQL driver
- `Connect(dsn)` opens DB
- `AutoMigrate` exists but **is not called** at runtime (schema comes from SQL init scripts to avoid constraint conflicts)

### 3.6 `pkg/models`

Central JSON/domain types — see [Event & Message Schemas](#6-event--message-schemas).

---

## 4. Database Design

### 4.1 Tables

| Table | Purpose |
|-------|---------|
| `users` | Demo users (ids 1–5) |
| `categories` | Hierarchy: Electronics, Computers, Phones, Fashion, Home, Sports |
| `content_catalog` | 15 products (item_id 1001–1015) with title, category, price, image_url |
| `user_events` | Append-only event log from recommendation service |
| `recommendations` | Schema ready; not heavily written in current flow |
| `analytics_metrics` | Snapshot history every 3 seconds from retention service |

### 4.2 Indexes

- `user_events`: `user_id`, `event_type`, `timestamp`
- `recommendations`: `user_id`
- `analytics_metrics`: `recorded_at`

### 4.3 Seed Data

- **Users**: `demo_user_1` … `demo_user_5`
- **Catalog**: Laptops, phones, sports gear, etc. with Picsum image URLs
- Used by recommendation engine when loading `content_catalog` at startup

---

## 5. Kafka Design

### 5.1 Topics

| Topic | Partitions | Purpose |
|-------|------------|---------|
| `user-events` | 4 | Raw customer behavior (normalized from CSV) |
| `recommendation-events` | 4 | Recalculated recommendation sets per user |
| `dashboard-events` | 4 | Materialized dashboard payloads (extensibility) |
| `analytics-events` | 4 | Global metrics snapshots + retention alerts |

### 5.2 Partition Key Strategy

Messages use **hash partitioning** on the message key:

| Producer | Topic | Key |
|----------|-------|-----|
| dataset-replay | `user-events` | `fmt.Sprintf("%d", userId)` |
| recommendation | `recommendation-events` | `userId` string |
| engagement | `dashboard-events` | `userId` string |
| retention-analytics | `analytics-events` | `"global"` |
| ai-insights | `analytics-events` | `"retention-alert"` (alerts only) |

**Effect**: All events for the same user land in the same partition → ordering per user is preserved.

### 5.3 Topic Creation

- **dataset-replay-service** calls `EnsureTopics` on startup (idempotent).
- Docker Kafka also has `KAFKA_AUTO_CREATE_TOPICS_ENABLE=true`.

---

## 6. Event & Message Schemas

### 6.1 `UserEvent` (topic: `user-events`)

```json
{
  "eventId": "uuid",
  "userId": 1,
  "itemId": 1002,
  "categoryId": 2,
  "eventType": "VIEWED | ADD_TO_CART | PURCHASED",
  "timestamp": "2024-01-01T10:01:00Z"
}
```

**CSV → normalized mapping** (dataset-replay):

| CSV `event` | `eventType` |
|-------------|-------------|
| `view` | `VIEWED` |
| `addtocart` | `ADD_TO_CART` |
| `transaction` | `PURCHASED` |

**CSV columns** (flexible header detection):

- `timestamp`, `visitorid`, `event`, `itemid`
- `categoryid` optional — if missing, derived as `itemId % 6 + 1`

### 6.2 `RecommendationEvent` (topic: `recommendation-events`)

```json
{
  "userId": 1,
  "recommendations": {
    "recommendedForYou": [ { "itemId", "title", "category", "score", "reason", "imageUrl" } ],
    "trendingNow": [ ... ],
    "becauseYouViewed": [ ... ],
    "cartRecommendations": [ ... ]
  },
  "timestamp": "..."
}
```

Each section holds up to **6** items.

### 6.3 `DashboardPayload` (Redis + `dashboard-events`)

```json
{
  "userId": 1,
  "recommendations": { /* PersonalizedRecs */ },
  "updatedAt": "..."
}
```

### 6.4 `AnalyticsMetrics` (topic: `analytics-events` + Redis `analytics:global`)

```json
{
  "retentionRate": 40.0,
  "ctr": 0.0,
  "conversionRate": 5.2,
  "engagementScore": 127.0,
  "roiPercentage": -23.5,
  "activeUsers": 5,
  "totalUsers": 5,
  "returningUsers": 2,
  "recClicks": 0,
  "recImpressions": 10,
  "purchases": 2,
  "views": 15,
  "revenueGain": 150.0,
  "systemCost": 5000.0,
  "updatedAt": "..."
}
```

### 6.5 `RetentionAlert` (in-memory + optional Kafka)

```json
{
  "alertType": "RETENTION_DROP | LOW_CONVERSION",
  "category": "Electronics",
  "dropPercentage": 12.5,
  "suggestion": "Increase electronics recommendations",
  "timestamp": "..."
}
```

### 6.6 `ActivityFeedItem` (in-memory in engagement-orchestrator)

```json
{
  "id": "uuid",
  "message": "User 1 viewed item 1002",
  "userId": 1,
  "timestamp": "...",
  "type": "event | recommendation | dashboard | analytics"
}
```

Max **100** items retained (newest first).

### 6.7 `ReplayState` (dataset-replay HTTP API)

```json
{
  "running": true,
  "paused": false,
  "speed": 2.0,
  "processed": 12,
  "total": 30
}
```

---

## 7. Service 1: Dataset Replay

**Port**: `8081`  
**Binary**: `services/dataset-replay-service/main.go`

### 7.1 Startup Sequence

1. Load config (`PEP_*` env vars).
2. Create structured logger.
3. `EnsureTopics` for all four Kafka topics.
4. Open Kafka writer for `user-events` only.
5. **Load entire CSV into memory** (`loadEvents`) — all rows parsed at boot.
6. Start Gin HTTP server.
7. Register SIGINT/SIGTERM → graceful shutdown, close writer.

### 7.2 CSV Loading Details

- Reads file at `PEP_EVENTS_CSV_PATH` (Docker: `/data/events.csv` mounted from `./data`).
- First row = header map (case-insensitive column names).
- Each data row → `UserEvent` with fresh `uuid` as `eventId`.
- Sets `state.Total` = number of parsed events.

### 7.3 Replay Loop (`runReplay`)

Runs in a **goroutine** when start/resume is called:

```
FOR each event index from state.Processed to len(events)-1:
  1. Check context cancelled → exit
  2. WHILE state.Paused → sleep 100ms
  3. Publish event to Kafka (key = userId, 3 retries)
  4. Increment state.Processed
  5. Sleep: 500ms / speed  (speed=2 → 250ms between events)
END
Set state.Running = false
```

**Speed control**: `PUT /api/replay/speed` with `{"speed": N}` — values ≤ 0 rejected.

**Pause**: Sets `paused=true`; loop spins without publishing.

**Resume**: If never started, starts new goroutine; else clears `paused`.

**Restart**: If `processed >= total` on start, resets `processed` to 0.

### 7.4 HTTP API

| Method | Path | Action |
|--------|------|--------|
| GET | `/health` | `{"status":"ok"}` |
| GET | `/api/replay/status` | Returns `ReplayState` |
| POST | `/api/replay/start` | Start replay goroutine |
| POST | `/api/replay/pause` | Pause |
| POST | `/api/replay/resume` | Resume |
| PUT | `/api/replay/speed` | Update speed multiplier |

All endpoints have **CORS** `Access-Control-Allow-Origin: *`.

---

## 8. Service 2: Recommendation Engine

**Port**: `8082`  
**Binary**: `services/recommendation-service/main.go`

### 8.1 Startup Sequence

1. Connect PostgreSQL (no AutoMigrate).
2. Load `content_catalog` from DB into in-memory `map[itemID]`.
3. If catalog empty → inject 15-item **fallback catalog** (hardcoded).
4. Start Kafka consumer goroutine on `user-events` (group: `recommendation-service`).
5. Start Gin HTTP server.

### 8.2 In-Memory State

**Per-user profile** (`UserProfile`):

| Field | Updated When |
|-------|--------------|
| `ViewedItems[itemID]++` | `VIEWED` |
| `CartItems[itemID]=true` | `ADD_TO_CART` |
| `PurchasedItems[itemID]=true` | `PURCHASED` |
| `CategoryCounts[categoryID]+=` | view +1, cart +2, purchase +3 |

**Global state** (shared across users):

| Field | Purpose |
|-------|---------|
| `itemViews[itemID]` | Popularity counter |
| `categoryHot[categoryID]` | Category activity |
| `coView[itemA][itemB]` | Co-occurrence when user views items |

### 8.3 Per-Event Processing (`processEvent`)

```
1. Update user profile + global stats (mutex locked)
2. INSERT into user_events table (PostgreSQL)
3. buildRecommendations(userId)
4. Publish RecommendationEvent to recommendation-events (key=userId, 3 retries)
```

### 8.4 Rule-Based Recommendation Rules

#### Section 1: `recommendedForYou` (limit 6)

- Score = `globalViews[item] × categoryBoost`
- `categoryBoost` = user's `CategoryCounts[item.CategoryID]`, min 0.5 if zero
- Exclude items already viewed or purchased
- Sort by score descending

#### Section 2: `trendingNow` (limit 6)

- Top items by global `itemViews` count
- Reason: `"Trending now across platform"`

#### Section 3: `becauseYouViewed` (limit 6)

- Find most-viewed item in user's history
- Recommend co-viewed items from `coView[lastViewed]`
- Fallback: other items in **same category** as last viewed
- Reason includes viewed item title

#### Section 4: `cartRecommendations` (limit 6)

- If cart empty → no recommendations
- Take first cart item → recommend other items in **same category**
- Reason: `"Frequently bought with cart items"`

### 8.5 HTTP API

| Method | Path | Response |
|--------|------|----------|
| GET | `/health` | ok |
| GET | `/api/recommendations/:userId` | `PersonalizedRecs` JSON (live in-memory, no Kafka wait) |

---

## 9. Service 3: Engagement Orchestrator

**Port**: `8083`  
**Binary**: `services/engagement-orchestrator/main.go`

Central **real-time hub** connecting Kafka, Redis, WebSocket, and activity feed.

### 9.1 Three Kafka Consumers (parallel goroutines)

| Goroutine | Topic | Consumer Group | On Message |
|-----------|-------|----------------|------------|
| `consumeRecommendations` | `recommendation-events` | `engagement-orchestrator-recs` | Cache + WS + feed + publish `dashboard-events` |
| `consumeAnalytics` | `analytics-events` | `engagement-orchestrator-analytics` | Cache global metrics + WS broadcast all users + feed |
| `consumeUserEvents` | `user-events` | `engagement-orchestrator-events` | Activity feed + WS `activity` to that user |

### 9.2 Recommendation Consumer Flow

```
Receive RecommendationEvent
  → Build DashboardPayload { userId, recommendations, updatedAt }
  → Redis SET dashboard:user:{id} TTL 5min
  → WebSocket broadcast to userId: { type: "dashboard_update", data: dashboard }
  → Kafka publish dashboard-events (key=userId)
  → Activity feed: "recommendations refreshed" + "dashboard updated"
```

### 9.3 Analytics Consumer Flow

```
Receive AnalyticsMetrics
  → Redis SET analytics:global TTL 5min
  → WebSocket broadcast ALL connected users: { type: "analytics_update", data: metrics }
  → Activity feed: "Analytics metrics refreshed"
```

### 9.4 User Event Consumer Flow

```
Receive UserEvent
  → Human message: "User N viewed/added to cart/purchased item X"
  → Activity feed (type: event)
  → WebSocket to userId: { type: "activity", data: { userId, itemId, eventType, message } }
```

### 9.5 WebSocket Hub

- **Endpoint**: `GET /ws/dashboard/:userId` (HTTP upgrade)
- **Hub structure**: `map[userID]map[connection]bool` — multiple tabs per user supported
- **On connect**:
  1. Register connection
  2. If Redis has cached dashboard → send immediate `dashboard_update`
  3. Read loop (keeps connection alive; 60s read deadline)
- **On disconnect**: Unregister connection
- **CheckOrigin**: always `true` (dev/demo)

### 9.6 HTTP API

| Method | Path | Behavior |
|--------|------|----------|
| GET | `/api/dashboard/:userId` | Read Redis; empty structure if miss |
| GET | `/api/activity` | Last 100 feed items |
| GET | `/ws/dashboard/:userId` | WebSocket upgrade |

---

## 10. Service 4: Retention Analytics

**Port**: `8084`  
**Binary**: `services/retention-analytics-service/main.go`

### 10.1 Constants

- `systemCost = 5000.0` (fixed platform cost for ROI)
- `avgOrderValue = 75.0` (revenue per `PURCHASED` event)

### 10.2 Event Consumer (`user-events`, group: `retention-analytics-service`)

Updates in-memory counters:

| Event Type | Counter Changes |
|------------|-----------------|
| `VIEWED` | `views++`, `categoryViews[cat]++`, `recImpressions++` |
| `ADD_TO_CART` | `addToCart++` |
| `PURCHASED` | `purchases++`, `revenueGain += 75`, `categoryPurch[cat]++` |

**User tracking**:

- First event for user → `usersSeen[user]=true`, `sessionCount[user]++`
- Second+ event for same user in session → `returningUsers[user]=true`

### 10.3 Periodic Publisher (every 3 seconds)

```
1. recalculate() all metrics from in-memory state
2. Redis SET analytics:global
3. Kafka publish analytics-events (key="global", 3 retries)
4. INSERT row into analytics_metrics table
```

### 10.4 Metric Formulas

| Metric | Formula |
|--------|---------|
| **Retention Rate** | `(returningUsers / totalUsers) × 100` — 0 if no users |
| **CTR** | `(recClicks / recImpressions) × 100` — 0 if no impressions |
| **Conversion Rate** | `(purchases / views) × 100` — 0 if no views |
| **Engagement Score** | `views + 3×addToCart + 5×purchases` |
| **ROI %** | `((revenueGain - systemCost) / systemCost) × 100` |
| **Active Users** | Count of users with `sessionCount > 0` |

### 10.5 HTTP API

| Method | Path | Response |
|--------|------|----------|
| GET | `/api/analytics` | Latest `AnalyticsMetrics` |
| GET | `/api/analytics/history` | Last 50 DB records |

---

## 11. Service 5: AI Insights

**Port**: `8085`  
**Binary**: `services/ai-insights-service/main.go`

### 11.1 Agent 1: Retention Analysis

**Triggers**:

1. **Kafka consumer** on `analytics-events` (group: `ai-insights-service`) — runs `analyzeRetention` on each metrics message.
2. **Ticker** every 15s — reads `analytics:global` from Redis and analyzes.

**Rules** (`analyzeRetention`):

| Condition | Alert |
|-----------|-------|
| `prevRetention > 0` AND `retentionRate < prevRetention - 5` | `RETENTION_DROP`, platform-wide |
| `conversionRate < 2` | `LOW_CONVERSION` |
| Per-category simulated score drops >15% vs previous | `RETENTION_DROP` for that category |

Alerts stored in-memory (max 50, newest first). Some alerts also republished to `analytics-events` with key `retention-alert`.

### 11.2 Agent 2: SQL AI Assistant

**Endpoint**: `POST /api/ai/sql`  
**Body**: `{ "question": "Show top retained users" }`

**Flow**:

```
1. generateSQL(question)
   - If PEP_USE_MOCK_AI=true OR no OPENAI_API_KEY → mockSQLFromQuestion()
   - Else → OpenAI chat/completions (gpt-4o-mini), return SQL only
2. executeSafeSQL(sql)
   - Must start with SELECT (case-insensitive)
   - Reject: INSERT, UPDATE, DELETE, DROP, ALTER, TRUNCATE, CREATE, GRANT
   - Run via GORM db.Raw(sql).Rows()
3. Return { question, generatedSql, rows[], rowCount }
```

**Mock SQL patterns**:

| Question contains | Generated query |
|-------------------|-----------------|
| "retained" / "retention" | Top users by event count HAVING COUNT > 3 |
| "conversion" + "categor" | Conversion rate by category_id |
| "active" + "week" | Events in last 7 days by user |
| "purchase" | Recent PURCHASED events |
| default | Event type counts GROUP BY |

### 11.3 HTTP API

| Method | Path | Response |
|--------|------|----------|
| GET | `/api/ai/alerts` | Array of `RetentionAlert` |
| POST | `/api/ai/sql` | `SQLQueryResponse` |

---

## 12. Frontend Dashboard

**URL**: http://localhost:3000  
**Stack**: React 18, React Router 6, Vite 6, Tailwind 3, Axios, Recharts

### 12.1 Environment / API Routing

Built with Vite env vars (defaults to localhost):

| Variable | Target |
|----------|--------|
| `VITE_REPLAY_URL` | `:8081` |
| `VITE_ENGAGEMENT_URL` | `:8083` |
| `VITE_ANALYTICS_URL` | `:8084` |
| `VITE_AI_URL` | `:8085` |
| `VITE_WS_URL` | `ws://localhost:8083` |

Nginx in Docker can proxy `/api/*` and `/ws/*` to internal service names.

### 12.2 Pages

#### `/` — Personalized Dashboard

1. User selector (ids 1–5).
2. `GET /api/dashboard/:userId` on mount and user change.
3. `useWebSocket(userId)` → connects to `/ws/dashboard/{userId}`.
4. On `dashboard_update` → replace all four recommendation rows + pulse animation.
5. Four `RecommendationRow` sections with horizontal scroll cards.

#### `/activity` — Live Activity Feed

1. Polls `GET /api/activity` every **2 seconds**.
2. WebSocket (user 1) prepends `activity` messages in real time.
3. Icons by type: event 👤, recommendation 🎯, dashboard 📊, analytics 📈.

#### `/analytics` — Analytics Dashboard

1. Polls analytics + history + AI alerts every **3 seconds**.
2. WebSocket listens for `analytics_update` → instant metric refresh.
3. **Metric cards**: retention, CTR, conversion, engagement, ROI, active users.
4. **Charts**: Bar (overview), Line (engagement + retention history).
5. **AI alerts panel**: last 8 retention insights.

#### `/sql` — AI SQL Assistant

1. Textarea + Run button + example chips.
2. `POST /api/ai/sql` → shows SQL block + result table.
3. Dynamic columns from first result row keys.

#### `/replay` — Replay Controls

1. Polls `GET /api/replay/status` every **1 second**.
2. Buttons: Start, Pause, Resume.
3. Speed slider (0.5×–10×) → `PUT /api/replay/speed` on release.
4. Progress bar: `processed / total`.

### 12.3 WebSocket Hook (`useWebSocket`)

- Connects on `userId` change.
- On close → **auto-reconnect after 3 seconds**.
- Parses JSON messages, calls `onMessage` callback.
- Exposes `connected` boolean for UI indicator (green "Live" / yellow "Reconnecting").

---

## 13. End-to-End Sequence (One User Event)

**Scenario**: Replay publishes User 1 `VIEWED` item 1002.

```
Step 1  [dataset-replay]
        POST /api/replay/start → goroutine reads event #N
        → Kafka user-events partition(hash(1))
        Message: { eventId, userId:1, itemId:1002, eventType:"VIEWED", ... }

Step 2  [recommendation-service]  (parallel with steps 3–4)
        Consumer group recommendation-service reads message
        → Update profile: ViewedItems[1002]++, coView tracking
        → INSERT user_events row
        → buildRecommendations(1) → 4 sections × up to 6 items
        → Kafka recommendation-events partition(hash(1))

Step 3  [retention-analytics-service]  (parallel)
        Consumer reads same user-events message
        → views++, recImpressions++, usersSeen[1]=true
        (On next 3s tick → publish analytics-events, update Redis)

Step 4  [engagement-orchestrator]  (3 consumers, parallel)

  4a  consumeUserEvents:
        → Feed: "User 1 viewed item 1002"
        → WS to user 1: { type:"activity", ... }

  4b  consumeRecommendations (slightly later):
        → Redis SET dashboard:user:1 (5 min TTL)
        → WS to user 1: { type:"dashboard_update", data: {...} }
        → Kafka dashboard-events
        → Feed: "recommendations refreshed", "dashboard updated"

  4c  consumeAnalytics (every 3s from retention):
        → WS to ALL users: { type:"analytics_update", ... }

Step 5  [ai-insights-service]
        On analytics-events message → analyzeRetention → maybe new alert
        Frontend /analytics shows alert on next poll or WS

Step 6  [frontend Dashboard]
        WebSocket receives dashboard_update
        → React state updates → cards re-render with animation
        User sees new "Because You Viewed" / "Recommended For You" items
```

**Typical latency**: One event → dashboard WS update in **~500ms–1s** (replay sleep) + Kafka consumer lag (<100ms in dev).

---

## 14. Consumer Groups & Parallelism

| Service | Topic | Consumer Group ID |
|---------|-------|-------------------|
| recommendation-service | user-events | `recommendation-service` |
| engagement-orchestrator | recommendation-events | `engagement-orchestrator-recs` |
| engagement-orchestrator | analytics-events | `engagement-orchestrator-analytics` |
| engagement-orchestrator | user-events | `engagement-orchestrator-events` |
| retention-analytics-service | user-events | `retention-analytics-service` |
| ai-insights-service | analytics-events | `ai-insights-service` |

**Important**: `recommendation-service` and `retention-analytics-service` are **separate consumer groups** on `user-events` — both receive **every** message (fan-out), not competing consumers.

**Within** engagement-orchestrator, three groups on three topics — independent offset tracking per pipeline.

---

## 15. Configuration Reference

### Docker Compose Service Ports

| Container | Host Port | Internal |
|-----------|-----------|----------|
| frontend | 3000 | 80 |
| dataset-replay | 8081 | 8081 |
| recommendation | 8082 | 8082 |
| engagement | 8083 | 8083 |
| retention-analytics | 8084 | 8084 |
| ai-insights | 8085 | 8085 |
| kafka | 9092 | 29092 |
| postgres | 5432 | 5432 |
| redis | 6379 | 6379 |
| zookeeper | 2181 | 2181 |

### Graceful Shutdown (all Go services)

1. SIGINT / SIGTERM received.
2. Cancel consumer context → goroutines exit.
3. `http.Server.Shutdown` with 10s timeout.
4. Close Kafka writers.

---

## 16. Docker Networking

- All services on default Compose network.
- Services reference each other by **service name** (`kafka`, `postgres`, `redis`, etc.).
- Frontend nginx uses `resolver 127.0.0.11` (Docker embedded DNS) for dynamic upstream resolution.
- `depends_on` with `condition: service_healthy` for Kafka/Postgres/Redis before app services start.

### Build

- Go services: `Dockerfile.go` with `ARG SERVICE=<folder-name>`.
- Multi-stage: `golang:1.24-alpine` builder → `alpine:3.20` runtime (~minimal image).

---

## 17. Demo Walkthrough (Step by Step)

### Prerequisites

```bash
docker compose up --build -d
# Wait until all containers show "Up"
docker compose ps
```

### Step 1 — Open UI

Navigate to **http://localhost:3000**

### Step 2 — Start Data Flow

1. Click **Replay** in nav.
2. Click **Start Replay** (speed 1× = one event every 500ms).
3. Watch progress bar advance (30 events in sample CSV).

### Step 3 — Personalized Dashboard

1. Click **Dashboard**.
2. Select **User 1**.
3. Confirm **● Live** WebSocket indicator is green.
4. As replay runs, four rows populate/update:
   - Recommended For You
   - Trending Now
   - Because You Viewed
   - Cart Recommendations
5. Ring pulse animation flashes on each `dashboard_update`.

### Step 4 — Live Activity

1. Open **Live Activity**.
2. See lines like `User 1 viewed item 1002`, `User 1: recommendations refreshed`.
3. Events appear via poll + WebSocket.

### Step 5 — Analytics

1. Open **Analytics**.
2. Watch metrics increase: views, engagement score, active users.
3. ROI starts negative (system cost $5000 vs growing revenue).
4. After enough events, **AI Retention Insights** may show alerts.

### Step 6 — SQL Assistant

1. Open **AI SQL**.
2. Click example **"Show top retained users"**.
3. Review generated SQL and result table from `user_events`.

### Step 7 — Pause / Speed

1. Return to **Replay**.
2. **Pause** — event stream stops; dashboard stops updating.
3. Set speed to **5×** — faster simulation.
4. **Resume** — continues from last `processed` index.

---

## 18. Failure Modes & Design Decisions

| Topic | Decision |
|-------|----------|
| **No ML** | Rule-based engine for predictability and demo clarity |
| **In-memory profiles** | Fast recalculation; lost on restart (acceptable for university demo) |
| **CSV preloaded** | Simple replay; full Kaggle file works via same parser |
| **No AutoMigrate** | SQL init scripts own schema; avoids GORM constraint conflicts |
| **analytics fan-out** | Same topic used for metrics + alerts; consumers filter by JSON shape |
| **ROI negative at start** | Expected until enough purchases (`revenueGain` > `systemCost`) |
| **CTR often 0** | `recClicks` not incremented in current event flow (impression on VIEW only) |
| **Activity feed in-memory** | Lost on engagement-orchestrator restart; poll + WS still work for live |
| **CORS *** | Dev-friendly; not production-hardened |
| **WebSocket CheckOrigin true** | Demo only |

### Extending the System

| Enhancement | Where to change |
|-------------|-----------------|
| Track rec clicks | Emit `REC_CLICK` events from frontend → handle in analytics |
| Persist recommendations | Write to `recommendations` table in recommendation-service |
| Real category from dataset | Join `item_properties` / `category_tree` in dataset-replay loader |
| Auth | Add middleware in Gin + JWT in frontend |
| Horizontal scale | Multiple engagement instances + Redis Pub/Sub instead of in-process Hub |

---

## Quick Reference: Redis Keys

| Key | Writer | Reader | TTL |
|-----|--------|--------|-----|
| `dashboard:user:{id}` | engagement | engagement, WS on connect | 5 min |
| `analytics:global` | retention, engagement | ai-insights ticker, engagement | 5 min |

---

## Quick Reference: All HTTP Endpoints

| Port | Endpoints |
|------|-----------|
| 8081 | `/health`, `/api/replay/*` |
| 8082 | `/health`, `/api/recommendations/:userId` |
| 8083 | `/health`, `/api/dashboard/:userId`, `/api/activity`, `/ws/dashboard/:userId` |
| 8084 | `/health`, `/api/analytics`, `/api/analytics/history` |
| 8085 | `/health`, `/api/ai/alerts`, `/api/ai/sql` |
| 3000 | React SPA (all routes client-side) |

---

*This document reflects the codebase as implemented. For setup commands, see the root [README.md](../README.md).*
