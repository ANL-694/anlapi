# anlapi Docker Image

`anlapi` is the project/repository name. `anlapi` is the runtime name used by
the binary, container, service, and Linux user because some environments do
not accept hyphens. Existing deployments may continue using the compatible
PostgreSQL database name `ikik_api`; renaming that database is not required.

## Quick Start

```bash
docker run -d \
  --name anlapi \
  -p 8080:8080 \
  -e DATABASE_HOST=host.docker.internal \
  -e DATABASE_PORT=5432 \
  -e DATABASE_USER=ikik_api \
  -e DATABASE_PASSWORD="change-me" \
  -e DATABASE_DBNAME=ikik_api \
  -e REDIS_HOST=host.docker.internal \
  -e REDIS_PORT=6379 \
  anlapi:latest
```

## Docker Compose

```yaml
version: '3.8'

services:
  anlapi:
    image: anlapi:latest
    ports:
      - "8080:8080"
    environment:
      - DATABASE_HOST=db
      - DATABASE_PORT=5432
      - DATABASE_USER=ikik_api
      - DATABASE_PASSWORD=change-me
      - DATABASE_DBNAME=ikik_api
      - DATABASE_SSLMODE=disable
      - REDIS_HOST=redis
      - REDIS_PORT=6379
    depends_on:
      - db
      - redis

  db:
    image: postgres:15-alpine
    environment:
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=change-me
      - POSTGRES_DB=ikik_api
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  redis_data:
```

## Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `DATABASE_HOST` | PostgreSQL host | Yes | - |
| `DATABASE_PORT` | PostgreSQL port | No | `5432` |
| `DATABASE_USER` | PostgreSQL user | No | `ikik_api` |
| `DATABASE_PASSWORD` | PostgreSQL password | Yes | - |
| `DATABASE_DBNAME` | PostgreSQL database name | No | `ikik_api` |
| `DATABASE_SSLMODE` | PostgreSQL SSL mode | No | `disable` |
| `REDIS_HOST` | Redis host | Yes | - |
| `REDIS_PORT` | Redis port | No | `6379` |
| `PORT` | Server port | No | `8080` |
| `GIN_MODE` | Gin framework mode (`debug`/`release`) | No | `release` |

## Supported Architectures

- `linux/amd64`
- `linux/arm64`

## Tags

- `latest` - Latest stable release
- `x.y.z` - Specific version
- `x.y` - Latest patch of minor version
- `x` - Latest minor of major version

## Links

- [GitHub Repository](https://github.com/ANL-694/anlapi)
- [Documentation](https://github.com/ANL-694/anlapi#readme)
