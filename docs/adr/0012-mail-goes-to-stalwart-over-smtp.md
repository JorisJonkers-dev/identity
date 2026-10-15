---
status: accepted
---

# identity sends its mail straight to Stalwart over SMTP submission

Sign-up confirmation, password reset and invite mail go from identity to Stalwart over SMTP submission (port 587, STARTTLS required), authenticated as a dedicated sending mailbox whose credentials come from Vault through the platform's secret delivery. A send happens inside the request that needs it: when Stalwart refuses or cannot be reached, the request fails with a generic error, the User can ask again, and the failure is logged with its detail and counted in a metric an alert watches. Mail templates are plain text and HTML rendered by identity itself.

Closes identity#7.

## Considered Options

- **An outbox table and an in-process sender.** Survives Stalwart being down for a while, at the cost of a worker, retries and an outbox to watch. Sign-up is rare enough that "try again" is acceptable.
- **A RabbitMQ event, as auth-api does.** Reuses the estate's pattern, and makes RabbitMQ a dependency of signing up.

## Consequences

- Stalwart must be up for a sign-up or reset to complete. The send-failure alert makes that visible.
- The sending mailbox is provisioned like every other Stalwart account, and its password is rotated through Vault.
