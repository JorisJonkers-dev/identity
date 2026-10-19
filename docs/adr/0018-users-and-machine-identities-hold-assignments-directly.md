---
status: accepted
---

# Users and Machine Identities hold Assignments directly, not only through Groups

Until now an Assignment was a Group holding an App Role on a Client, and access was never given to a User directly. Now an Assignment's holder is a Group, a User or a Machine Identity. A User holds every App Role assigned to them directly or to any of their Groups, and roles still combine, never override. A Machine Identity holds Assignments directly and never joins a Group. An Unassigned User is one who holds no Assignment by either route.

Three needs drove the change:
- **The import from auth-api (#24).** auth-api grants each person `SERVICE_*` roles one by one. Direct Assignments carry those grants over exactly, without inventing Groups nobody chose.
- **One-off access.** An Admin can let a single person into a single app without making a one-member Group.
- **Machine Identities.** Group membership means nothing to a workload.

## Considered Options

- **Group-only, with a Group per person or per service.** The model stays as it was. It fills the Groups list with one-member Groups, and the import would have to invent Group names.
- **A separate term for a User's own role.** It keeps "Assignment" meaning a Group, but every access question would have two answers with two names.

## Consequences

- An Admin reading someone's access looks in two places. The person page shows direct roles as toggles and Group roles locked with their source. The app page lists both Groups and people under each role.
- A Two-factor Group holds back only its own Assignments. A direct Assignment is not held back by a Group's requirement.
- The ID token's `roles` claim, the Launcher, the Gate and introspection all union both routes.
- Removing someone from every Group no longer makes them an Unassigned User if they still hold a direct Assignment.
