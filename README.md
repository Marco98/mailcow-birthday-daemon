# Mailcow Birthday Daemon 🎂

Very simple daemon that generates and synchronizes a Birthday Calendar for every Mailcow mailbox.

No user action is required. Everything is handled automatically.

## Installation

Just add it to the `docker-compose.override.yml`:

```yaml
services:
    birthdaydaemon:
        image: ghcr.io/marco98/mailcow-birthday-daemon:0.1.1
        restart: always
        environment:
        - MAILCOW_BASE=https://mailcow.host
        - MAILCOW_APIKEY=YOUR-APIKEY-HERE
        volumes:
        - birthdaydaemon:/data
volumes:
    birthdaydaemon:
```

The API-Key can be obtained in the admin panel at Configuration > Access > Edit administrator details > API > Read-Write Access

As the Mailcow API does not seem to be complete and looks more like a early access, i would strongly advice against enabling "Skip IP check for API".

## How it works

- Via the mailcow API a app password with access to carddav and caldav in generated for every user
    - As every app password in mailcow gets a global autoincrementing number, the app passwords are kept and saved to disk to avoid massively increasing this number
- All contacts of all address books are fetched and the birthday information is extracted per user
- The resulting events in the calendar are calculated in advance.
    - currently hardcoded to: 1 year in past; 10 years in future
    - Isolated per mailbox of course. A user will only see birthdays of his own contacts.
- The calculated events will get synchronized to a calendar in every mailbox called "Birthdays" (display name can be renamed by user in SOGo)
