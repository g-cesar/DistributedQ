# Production Deployment Guide

## Overview

This guide explains how to deploy DistributedQ to production using Docker Compose with optimized settings for reliability and performance.

## Prerequisites

- Docker 20.10+
- Docker Compose 2.0+
- 2GB+ RAM recommended
- Linux/macOS/Windows with WSL2

---

## Quick Start

### 1. Clone Repository

```bash
git clone https://github.com/guido-cesarano/distributedq.git
cd distributedq
```

### 2. Configure Environment

```bash
cp .env.example .env
nano .env  # Edit configuration
```

**Important variables:**
```env
REDIS_PASSWORD=your_secure_password_here
GRAFANA_PASSWORD=your_grafana_password_here
```

### 3. Build and Start Services

```bash
docker-compose -f docker-compose.prod.yml up -d --build
```

This starts:
- **Redis** with persistence enabled
- **3 Worker instances** (horizontally scaled)
- **API Server** with health checks
- **Prometheus** with 30-day retention
- **Grafana** with provisioned dashboards

### 4. Verify Deployment

```bash
# Check all services are healthy
docker-compose -f docker-compose.prod.yml ps

# View worker logs
docker-compose -f docker-compose.prod.yml logs -f worker

# Test API
curl -X POST http://localhost:8081/enqueue \
  -H "Content-Type: application/json" \
  -d '{"type":"test","payload":{"message":"Hello Production"}}'
```

---

## Production Configuration

### Health Checks

All services include health checks:

**Redis:**
```yaml
healthcheck:
  test: ["CMD", "redis-cli", "--raw", "incr", "ping"]
  interval: 30s
  timeout: 3s
  retries: 3
```

**Worker:**
```yaml
healthcheck:
  test: ["CMD", "wget", "--spider", "-q", "http://localhost:8080/metrics"]
  interval: 30s
  timeout: 5s
  retries: 3
```

**Server:**
```yaml
healthcheck:
  test: ["CMD", "wget", "--spider", "-q", "http://localhost:8081/health"]
  interval: 30s
  timeout: 5s
  retries: 3
```

### Resource Limits

Workers are configured with resource limits to prevent overconsumption:

```yaml
deploy:
  resources:
    limits:
      cpus: '0.5'      # 50% of one CPU
      memory: 512M     # 512MB RAM max
    reservations:
      cpus: '0.25'     # 25% guaranteed
      memory: 256M     # 256MB guaranteed
```

Adjust based on your workload and available resources.

### Data Persistence

Three volumes persist data across container restarts:

- `redis_data` - Queue data and task history
- `prometheus_data` - Metrics (30-day retention)
- `grafana_data` - Dashboard configurations and user settings

**Backup volumes:**
```bash
# Backup Redis data
docker run --rm -v distributedq_redis_data:/data -v $(pwd):/backup alpine \
  tar czf /backup/redis-backup-$(date +%Y%m%d).tar.gz /data

# Backup Prometheus data
docker run --rm -v distributedq_prometheus_data:/data -v $(pwd):/backup alpine \
  tar czf /backup/prometheus-backup-$(date +%Y%m%d).tar.gz /data
```

---

## Scaling Workers

### Horizontal Scaling

Increase worker replicas via docker-compose:

```yaml
worker:
  deploy:
    replicas: 10  # Scale to 10 workers
```

Or scale dynamically:

```bash
docker-compose -f docker-compose.prod.yml up -d --scale worker=10
```

### Monitoring Worker Health

```bash
# View worker metrics
curl http://localhost:8080/metrics | grep distributedq

# Check processing rate
docker-compose -f docker-compose.prod.yml exec worker \
  wget -qO- http://localhost:8080/metrics | grep distributedq_processed_total
```

---

## Security Hardening

### 1. Redis Authentication

Redis requires password authentication in production:

```yaml
command: redis-server --appendonly yes --requirepass ${REDIS_PASSWORD}
```

Update worker/server to use password:

```go
client := redis.NewClient(&redis.Options{
    Addr:     os.Getenv("REDIS_ADDR"),
    Password: os.Getenv("REDIS_PASSWORD"),
})
```

### 2. Network Isolation

All services run on isolated network:

```yaml
networks:
  distributedq-network:
    driver: bridge
```

