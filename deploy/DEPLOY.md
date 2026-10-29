# DEPLOY — budol-api on your own Linux server (generic guide)

The API is a single static Go binary. It runs as an unprivileged user under
`~/budol/` and listens on `127.0.0.1:8790`. TLS and the public hostname come
from a reverse proxy or tunnel in front (nginx or cloudflared — pick one).
Nothing here is executed by the repo; run the steps by hand. Never commit
secrets or machine-specific notes (hostnames, IPs, users) to this repository.

Layout on the server:

```
~/budol/
  bin/budol-api        # the cross-compiled binary
  budol-api.env        # live config: copy of deploy/budol-api.env.example + secrets
  ensure.sh            # keep-alive (cron @reboot + every 5 min)
  state/               # daily counters + neuron ledger (counters only — never text)
  logs/budol-api.log   # stdout logs (metadata only)
  DISABLED             # kill switch: touch to stop, rm to resume
~/.config/budol/ai.env # mode 600: AI_API_KEY=<Cloudflare API token> (the secret itself)
```

## 0. Before going live

1. **Provider**: the default is Cloudflare Workers AI `@cf/openai/gpt-oss-120b` (see docs/PROVIDER.md and eval/RESULTS.md). You need a free Cloudflare account, its Account ID, and an API token with only the "Workers AI: Read" permission.
2. **Ingress**: put nginx (+ certbot) or a Cloudflare named tunnel in front of `127.0.0.1:8790`.
3. **DNS**: point the API hostname (`prod.apiUrl` in `config/project.json`) at your server or tunnel.

## 1. Build (on your dev machine)

```sh
bash deploy/build.sh        # -> dist-bin/budol-api (linux/amd64, static)
```

## 2. Copy to the server (as the unprivileged service user)

```sh
scp dist-bin/budol-api  <user>@<server>:~/budol/bin/budol-api
scp deploy/ensure.sh    <user>@<server>:~/budol/ensure.sh
scp deploy/budol-api.env.example <user>@<server>:~/budol/budol-api.env
chmod +x ~/budol/bin/budol-api ~/budol/ensure.sh   # on the server
```

## 3. Secrets (on the server — never in git)

```sh
umask 077
mkdir -p ~/.config/budol
# Type/paste the key at the hidden prompt: it never lands in shell history or argv.
read -rsp 'AI_API_KEY (Cloudflare API token, Workers AI Read only): ' k; echo
printf 'AI_API_KEY=%s\n' "$k" > ~/.config/budol/ai.env; unset k
chmod 600 ~/.config/budol/ai.env
```

Edit `~/budol/budol-api.env`: set `BUDOL_ENV=prod`, `ALLOWED_ORIGINS` (your
site origin), `TURNSTILE_SECRET` (if used), `BUDOL_STATS_TOKEN`,
`BUDOL_IP_SALT` (random), `CF_ACCOUNT_ID=<your Cloudflare Account ID>` (not
secret; substituted into `AI_BASE_URL`), `AI_DAILY_NEURON_BUDGET` (default 8000
of the 10,000 free neurons per UTC day) and confirm `AI_BASE_URL` / `AI_MODEL`.
To switch provider later, change only the `AI_*` lines (see docs/PROVIDER.md).
Leave `TRUST_CF_HEADERS` empty for nginx; set `TRUST_CF_HEADERS=1` only for
the cloudflared tunnel path.

## 4. Keep-alive cron (as the service user; do not touch other entries)

```sh
crontab -l > /tmp/cron.old && cat /tmp/cron.old deploy/crontab.add | crontab -
# (copy deploy/crontab.add to the server first, or paste the two lines by hand)
```

Start once by hand: `~/budol/ensure.sh`, then check `~/budol/logs/budol-api.log`.

## 5. Ingress — pick ONE

- **nginx** (as root): install `deploy/nginx-api.conf.example` (replace the
  example hostname), enable certbot for your API hostname,
  `nginx -t && systemctl reload nginx`.
- **cloudflared**: follow the comments in `deploy/cloudflared-config.yml.example`
  and set `TRUST_CF_HEADERS=1`.

## 6. Verify (from the server)

```sh
curl -s http://127.0.0.1:8790/v1/health
# {"ok":true,"version":"1","engine_version":"...","ai_configured":true}
curl -s http://127.0.0.1:8790/v1/check \
  -H 'Content-Type: application/json' \
  -d '{"text":"GCash: i-verify ang account dito http://example.com/login"}'
# 200 with rules + ai verdict; ai_status shows quota/timeout/etc on AI trouble
curl -s http://127.0.0.1:8790/v1/stats -H "Authorization: Bearer $BUDOL_STATS_TOKEN"
```

Real-provider smoke test (dev machine, spends one LLM call):

```sh
CF_ACCOUNT_ID=... AI_API_KEY='...' BUDOL_ENV=dev go run ./cmd/aismoke
# prints only: ai_status, model, verdict, risk, latency
```

## 7. Frontend

`go run ./cmd/build -env prod` renders `dist/<slug>/` with `{{API_URL}}` from
`config/project.json` (`prod.apiUrl`), so the web client posts to
`{{API_URL}}/v1/check`. Host the static `dist/<slug>/` folder anywhere
(GitHub Pages, any static host). Dev uses `http://localhost:18790` with the
page at `http://localhost:8788/budol-bantay/` (`make dev` / `make e2e`).

## 8. Kill switch / rollback

```sh
touch ~/budol/DISABLED   # ensure.sh stops the service within 5 min
rm ~/budol/DISABLED      # resume on next cron tick (or run ensure.sh)
```
