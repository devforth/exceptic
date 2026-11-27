# Deployment

The server is one container, `devforth/exceptic`, for amd64 and arm64. All its data is in `/data`.

<!-- image-tags -->
Image tags: `latest` (newest release), `X.Y` (newest patch of a minor version), `X.Y.Z`. Pin `X.Y` or `X.Y.Z` in production and upgrade on purpose.
<!-- /image-tags -->

## Docker Compose

1. Copy [compose.yml](../compose.yml) to the server and set its image to the version you start with (`devforth/exceptic:X.Y`).
2. Next to it create `.env` (every setting: [Configuration](configuration.md)):

   <!-- minimal-env -->
   ```sh
   PUBLIC_URL=https://errors.example.com
   ADMIN_PASSWORD=<at least 12 characters>
   SECRET_KEY=<openssl rand -hex 32>   # encrypts chat tokens; generated into /data when not set
   ```
   <!-- /minimal-env -->

3. Run `docker compose up -d`, open the UI and log in as `admin`. The password is `ADMIN_PASSWORD`. A form opens where you create the first admin with your email and your own password.
4. On the **Projects** page add a project and give its DSN to your app: [Connecting apps](sdk.md).

The container runs as an unprivileged user (uid 65534). A bind-mounted data folder must be writable by it: `sudo chown 65534:65534 ./data`. A named volume, as in the example, needs nothing.

`GET /health` answers 200 while the server works; Docker uses it too (`docker ps` shows `healthy`).

## HTTPS and a reverse proxy

Use HTTPS whenever the server is reachable from other machines.

1. In `compose.yml` publish the port only on localhost and give the network a fixed subnet:

   ```yaml
   services:
     exceptic:
       ports:
         - "127.0.0.1:8000:8000"
   networks:
     default:
       ipam:
         config:
           - subnet: 172.30.0.0/24
   ```

2. In `.env` set:

   ```sh
   PUBLIC_URL=https://errors.example.com
   TRUSTED_PROXIES=172.30.0.1   # the first address of that subnet
   ```

   For a proxy in a container of the same compose file, give it a fixed address on that network (`ipv4_address: 172.30.0.10`) and put that address in `TRUSTED_PROXIES` instead. Never trust a whole range such as `172.16.0.0/12`.

3. Point the proxy at `127.0.0.1:8000` and allow request bodies up to 20 MB.

Caddy (gets the certificate itself):

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

- Apps send errors to `/api/<project id>/envelope/`. It must stay reachable for them (and for browsers, if you monitor a frontend). It accepts only requests with the project's key.
- Limit the UI, REST API and MCP to your office or VPN with `ALLOWED_IPS`.
- Passwords, cookies and tokens are removed from incoming errors and logs before they are stored.
- Two roles: **admin** (everything: every project, users, settings, deleting) and **user** (issue statuses, logs, project DSNs and their own coding agents). A user sees only the projects given to them. Add users on the **Users** page: [Users](ui.md#users).
- After 10 wrong passwords in 15 minutes a login is blocked until the window ends. Browsers the user logged in from before can still log in.

## Resources

| | Minimum | Recommended |
|---|---|---|
| CPU | 1 vCPU | 2 vCPU for many busy projects |
| Memory | 256 MB | 512 MB |
| Disk | 1.3 × `MAX_DATABASE_SIZE` | the same, on an SSD |

The server uses about 200 MB of memory whatever the load; more memory does not make it faster.

Measured: 10 projects sending errors as fast as they could, each error with 5 logs, through the official Node.js SDK. The server accepted about this many requests a second (one request is one error or one batch of logs):

| CPU | 0.5 | 1 | 2 | 4 |
|---|---|---|---|---|
| Requests a second | 130 | 220 | 310 | 410 |

When the server cannot keep up, apps pause for a few seconds and then send again. What they drop meanwhile shows on the Projects page. A project that sends more than its part of the queue is paused alone ([Ingest](configuration.md#ingest)).

## Backup and restore

```sh
docker compose run --rm exceptic backup
```

It writes a copy to `/data/backups/<time>` while the server runs. Copy it off the server:

```sh
docker compose cp exceptic:/data/backups ./backups
```

Never copy `/data/leveldb` of a running server by hand: such a copy may not open.

To restore, stop the server and replace the data with the backup:

```sh
docker compose stop exceptic
docker run --rm -v <project>_data:/data -v "$PWD/backups/<date>:/backup:ro" alpine \
  sh -c 'rm -rf /data/leveldb && cp -a /backup/. /data/ && chown -R 65534:65534 /data'
docker compose start exceptic
```

`docker volume ls` shows the volume's name.

## Upgrades

```sh
docker compose pull && docker compose up -d
```

This brings the newest patch of your `X.Y`. For a new minor version change the tag first.

On stop the server sends the notifications still waiting. `compose.yml` gives it 30 s for that; with `docker run` add `--stop-timeout 30`.

## Sign in with Google

Users added by an admin can also sign in with Google. Nobody can sign up that way.

1. In Google Cloud, APIs & Services › Credentials, create an OAuth client ID of type Web application with the authorized redirect URI `<PUBLIC_URL>/auth/google/callback` (for example `https://errors.example.com/auth/google/callback`).
2. Set `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` and `PUBLIC_URL`, and restart.

The first Google sign-in links the Google account to the user with the same email. Add users only with addresses that are theirs now: someone who left a company may still have a Google account made with their work email.

## Forgotten password

```sh
docker compose stop exceptic
docker compose run --rm exceptic reset-password          # the only admin
docker compose run --rm exceptic reset-password <email>
docker compose start exceptic
```

It prints a temporary password; the user chooses a new one at the next login.

## Logs

`docker compose logs -f` shows the settings at start, warnings and errors. `LOG_LEVEL=debug` also shows every request and why an app's data was rejected.
