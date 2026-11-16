# UTMN events bot

Copy `.env.example` to `.env` and replace the placeholder token and passwords.
The password in `DATABASE_URL` must match `POSTGRES_APP_PASSWORD` (URL-encode
special characters in the connection URL). The database hostname is `postgres`.

```sh
docker compose up -d --build
docker compose logs -f bot
```

Compose passes environment variables to the bot; the application does not read
the `.env` file itself. In a private chat, send `/start` to register: first name,
last name, optional surname (Skip button), then confirmation. Inline buttons
let users cancel, restart, confirm, and open the menu. `/start` also restarts an
incomplete registration. Registration is required for all messages and buttons,
including admin access. Completed registrations are saved in PostgreSQL;
incomplete drafts are stored in memory and disappear when the bot restarts.
Startup verifies database connectivity; database failures block protected actions
and leave confirmation drafts available for retry.

Set `BOT_ADMIN_TELEGRAM_IDS` to semicolon-separated Telegram user IDs. Registered
admins land in the admin panel on `/start`, after registration, and when expired
navigation opens Home. They can explicitly switch to the user menu and back.
Home returns to the currently selected panel. `/admin` also opens the admin panel.

The shared navigator keeps up to 10 screens per private chat. Back restores the
previous screen and its draft; Home clears the draft and returns to the current
panel's main menu. Refresh rerenders the current screen without adding history.
Text screens reuse one bot message. Photo/long previews are replaced together,
and processed input messages are deleted when Telegram permits it. Registration
prompts use the same message cleanup with their own Cancel/Restart controls.
Completed writes clear draft history so Back cannot repeat a save. Navigation
history is in memory and resets on restart; expired navigation opens the admin
panel for admins and the user menu for other users.

The user menu offers upcoming events, My registrations, and tag search. Users
choose at least one tag; search matches any selected tag and never duplicates an
event with multiple matches. Event cards show dates, location, photo, tags,
capacity, registration count, and the user's enrollment status. Enrollment closes
at the start time and locks the event row before checking capacity. Repeated
enrollment is idempotent. Cancellation requires confirmation and only removes the
sender's registration; users can also cancel a past registration from My
registrations. All lists and tag selectors use 8 items per page, with the current
page and selection restored on Back/Refresh.

Choose Create event to enter title, optional description/photo, location,
start/end, capacity, and semicolon-separated tags. Enter start/end dates as text
in `DD.MM.YYYY HH:MM` format. Prompts show an example based on the current time
for start, and one hour after start for end. Past start times are rejected;
end time must be later than start.
Dates use `Asia/Yekaterinburg` internally; timestamps are stored in UTC.
Preview the event and press Save to insert the event and tags in one transaction.
Skip is available for optional fields; skipping capacity means unlimited places.
Switching panels or pressing Cancel discards an unfinished event draft.
The admin Tags section supports creating tags and assigning them to existing
events. Choose an event, toggle tags across pages, and press Save. Clear removes
all assignments only after Save; leaving the section discards unsaved changes.
Tag names are trimmed and lowercased; creating the same name reuses the tag.
The event creation wizard also accepts semicolon-separated tag names.
The Events section lists existing events with pagination. Open an event to edit
its title, description, photo, location, dates, capacity, or tags. Edits use a
preview and confirmation and only update the selected field. Editing metadata on
past events is allowed; changing dates requires a future start. Capacity cannot
be reduced below the current registration count.
Deleting an event requires confirmation and also removes its registrations and
tag associations in one transaction; tags themselves are retained. Announcements
are still a placeholder.

Admin event cards include an Participants button opening a paginated participant
list. The Users section lists every registered user with name, username, Telegram
ID, and effective role. Displayed roles follow `BOT_ADMIN_TELEGRAM_IDS`, the same
source used by admin authorization; editing `users.role` alone does not grant
admin access.

PostgreSQL uses persistent volume `postgres_data`. The `bot` role owns the
`utmn-events` database and its tables, and has no superuser privileges. PostgreSQL
is accessible to the bot over the Compose network and currently exposes port
5432 on the host, as configured in `compose.yaml`.

The bootstrap and schema scripts run only when the PostgreSQL data directory is
empty. Editing these scripts or changing passwords in `.env` does not update an
existing database. Apply later schema and password changes explicitly.

New databases include the lookup indexes in `init.sql`. For an existing database,
apply the optional performance indexes without recreating its volume:

```sh
docker compose exec -T postgres psql -U postgres -d utmn-events < deploy/db/002_indexes.sql
```

```sh
docker compose down
```

Stopping the services this way preserves the database volume.
