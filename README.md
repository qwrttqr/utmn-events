# UTMN events bot

Install Docker with Docker Compose, then create the environment file:

```sh
cp .env.example .env
```

Set these values in `.env`:

- `BOT_TOKEN`: your Telegram bot token.
- `BOT_PROXY_URL` (optional) routes Telegram API calls through a SOCKS5 proxy, for example `socks5://host:1080`.
- `POSTGRES_PASSWORD`: the PostgreSQL administrator password.
- `POSTGRES_APP_PASSWORD`: the password for the `bot` database role.
- `DATABASE_URL`: `postgres://bot:<app_password>@postgres:5432/utmn-events?sslmode=disable`. Use `POSTGRES_APP_PASSWORD` and URL-encode special characters in the password. The database name must be `utmn-events`.
- `BOT_ADMIN_TELEGRAM_IDS`: administrator Telegram IDs separated by semicolons, for example `123456789;987654321`.

Compose passes these variables to the bot. The application does not load `.env`
itself.

Build and start the services:

```sh
docker compose up -d --build
```