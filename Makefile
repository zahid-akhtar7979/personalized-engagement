.PHONY: up down build tidy logs

up:
	docker compose up --build -d

down:
	docker compose down

build:
	docker compose build

tidy:
	go mod tidy

logs:
	docker compose logs -f

# Run a single service locally (requires infra: make infra)
infra:
	docker compose up -d zookeeper kafka postgres redis

replay:
	PEP_PORT=8081 PEP_KAFKA_BROKERS=localhost:9092 PEP_EVENTS_CSV_PATH=./data/events.csv \
		go run ./services/dataset-replay-service

recommendation:
	PEP_PORT=8082 PEP_KAFKA_BROKERS=localhost:9092 \
		go run ./services/recommendation-service

engagement:
	PEP_PORT=8083 PEP_KAFKA_BROKERS=localhost:9092 PEP_REDIS_ADDR=localhost:6379 \
		go run ./services/engagement-orchestrator

analytics:
	PEP_PORT=8084 PEP_KAFKA_BROKERS=localhost:9092 PEP_REDIS_ADDR=localhost:6379 \
		go run ./services/retention-analytics-service

ai:
	PEP_PORT=8085 PEP_KAFKA_BROKERS=localhost:9092 PEP_USE_MOCK_AI=true \
		go run ./services/ai-insights-service

frontend:
	cd frontend-dashboard && npm install && npm run dev
