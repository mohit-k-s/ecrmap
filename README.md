# ecrmap 🗺️

> **Instant reverse lookup from ECR container images to active AWS ECS services.**  
> Zero infrastructure. Evaluated at runtime in seconds with goroutine concurrency and automatic local caching.

---

## The Problem

In AWS, container image mapping is strictly one-way:
- ECS Task Definitions declare which ECR image to pull.
- **ECR has zero native visibility into who is pulling or currently running its images.**

When an SRE or DevOps engineer asks:
* *"Is `payment-api:v2.1.0` actively running anywhere in production right now?"*
* *"Which services across our 8 clusters will break if I delete this image tag?"*
* *"What version distribution of `auth-service` is live right now?"*

They normally have to write throwaway scripts or manually click through dozens of ECS task definitions and clusters.

## The Solution: `ecrmap`

`ecrmap` performs a fast, parallelized runtime scan across your AWS ECS clusters:
1. Discovers all active ECS services and extracts their currently deployed `taskDefinition`.
2. Resolves unique task definitions concurrently with a worker pool.
3. Automatically parses container image URIs, ECR registries, tags, and digests.
4. Caches the snapshot locally in `~/.cache/ecrmap/` for 5 minutes so subsequent queries respond in **< 5 milliseconds**.

---

## Installation

### From Source (Go 1.24+)

```bash
git clone https://github.com/mohit-k-s/ecrmap.git
cd ecrmap
go build -o ecrmap .

# Optionally move to your PATH
sudo mv ecrmap /usr/local/bin/
```

Verify the installation:
```bash
ecrmap --help
```

---

## Commands & Usage

### 1. `ecrmap where` (Find where an image is running)

Find all active ECS services running a specific tag, repository, or digest:

```bash
# By repository and tag
ecrmap where payment-api:v2.1.0

# By repository name (finds all running tags of that repo)
ecrmap where payment-api

# By image digest
ecrmap where sha256:4f53cda18c2b...

# Force live AWS scan (bypass local cache)
ecrmap where payment-api:v2.1.0 --fresh
```

**Sample Output:**
```
Found 2 active container instance(s) matching payment-api:v2.1.0:

CLUSTER           SERVICE           STATUS   COUNT   TASK DEF             CONTAINER   IMAGE REF
prod-us-east-1    payment-service   ACTIVE   6/6     payment-service:142  app         v2.1.0
stage-us-east-1   payment-stage     ACTIVE   2/2     payment-stage:88     app         v2.1.0
```

---

### 2. `ecrmap repo` / `ecrmap audit` (Repository Version Distribution)

See every active version running from a specific ECR repository and which services are running them:

```bash
# Using repo alias
ecrmap repo payment-service

# Or audit
ecrmap audit payment-service
```

**Sample Output:**
```
Repository Audit: payment-service
Active in 3 service(s) across 2 unique tag(s)

Tag / Version Distribution:
  • v2.1.0               → prod-us-east-1/payment-service (1 service(s))
  • v2.0.9               → stage-us-east-1/payment-stage, dev-cluster/payment-dev (2 service(s))

SERVICE          CLUSTER           IMAGE VERSION   TASK DEF             STATUS   COUNT
payment-service  prod-us-east-1    v2.1.0          payment-service:142  ACTIVE   6/6
payment-stage    stage-us-east-1   v2.0.9          payment-stage:88     ACTIVE   2/2
payment-dev      dev-cluster       v2.0.9          payment-dev:12       ACTIVE   1/1
```

---

### 3. `ecrmap pulls` / `ecrmap churn` (Detect High Pull Churn & Crash-Loops)

Identify which services are driving excessive ECR image pull counts and rate limits:

```bash
# Analyze pull churn over the default 24h window
ecrmap pulls payment-service

# Or using churn alias over a 6-hour window
ecrmap churn payment-service --window 6h
```

