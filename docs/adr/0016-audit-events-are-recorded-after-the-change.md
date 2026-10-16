---
status: accepted
---

# Audit Events are recorded after the change, logged before they are stored

Every sign-in, failed sign-in, sign-out, and change to a User, Group or Assignment is an Audit Event. An Event is written once its change has been made. It is not written inside the change's transaction. The event is logged as JSON first, so Loki receives it, and then appended to `audit_events`, which keeps it for a year. If the database refuses the append, that failure is logged and the change stands.

Writing inside the change's transaction would make the change and its record succeed or fail together. That would mean threading a transaction through every store of every context: users, access, sign-in and the Gate, most of which do single statements today. It would also tie the audit table's health to signing in: a full or locked `audit_events` would stop people signing in. Signing in matters more than a complete table, and the Loki copy covers the rare event the table loses.

A change that changes nothing records nothing. Adding a member who is already in the Group, or giving an App Role the Group already holds, writes no Event, so each action is recorded exactly once.

The Admin who made a change is its actor. A User acting on themself, such as signing in or renaming themself, has no actor. A failed sign-in keeps the email it was given, even when no User has that address, because that is what an Admin looks for when someone tries to guess passwords.

Pruning removes events older than 365 days. It runs at start-up and every hour on every replica, and is safe to repeat.
