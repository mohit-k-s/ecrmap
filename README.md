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
cd /Users/mohit/experiments/ecrmap
go build -o /usr/local/bin/ecrmap .
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

### 2. `ecrmap audit` (Repository Version Distribution)

See every active tag running from a specific repository and which services are running them:

```bash
ecrmap audit payment-service
```

**Sample Output:**
```
Repository Audit: payment-service
Active in 3 service(s) across 2 unique tag(s)

Tag / Version Distribution:
  • v2.1.0               → prod-us-east-1/payment-service (1 service(s))
  • v2.0.9               → stage-us-east-1/payment-stage, dev-cluster/payment-dev (2 service(s))

TAG        CLUSTER           SERVICE          TASK DEF             COUNT   FULL IMAGE
v2.1.0     prod-us-east-1    payment-service  payment-service:142  6/6     123456789012.dkr.ecr.us-east-1.amazonaws.com/payment-service:v2.1.0
v2.0.9     stage-us-east-1   payment-stage    payment-stage:88     2/2     123456789012.dkr.ecr.us-east-1.amazonaws.com/payment-service:v2.0.9
v2.0.9     dev-cluster       payment-dev      payment-dev:12       1/1     123456789012.dkr.ecr.us-east-1.amazonaws.com/payment-service:v2.0.9
```

---

### 3. `ecrmap list` (Cluster & Service Inventory)

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

### 4. `ecrmap cache`

```bash
# Show local cache directory
ecrmap cache dir

# Invalidate and clear local cache
ecrmap cache clean
```

---

## Global Flags

| Flag | Description | Default |
| :--- | :--- | :--- |
| `-p, --profile <name>` | AWS profile to use | Default credentials / SSO |
| `-r, --region <region>` | AWS region override | From AWS config / env |
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