Only expose necessary ports externally.

### 3. API Authentication

Add middleware for API key validation:

```go
func apiKeyMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        apiKey := r.Header.Get("X-API-Key")
        if apiKey != os.Getenv("API_KEY") {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        next(w, r)
    }
}
```

### 4. TLS/HTTPS

Use reverse proxy (nginx, Traefik, Caddy) for TLS termination:

**Example with Caddy:**
```caddyfile
api.example.com {
    reverse_proxy localhost:8081
}

grafana.example.com {
    reverse_proxy localhost:3000
}
```

---

## Monitoring & Alerting

### Prometheus Alerts

Create `alerts.yml`:

```yaml
groups:
  - name: distributedq
    rules:
      - alert: HighQueueDepth
        expr: distributedq_queue_depth{queue="main_queue"} > 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Queue backlog detected"

      - alert: HighFailureRate
        expr: rate(distributedq_processed_total{status="failed"}[5m]) > 0.1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Task failure rate > 10%"
```

### Grafana Alerting

Configure alerts in Grafana dashboard:
1. Edit panel → Alert tab
2. Set threshold (e.g., queue depth > 1000)
3. Configure notification channel (Slack, PagerDuty, email)

---

## Troubleshooting

### Worker Not Processing Tasks

```bash
# Check worker logs
docker-compose -f docker-compose.prod.yml logs worker

# Check Redis connection
docker-compose -f docker-compose.prod.yml exec worker \
  ping redis
```

### High Memory Usage

```bash
# View resource usage
docker stats

# Reduce worker replicas
docker-compose -f docker-compose.prod.yml up -d --scale worker=2
```

### Redis Out of Memory

```bash
# Check Redis memory
docker-compose -f docker-compose.prod.yml exec redis redis-cli INFO memory

# Clear old data
docker-compose -f docker-compose.prod.yml exec redis redis-cli FLUSHDB
```

---

## Updating to New Version

```bash
# Pull latest code
git pull origin main

# Rebuild images
docker-compose -f docker-compose.prod.yml build

# Zero-downtime rolling update
docker-compose -f docker-compose.prod.yml up -d --no-deps --build worker
docker-compose -f docker-compose.prod.yml up -d --no-deps --build server
```

---

## Performance Tuning

### Redis Optimization

```yaml
command: |
  redis-server
  --appendonly yes
  --maxmemory 2gb
  --maxmemory-policy allkeys-lru
  --save 900 1
  --save 300 10
```

### Worker Pool Concurrency

Modify worker to use goroutine pool:

```go
const workerPoolSize = 10

for i := 0; i < workerPoolSize; i++ {
    go func() {
        for {
            // Dequeue and process
        }
    }()
}
```

---

## Cost Optimization

### Running on Cloud Providers

**AWS ECS:**
- Use Fargate for serverless containers
- Store volumes on EFS
- Use Application Load Balancer

**Google Cloud Run:**
- Deploy as container service
- Use Cloud Memorystore for Redis
- Autoscale based on queue depth

**DigitalOcean App Platform:**
- Deploy via App Spec
- Use Managed Redis
- $5/month for minimal setup

---

## Backup & Disaster Recovery

### Automated Backups

```bash
# Cron job for daily backups
0 2 * * * /path/to/backup.sh
```

**backup.sh:**
```bash
#!/bin/bash
DATE=$(date +%Y%m%d)
docker run --rm -v distributedq_redis_data:/data -v /backups:/backup alpine \
  tar czf /backup/redis-$DATE.tar.gz /data

# Upload to S3
aws s3 cp /backups/redis-$DATE.tar.gz s3://my-backups/distributedq/
```

### Restore from Backup

```bash
# Stop services
docker-compose -f docker-compose.prod.yml down

# Restore volume
docker run --rm -v distributedq_redis_data:/data -v $(pwd):/backup alpine \
  tar xzf /backup/redis-20250101.tar.gz -C /

# Restart services
docker-compose -f docker-compose.prod.yml up -d
```

---

## Support

For production support:
- GitHub Issues: https://github.com/guido-cesarano/distributedq/issues
- Email: g-cesar1@hotmail.com
- Documentation: https://github.com/guido-cesarano/distributedq/docs
