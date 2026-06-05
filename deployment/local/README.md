# Local development (Docker Compose)

Local deployment is unchanged. Use the **root** `docker-compose.yml` in the repository:

```bash
# From repository root
docker compose up --build
```

Dashboard: http://localhost:3000

| Service | Port |
|---------|------|
| dataset-replay | 8081 |
| recommendation | 8082 |
| engagement | 8083 |
| retention-analytics | 8084 |
| ai-insights | 8085 |
| frontend (nginx) | 3000 |

See the root [README.md](../../README.md) and [Makefile](../../Makefile) for additional local commands.
