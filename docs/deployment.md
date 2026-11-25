# Deployment

The server is one container, `devforth/exceptic`, for amd64 and arm64. It keeps all its data in an embedded database, `/data/leveldb`, and needs nothing else; without `SECRET_KEY`, the key that encrypts chat tokens is `/data/secret.key`.

Image tags: `latest` (newest release), `X.Y` (newest patch of a minor version), `X.Y.Z`. Pin `X.Y` or `X.Y.Z` in production and upgrade on purpose.

## Docker Compose

1. Copy [compose.yml](../compose.yml) to the server and pin its image to the `X.Y` you start with (`devforth/exceptic:X.Y`).
2. Next to it, create `.env` (all settings: [Configuration](configuration.md)). A minimal one:

   ```sh
   PUBLIC_URL=https://errors.example.com
   ADMIN_PASSWORD=<at least 12 characters>
   SECRET_KEY=<openssl rand -hex 32>   # encrypts chat tokens; generated into /data when not set
   ```

   Behind a reverse proxy, also publish the port on localhost and set `TRUSTED_PROXIES`: [HTTPS and a reverse proxy](#https-and-a-reverse-proxy).

3. `docker compose up -d`, then open the UI and log in as `admin`.
4. Add projects on the **Projects** page (or with `PROJECTS`) and give their DSNs to your apps: [Connecting apps](sdk.md).

The container runs as an unprivileged user (uid 65534) without a shell. A bind-mounted data directory must be writable by it: `sudo chown 65534:65534 ./data`. A named volume, as in the example, needs nothing.

Docker checks the server with the `healthcheck` subcommand, which asks `/health` on `PORT` (`docker ps` shows `healthy`). A load balancer can use `GET /health`: it answers 200 while the database is reachable and 503 otherwise. `docker compose run --rm exceptic version` prints the version.

## HTTPS and a reverse proxy

Use HTTPS whenever the server is reachable from other machines: users log in to the UI and coding agents send their tokens. Put it behind a reverse proxy, publish the port only on localhost (`"127.0.0.1:8000:8000"` in `compose.yml`) and set:

- `PUBLIC_URL=https://errors.example.com`, so DSNs, links and the MCP address use it.
- `TRUSTED_PROXIES` to the one address the proxy's requests come from, as the container sees it. Docker picks the subnet of the compose network itself and may pick another one when the network is recreated (`docker compose down`), so give it a fixed one in `compose.yml` (any free private subnet; Docker makes its first address the gateway):

  ```yaml
  networks:
    default:
      ipam:
        config:
          - subnet: 172.30.0.0/24
  ```

  - A proxy on the host (the Caddy and nginx examples below): Docker hands its connections to the container from the gateway of that network, not from `127.0.0.1`: `TRUSTED_PROXIES=172.30.0.1`. Without a fixed subnet, `docker network inspect <project>_default --format '{{range .IPAM.Config}}{{.Gateway}}{{end}}'` shows the current gateway (`<project>` is the directory of `compose.yml`).
  - A proxy in a container of the same compose file: give it a fixed address on that network and trust that one, `TRUSTED_PROXIES=172.30.0.10` (the Exceptic port then needs no publishing at all; Compose accepts `ipv4_address` only on a network with a fixed subnet):

    ```yaml
    services:
      proxy:
        networks:
          default:
            ipv4_address: 172.30.0.10
    ```

  - `127.0.0.1` only for the binary run without Docker.

  Trust a Docker address only while the port is published on `127.0.0.1` (or not at all): with `"8000:8000"`, Docker relays some outside clients (IPv6 ones) through the gateway and other containers connect from Docker networks, so they could set `X-Forwarded-For` to any address and pass `ALLOWED_IPS` or reset the login limit. Never trust a whole range such as `172.16.0.0/12`.

  Without `TRUSTED_PROXIES` (or with a wrong address) the server sees every request coming from the proxy, so `ALLOWED_IPS` and the login limit cannot tell clients apart, and pages are treated as plain HTTP.

The proxy must pass `Host`, set `X-Forwarded-Proto`, and append the address it got the request from to `X-Forwarded-For` (Caddy does; nginx with `$proxy_add_x_forwarded_for` as below): a proxy that forwards the client's own header lets anyone pass `ALLOWED_IPS`. It must allow request bodies up to 20 MB (SDKs send large envelopes).

Caddy (sets the headers itself, gets the certificate):

```
errors.example.com {
	request_body {
		max_size 20MB
	}
	reverse_proxy 127.0.0.1:8000
}
```

nginx:

```nginx
server {
    listen 443 ssl;
    server_name errors.example.com;
    # ssl_certificate ...

    client_max_body_size 20m;

    location / {
        proxy_pass http://127.0.0.1:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Securing it

- The ingest endpoint (`/api/<project id>/envelope/`) must stay reachable by your apps, and by browsers if you monitor a frontend. It accepts only requests with a project's key, limited by `RATE_LIMIT_EVENTS_PER_MINUTE` and `RATE_LIMIT_LOGS_PER_MINUTE`, and reads no more of a request than its first line before the key is checked; envelopes being read at once may hold 80 MB together and those of one project half of it or 56 MB, the most one envelope may take (a brotli body, its window and its output), beyond that SDKs get `429` and retry, and a body must arrive within 10 seconds; `ALLOWED_ORIGINS` limits which sites a browser may send from.
- The links that stop notification emails (`/unsubscribe/<id>?token=…`) stay open too: mail providers send their one-click request from their own servers. Each works only with the token signed by `SECRET_KEY` (or `secret.key`) for one address, and only pauses that address's connections.
- Everything else (UI, REST API, MCP) can be restricted to your office or VPN with `ALLOWED_IPS`. In Docker without a proxy the server sees IPv4 clients of a published port as they are, but the host itself, and with `"8000:8000"` every IPv6 client, as the network's gateway: do not add the gateway, publish on `127.0.0.1` behind a proxy instead.
- Passwords, cookies, tokens and similar values are removed from incoming events and logs before they are stored; envelopes still waiting in the ingest queue, and backups made meanwhile, hold them as sent.
- Users get one of three roles: **superadmin** (everything), **admin** (projects, notifications, users of the role user, deleting issues and logs) and **user** (issues, logs, DSNs, their own coding agents). Add them on the **Users** page; each gets a temporary password to change at the first login. Admins and users see all projects or only the projects chosen for them (users from before this setting see all); outside them they see no issue, log, project or notification connection, and their agents neither. Who may change whom: [Users](ui.md#users).

## Data

Events and logs are kept until the database nears [its size limit](configuration.md#retention). What takes effort to set up again lives in the same database: projects and their keys (keep them in `PROJECTS` and the DSNs survive any loss), users, notification connections and connected agents. Keep the `data` volume on a disk you trust, and back it up.

## Resources

Measured on a laptop with 20 threads and an NVMe disk, with the peak of 50 projects each sending 1,000 errors and 2,200 log lines at once:

| | |
|---|---|
| Memory | About 200 MB at the peak (the default `GOMEMLIMIT`), less when idle. |
| CPU | The errors are stored in about 14 s (3,600 a second) on 5 cores on average; SDKs get their answers in milliseconds, since envelopes wait in the queue. |
| Disk space | At least 1.3 × `MAX_DATABASE_SIZE`: the limit counts the data held, and the files also hold deleted records until a compaction (about 10%), the ingest queue (up to 20% under a peak) and the write journal (16 MB); merging its files, the database writes new ones before it deletes the old. |
| Disk writes | The database writes data several times as it merges its files: the 366 MB of that peak wrote 1.8 GB. Use an SSD; a slow network volume becomes the limit. |
| Connections | At most 1,000 at once; more wait in the queue until one closes (idle keep-alive connections are closed first), none is refused. Request headers are limited to 64 KiB (431 above). |
| At the size limit | The same peak while the cleanup runs is stored in about a minute, with answers under 0.5 s and nothing refused; the cleanup took 2 GB down to 0.5 GB in 3 minutes. |

A crash of the process loses nothing the server has answered. A power cut may lose the envelopes of the last few milliseconds: the queue is written without a disk sync of its own, and the next stored batch syncs it.

## Backup and restore

```sh
docker compose run --rm exceptic backup
```

The running server writes a consistent copy to `/data/backups/<time>` without stopping (with the server stopped, the command reads the database itself); add an absolute path for another place, a new or empty directory (in Docker under `/data`, since the running server writes it in its own container). The copy holds `leveldb/` and `secret.key`, which decrypts the chat tokens of notifications (with `SECRET_KEY` set there is no file: keep that value). While a backup runs, the cleanup by size and its compactions wait (a pass already under way goes on). `/data/backups` is on the database's disk: copy backups off (`docker compose cp exceptic:/data/backups ./backups`) and delete them there with the helper container below (`rm -rf /data/backups`). Never copy `/data/leveldb` of a running server by hand: such a copy may not open.

To restore, stop the server and put the backup's `leveldb/` (and `secret.key`, if it has one) in place of those in the volume (the image has no shell, so use a helper container; `docker volume ls` shows the volume's name):

```sh
docker compose stop exceptic
docker run --rm -v <project>_data:/data -v "$PWD/backups/<date>:/backup:ro" alpine \
  sh -c 'rm -rf /data/leveldb && cp -a /backup/. /data/ && chown -R 65534:65534 /data'
docker compose start exceptic
```

## Upgrades

```sh
docker compose pull && docker compose up -d
```

This brings the newest patch of the pinned `X.Y`; for a new minor version change the tag first.

On shutdown (`docker stop` sends SIGTERM) the server finishes requests and writes queued notifications to the database; they are sent after the next start. This can take more than Docker's default 10 s before SIGKILL: `compose.yml` gives it 30 s (`stop_grace_period`), with `docker run` add `--stop-timeout 30`.

## Forgotten password

The running server holds the database, so stop it first:

```sh
docker compose stop exceptic
docker compose run --rm exceptic reset-password          # the superadmin
docker compose run --rm exceptic reset-password <email>
docker compose start exceptic
```

It prints a temporary password; the user chooses a new one at the next login. Their sessions end; their connected agents keep working.

## Login limit

Each login (and each check of the current password on the account page) counts before the password is checked, in 15-minute windows. Every browser a user logged in from is a known device for them: at every login (and own password change) it gets a signed cookie for 400 days, which holds the last user who logged in on that browser. The cookie stops counting once the user's password is changed or reset, their role changes or they are deleted. A known device gets 10 attempts per network, and all known devices together 30 per email, so others' guesses never lock it out and a copied cookie gains little. Any other attempt counts per address (an IPv6 /64) and per email from all addresses, 10 each: guessing one password from many addresses gets 10 tries per window in all, and while someone keeps guessing an email, its owner logs in only from their known device; a restart clears the counts. The right password clears its address's count when every attempt counted there was at that email (the user's own typos); otherwise it takes only itself off, so a user cannot clear guesses at other accounts. At most half the CPU cores (at least one) check passwords at once (known devices have one more place of their own), so a flood of logins does not stall ingest; an attempt that finds no place within 5 seconds is refused like a blocked one. Past 10,000 counted addresses or emails (a distributed guessing attack), new ones are refused until windows end and a warning is logged, while known devices still log in.

## Logs

`docker compose logs -f` shows the startup settings (enabled notification types, the size limit), warnings about missing settings and errors. Set `LOG_LEVEL=debug` to see every request and why an envelope was rejected (wrong key, unknown project, rate limit), and `LOG_FORMAT=json` for a log collector.
