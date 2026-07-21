# HiveShare — Infrastructure Setup Guide

> **Goal:** Get you and one teammate running HiveShare together.
>
> **Three options:**
> | | Option A — ngrok | Option B — AWS EC2 | Option C — OpenShift |
> |---|---|---|---|
> | **Setup time** | 5 min | 30 min | 45 min |
> | **Cost** | Free | ~$8–10/mo (t3.small) or free tier (t2.micro) | Cluster cost varies |
> | **URL** | Temporary tunnel | Persistent (Elastic IP) | Persistent (cluster route) |
> | **Best for** | Quick two-person test | Ongoing team use, AWS shops | Enterprise / existing OCP cluster |

---

## Prerequisites (all options)

Build the binaries on your local machine first:

```bash
# Install Go if not present
sudo dnf install golang        # Fedora
# sudo apt install golang-go   # Ubuntu/Debian

cd ~/Work/sandbox/hiveshare
make deps build

ls bin/
# hiveshare-server   hiveshare-mcp   hshare
```

---

## Option A — ngrok

Run the server locally, expose it through an ngrok tunnel. Your teammate connects to the tunnel URL. Postgres and Redis run in Docker on your machine.

```
Your machine
  ├── Docker Compose  →  postgres :5432, redis :6379
  ├── hiveshare-server :8080
  └── ngrok tunnel  →  https://abc123.ngrok-free.app
                              └── Teammate's Claude Code / hshare CLI
```

### Step 1 — Start infrastructure

```bash
cd ~/Work/sandbox/hiveshare

make docker-up
sleep 5           # let postgres initialise
make migrate      # apply SQL schema

docker compose ps  # both should show "healthy"
```

### Step 2 — Start ngrok

