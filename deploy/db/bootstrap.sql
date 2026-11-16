\getenv bot_password POSTGRES_APP_PASSWORD

CREATE ROLE bot LOGIN PASSWORD :'bot_password';
ALTER DATABASE "utmn-events" OWNER TO bot;

\connect utmn-events
SET ROLE bot;
\i /opt/db/init.sql