**Sample Output:**
```
ECR Pull Churn Analysis: payment-service
Analyzed 3 active service(s) | Window: last 24h0m0s | Total Estimated Pulls: 852

CULPRIT RANKING (SERVICES DRIVING HIGHEST PULL CHURN):
RANK   SERVICE           CLUSTER         LAUNCH    RUNNING   STOPPED   EST. PULLS   RATE      HEALTH STATUS
#1     payment-worker    prod-us-east-1  FARGATE   4         842       ~846         35.2/hr   🔴 CRASH-LOOPING
#2     payment-api       prod-us-east-1  EC2       6         2         ~8           0.3/hr    🟢 HEALTHY
#3     payment-stage     stage-us-east-1 FARGATE   1         1         ~2           0.1/hr    🟢 HEALTHY

DIAGNOSTIC INSIGHTS & ROOT CAUSES:

  ⚠️  Service 'payment-worker' in 'prod-us-east-1' (Estimated pulls: ~846, Stopped: 842):
     • Dominant Stop Reason: Essential container in task exited
     • 840 task(s) exited with code 137 (OOM / Out-Of-Memory killed). Increase container/task memory limits.
     • Running on FARGATE: all 842 restarts required cold pulls without shared host layer cache.
```

---

### 4. `ecrmap list` (Cluster & Service Inventory)

List all active services and their deployed images:

```bash
# All clusters
ecrmap list

# Filter to a specific cluster
ecrmap list --cluster prod-us-east-1

# Output as JSON
ecrmap list --json
```

---

### 5. `ecrmap cache`

```bash
# Show local cache directory
ecrmap cache dir

# Invalidate and clear local cache
ecrmap cache clean
```

---

## Multi-Region Support 🌍

Many organizations keep a single central ECR repository (e.g. in `us-east-1`) while ECS workloads run across multiple AWS regions.

`ecrmap` queries multiple regions in parallel using goroutines, meaning a 5-region scan takes roughly the same ~3–5 seconds as a single-region scan:

```bash
# Query multiple specific regions (comma-separated)
ecrmap repo payment-service -r us-east-1,us-west-2,eu-west-1

# Or query ALL enabled regions in your AWS account in parallel
ecrmap where payment-api:v2.1.0 --all-regions
```

**Output across regions:**
```
Found 3 active container instance(s) matching payment-api:v2.1.0:

REGION       CLUSTER           SERVICE           STATUS   COUNT   TASK DEF             CONTAINER   IMAGE REF
us-east-1    prod-us-east-1    payment-service   ACTIVE   6/6     payment-service:142  app         v2.1.0
us-west-2    prod-us-west-2    payment-service   ACTIVE   4/4     payment-service:142  app         v2.1.0
eu-west-1    prod-eu-west-1    payment-eu        ACTIVE   2/2     payment-eu:65        app         v2.1.0
```

---

## Global Flags

| Flag | Description | Default |
| :--- | :--- | :--- |
| `-p, --profile <name>` | AWS profile to use | Default credentials / SSO |
| `-r, --region <region(s)>` | AWS region(s) (comma-separated, e.g. `us-east-1,us-west-2`) | From AWS config / env |
| `--all-regions` | Discover and query all enabled AWS regions in parallel | `false` |
| `-c, --cluster <name>` | Filter discovery to a single cluster | All clusters |
| `--fresh` | Bypass local cache and force live AWS discovery | `false` |
| `--cache-ttl <duration>` | Time to keep cached discoveries | `5m0s` |
| `--json` | Output results in JSON format | `false` |

---

## CI/CD Safe Deletion Check (Example)

Add this check to your CI/CD pipelines before running `aws ecr batch-delete-image`:

```bash
# If any active service is running this image, exit with error
MATCHES=$(ecrmap where "$REPO:$TAG" --fresh --json | jq '.deployments | length')

if [ "$MATCHES" -gt 0 ]; then
  echo "❌ Cannot delete $REPO:$TAG - actively running in $MATCHES ECS service(s)!"
  exit 1
fi

echo "✅ Safe to delete $REPO:$TAG"
```
