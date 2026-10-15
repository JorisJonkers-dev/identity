---
status: accepted
---

# Clients come only from deploy-kit, are held in memory, and an unknown host is refused

A Client exists only because an Application declares a `client` block in deploy-kit and the render applies the derived cluster-scoped `Client` object ([deploy-kit 0100](https://github.com/JorisJonkers-dev/deploy-kit/blob/main/docs/adr/model/0100-a-client-is-declared-on-the-application.md), chapter 17). identity never creates, edits or stores a Client definition: a `client-go` informer watches the objects and keeps a host index in memory, and the portal shows Clients read-only. A host with no Client is refused, which reverses auth-api's behaviour of letting any signed-in person through to a host it did not know. When a `Client` object disappears, the Client becomes a Removed Client: its Assignments and tokens are kept but give no access until it returns or an Admin purges it, so deleting an object by mistake costs nothing.

## Considered Options

- **Clients registered in the portal and stored in Postgres.** Easy to add, but unreviewed and invisible to the edge they configure.
- **Hand-written `Client` objects in fleet-infra.** No model change, but a second source that drifts from the exposure it describes.
- **Delete Assignments with the Client.** Simpler data, but a renamed or briefly removed Application would lose every grant.