Install: `snap install ngrok` or download from [ngrok.com/download](https://ngrok.com/download).

```bash
ngrok http 8080
```

Note the `Forwarding` URL, e.g. `https://abc123.ngrok-free.app`.

### Step 3 — Start the server

```bash
BASE_URL=https://abc123.ngrok-free.app \
EMBED_PROVIDER=openai \
OPENAI_API_KEY=sk-... \
./bin/hiveshare-server
```

> Omit `EMBED_PROVIDER` and `OPENAI_API_KEY` to use full-text search only — fine for a first test.

### Step 4 — Register and create a hiveshare

```bash
./bin/hshare auth register \
  --server https://abc123.ngrok-free.app \
  --email you@example.com \
  --name "Alice"
# saves hvs_... key to ~/.config/hiveshare/config.json

./bin/hshare headspace create "Sprint 42"
./bin/hshare headspace list        # note the UUID
./bin/hshare headspace use <uuid>
```

### Step 5 — Invite your teammate

```bash
./bin/hshare invite bob@example.com
# Prints:
#   Invite link: https://abc123.ngrok-free.app/api/v1/invitations/<token>/accept
```

Send Bob the link. He accepts it with curl or a browser:

```bash
curl -X POST https://abc123.ngrok-free.app/api/v1/invitations/<token>/accept \
  -H "Content-Type: application/json" \
  -d '{"name": "Bob"}'
# Returns Bob's hvs_ api_key (save it — only returned once; server stores SHA-256)
```

### Step 6 — Bob configures his client

Bob saves his config:

```bash
mkdir -p ~/.config/hiveshare
cat > ~/.config/hiveshare/config.json << 'EOF'
{
  "server_url": "https://abc123.ngrok-free.app",
  "api_key": "hvs_BOBS_KEY",
  "default_hiveshare": "HIVESHARE_UUID"
}
EOF
```

### Step 7 — Test live collaboration

**Alice's terminal** (keep running):
```bash
./bin/hshare stream
```

**Bob's terminal:**
```bash
echo "PROJ-42: JWT middleware skips validation on /internal/* routes." | \
  ./bin/hshare memory add \
    --source-type jira \
    --source-ref PROJ-42 \
    --tool claude
```

Alice's stream immediately shows:
```
[14:23:01] + memory added: jira/PROJ-42 by Bob
```

**Alice searches Bob's memory:**
```bash
./bin/hshare memory search "JWT validation"
```

---

## Option B — AWS EC2

A t3.small instance (2 vCPU, 2 GB RAM) running docker-compose for Postgres+Redis, nginx for TLS, and the hiveshare binary as a systemd service.

```
AWS
  └── EC2 t3.small  (Elastic IP  →  your-domain.com)
        ├── nginx (TLS via Let's Encrypt / ACM)
        ├── hiveshare-server :8080
        ├── Docker: postgres :5432 (localhost only)
        └── Docker: redis    :6379 (localhost only)
```

> **Free-tier note:** t2.micro (1 vCPU, 1 GB) qualifies for the 12-month AWS free tier. It will work for two people but may be tight if both are embedding large documents simultaneously. t3.small costs ~$15/mo on-demand, ~$8/mo reserved.

### Step 1 — Launch the EC2 instance

1. Open **EC2 → Launch Instance** in the AWS Console.
2. Choose **Ubuntu 24.04 LTS** (x86\_64).
3. Instance type: `t3.small` (or `t2.micro` for free tier).
4. Key pair: create or select one — you need SSH access.
5. **Security group** — add inbound rules:

   | Type | Port | Source |
   |---|---|---|
   | SSH | 22 | Your IP (use "My IP") |
   | HTTP | 80 | 0.0.0.0/0 |
   | HTTPS | 443 | 0.0.0.0/0 |

6. Storage: 20 GB gp3.
7. Launch.

### Step 2 — Assign an Elastic IP

1. **EC2 → Elastic IPs → Allocate**.
2. **Associate** the new IP with your instance.
3. Note the IP — you'll point DNS here.

### Step 3 — Point DNS to the EC2 instance

At your DNS provider (Route 53 or any registrar), add:

```
Type:  A
Name:  hiveshare     (or @ for apex)
Value: YOUR_ELASTIC_IP
TTL:   300
```

Wait 1–5 min for propagation. Verify: `dig hiveshare.yourdomain.com +short`

### Step 4 — SSH in and install dependencies

```bash
ssh -i your-key.pem ubuntu@YOUR_ELASTIC_IP

# Docker
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker ubuntu
newgrp docker

# nginx + certbot
sudo apt update
sudo apt install -y nginx certbot python3-certbot-nginx
```

### Step 5 — Copy the binary and config to EC2

On your **local machine**:

```bash
cd ~/Work/sandbox/hiveshare

# Cross-compile for Linux amd64
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" \
  -o bin/hiveshare-server-linux ./cmd/server

# Upload binary, migrations, and compose file
scp -i your-key.pem \
  bin/hiveshare-server-linux \
  docker-compose.yml \
  ubuntu@YOUR_ELASTIC_IP:/tmp/

scp -i your-key.pem -r migrations ubuntu@YOUR_ELASTIC_IP:/tmp/
```

On the **EC2 instance**:

```bash
sudo mkdir -p /srv/hiveshare
sudo mv /tmp/hiveshare-server-linux /srv/hiveshare/hiveshare-server
sudo mv /tmp/docker-compose.yml     /srv/hiveshare/
sudo mv /tmp/migrations             /srv/hiveshare/
sudo chmod +x /srv/hiveshare/hiveshare-server
```

### Step 6 — Start Postgres and Redis

```bash
cd /srv/hiveshare
sudo docker compose up -d
sleep 8

# Apply migrations
sudo docker compose exec -T postgres \
  psql -U hiveshare hiveshare < migrations/001_init.sql

sudo docker compose exec -T postgres \
  psql -U hiveshare hiveshare < migrations/002_indexes.sql

# Confirm
sudo docker compose ps
```

### Step 7 — Create a systemd service

```bash
sudo tee /etc/systemd/system/hiveshare.service << 'EOF'
[Unit]
Description=HiveShare API Server
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
WorkingDirectory=/srv/hiveshare
ExecStart=/srv/hiveshare/hiveshare-server
Restart=on-failure
RestartSec=5

Environment=LISTEN_ADDR=:8080
Environment=DATABASE_URL=postgres://hiveshare:hiveshare@localhost:5432/hiveshare?sslmode=disable
Environment=REDIS_URL=redis://localhost:6379
Environment=BASE_URL=https://hiveshare.yourdomain.com
Environment=EMBED_PROVIDER=openai
Environment=OPENAI_API_KEY=sk-YOUR_KEY

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now hiveshare
sudo systemctl status hiveshare
```

### Step 8 — Configure nginx and TLS

```bash
sudo tee /etc/nginx/sites-available/hiveshare << 'EOF'
server {
    listen 80;
    server_name hiveshare.yourdomain.com;

    location / {
        proxy_pass         http://localhost:8080;
        proxy_http_version 1.1;

        # Required for SSE (stream stays open)
        proxy_set_header   Connection '';
        proxy_set_header   Upgrade $http_upgrade;
        proxy_buffering    off;
        proxy_cache        off;
        proxy_read_timeout 3600s;

        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
    }
}
EOF

sudo ln -s /etc/nginx/sites-available/hiveshare /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx

# Issue TLS certificate (requires DNS to be propagated)
sudo certbot --nginx -d hiveshare.yourdomain.com \
  --non-interactive --agree-tos -m you@example.com

sudo systemctl reload nginx
```

### Step 9 — Register and invite

```bash
# On your local machine
./bin/hshare auth register \
  --server https://hiveshare.yourdomain.com \
  --email you@example.com \
  --name "Alice"

./bin/hshare headspace create "Sprint 42"
./bin/hshare headspace use <uuid>

./bin/hshare invite bob@example.com
# → prints invite link using https://hiveshare.yourdomain.com/...
```

Bob accepts the link, gets his key, and follows the same config step as in Option A (pointing to `https://hiveshare.yourdomain.com`).

### EC2 security checklist

```bash
# Verify Postgres and Redis are NOT exposed to the internet
sudo ss -tlnp | grep -E '5432|6379'
# Both should show 127.0.0.1 or docker network only, not 0.0.0.0

# Firewall (ufw)
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
sudo ufw status
```

---

## Option C — OpenShift

Deploy to an existing OpenShift cluster (OCP 4.x, ROSA, or ARO). All manifests are in `deploy/openshift/`.

```
OpenShift Cluster
  └── Namespace: hiveshare
        ├── Deployment: hiveshare   (your binary, init-container runs migrations)
        ├── Deployment: postgres    (pgvector image, PVC for data)
        ├── Deployment: redis       (ephemeral, pub/sub only)
        ├── Services:  hiveshare, postgres, redis
        └── Route:     hiveshare-hiveshare.apps.YOUR_CLUSTER_DOMAIN  (TLS edge)
```

### Step 1 — Log in to your cluster

```bash
# Get the login command from the OpenShift web console → top-right → "Copy login command"
oc login --token=<token> --server=https://api.YOUR_CLUSTER_DOMAIN:6443

# Confirm
oc whoami
oc cluster-info
```

### Step 2 — Build and push the image

You need a container registry the cluster can pull from. Options:

**Option i — OpenShift internal registry (simplest):**
```bash
# Expose the internal registry (if not already exposed)
oc patch configs.imageregistry.operator.openshift.io/cluster \
  --patch '{"spec":{"defaultRoute":true}}' --type=merge

REGISTRY=$(oc get route default-route -n openshift-image-registry \
  --template='{{ .spec.host }}')

# Log in Docker to the internal registry
docker login -u $(oc whoami) -p $(oc whoami -t) $REGISTRY

# Build and push
cd ~/Work/sandbox/hiveshare
docker build -t $REGISTRY/hiveshare/hiveshare:latest .
docker push $REGISTRY/hiveshare/hiveshare:latest
```

**Option ii — Docker Hub or Quay.io (pull from internet):**
```bash
docker build -t YOUR_DOCKERHUB_USER/hiveshare:latest .
docker push YOUR_DOCKERHUB_USER/hiveshare:latest
```

Update the `image:` field in `deploy/openshift/hiveshare.yaml` with your registry path.

### Step 3 — Create the namespace

```bash
oc apply -f deploy/openshift/namespace.yaml
oc project hiveshare
```

### Step 4 — Set secrets

Edit `deploy/openshift/secret.yaml` — fill in the real values:

```yaml
stringData:
  DATABASE_URL: "postgres://hiveshare:STRONG_PASSWORD@postgres:5432/hiveshare?sslmode=disable"
  OPENAI_API_KEY: "sk-..."
  BASE_URL: "https://hiveshare-hiveshare.apps.YOUR_CLUSTER_DOMAIN"
```

Also update the postgres password in `deploy/openshift/postgres.yaml` to match (`POSTGRES_PASSWORD: STRONG_PASSWORD`).

Apply:
```bash
oc apply -f deploy/openshift/secret.yaml
```

### Step 5 — Deploy Postgres and Redis

```bash
oc apply -f deploy/openshift/postgres.yaml
oc apply -f deploy/openshift/redis.yaml

# Wait for both to be ready
oc rollout status deployment/postgres
oc rollout status deployment/redis
```

> **OpenShift note:** If Postgres fails with a permission error on the data directory, add the postgres service account to the `anyuid` SCC:
> ```bash
> oc adm policy add-scc-to-serviceaccount anyuid -z default -n hiveshare
> ```

### Step 6 — Update the image reference and deploy HiveShare

Edit `deploy/openshift/hiveshare.yaml` — replace `YOUR_REGISTRY/hiveshare:latest` with your actual image path (same in both `initContainers` and `containers`).

Also replace `YOUR_CLUSTER_DOMAIN` in the `Route` resource with your cluster's apps domain:

```bash
# Find your cluster domain
oc get ingresses.config/cluster -o jsonpath='{.spec.domain}'
```

Apply:
```bash
oc apply -f deploy/openshift/hiveshare.yaml

oc rollout status deployment/hiveshare
oc get route hiveshare
# NAME        HOST/PORT                                           ...
# hiveshare   hiveshare-hiveshare.apps.YOUR_CLUSTER_DOMAIN       ...
```

### Step 7 — Register and invite

```bash
BASE_URL=https://hiveshare-hiveshare.apps.YOUR_CLUSTER_DOMAIN

./bin/hshare auth register \
  --server $BASE_URL \
  --email you@example.com \
  --name "Alice"

./bin/hshare headspace create "Sprint 42"
./bin/hshare headspace use <uuid>

./bin/hshare invite bob@example.com
```

Bob accepts the invite link and configures his client with:
```bash
mkdir -p ~/.config/hiveshare
cat > ~/.config/hiveshare/config.json << EOF
{
  "server_url": "$BASE_URL",
  "api_key": "hvs_BOBS_KEY",
  "default_hiveshare": "HIVESHARE_UUID"
}
EOF
```

### Useful OpenShift operations

```bash
# View server logs
oc logs -f deployment/hiveshare -n hiveshare

# Scale to 2 replicas (stateless, safe to do)
oc scale deployment/hiveshare --replicas=2 -n hiveshare

# Re-apply a config change
oc apply -f deploy/openshift/hiveshare.yaml

# Open a shell in the running pod
oc rsh deployment/hiveshare -n hiveshare

# Run a one-off migration manually
oc exec deployment/postgres -n hiveshare -- \
  psql -U hiveshare hiveshare -c '\dt'
```

---

## MCP Setup (any option — both teammates)

### Build and install the MCP binary

```bash
cd ~/Work/sandbox/hiveshare
make mcp

# Copy to somewhere on your PATH
cp bin/hiveshare-mcp ~/.local/bin/
```

### Configure Claude Code

Add to `~/.claude/claude_desktop_config.json` (or project `.mcp.json`):

```json
{
  "mcpServers": {
    "hiveshare": {
      "command": "~/.local/bin/hiveshare-mcp",
      "env": {
        "HIVESHARE_API_KEY": "hvs_your_key",
        "HIVESHARE_SERVER_URL": "https://YOUR_SERVER_URL",
        "HIVESHARE_DEFAULT_HIVESHARE": "YOUR_HIVESHARE_UUID"
      }
    }
  }
}
```

Restart Claude Code or run `/mcp` to reload.

Verify in Claude: `"List my hiveshares"` — Claude should call `list_hiveshares` and return your space.

---

## Verification checklist

```
[ ] curl https://YOUR_URL/api/v1/auth/whoami  →  401 (server is up, auth works)
[ ] Alice registers  →  gets hvs_ key
[ ] Alice creates hiveshare, sets default
[ ] Alice invites Bob  →  link contains the server URL (not localhost)
[ ] Bob accepts  →  gets his own hvs_ key
[ ] Bob adds a memory entry via CLI
[ ] Alice's `hshare stream` terminal shows live update within 2 seconds
[ ] Alice searches  →  finds Bob's entry
[ ] Both have MCP loaded in Claude Code
[ ] Claude calls search_memory before re-crunching a known ticket
[ ] `hshare metrics` shows entries from both users
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Invite link uses `localhost` | `BASE_URL` not set or wrong | Restart server with `BASE_URL=https://YOUR_URL` |
| SSE stream drops after ~60s | nginx default timeout | Confirm `proxy_read_timeout 3600s` in nginx config |
| `db ping: connection refused` | Postgres not ready | `docker compose ps` / `oc rollout status deployment/postgres` |
| `invalid api key` | Wrong key in config | Check `~/.config/hiveshare/config.json` |
| Search returns nothing | No entries yet, or wrong hiveshare UUID | Add at least one entry; verify UUID matches |
| OpenShift pod CrashLoopBackOff | Permission denied on DB data dir | `oc adm policy add-scc-to-serviceaccount anyuid -z default -n hiveshare` |
| OpenShift Route returns 503 | Pod not ready yet | `oc rollout status deployment/hiveshare` |
| MCP tool not found in Claude | Binary path wrong in config | Use absolute path; verify with `which hiveshare-mcp` |
| Embedding fails silently | Missing or invalid `OPENAI_API_KEY` | Set env var; or remove `EMBED_PROVIDER` for full-text-only mode |
