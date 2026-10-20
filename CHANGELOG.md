# Changelog

## 0.1.0 (2026-10-20)


### Features

* a forgotten password is reset through a mailed link ([#95](https://github.com/JorisJonkers-dev/identity/issues/95)) ([7090346](https://github.com/JorisJonkers-dev/identity/commit/709034638a6778df0109a3f7062c954b1181ddb6))
* a Tokens page, as the design canvas drew it, with the Launcher's warning ([#64](https://github.com/JorisJonkers-dev/identity/issues/64)) ([3aa172f](https://github.com/JorisJonkers-dev/identity/commit/3aa172f326a928612dee00f69ef9b40923770376))
* a User changes their email through a link mailed to the new address ([#93](https://github.com/JorisJonkers-dev/identity/issues/93)) ([7323e75](https://github.com/JorisJonkers-dev/identity/commit/7323e75312950babe494a2d198a32db7b5a183ff))
* a User deletes their account ([#94](https://github.com/JorisJonkers-dev/identity/issues/94)) ([f5a693f](https://github.com/JorisJonkers-dev/identity/commit/f5a693f6856e35775a645da386429d5e179e4185))
* Admins assign Groups to App Roles, and tokens carry only that Client's roles ([#35](https://github.com/JorisJonkers-dev/identity/issues/35)) ([6fbe553](https://github.com/JorisJonkers-dev/identity/commit/6fbe5534d3d0b174941a8193539731272f5f0716))
* an admin dashboard and Audit page, as the design canvas drew them ([#61](https://github.com/JorisJonkers-dev/identity/issues/61)) ([bf7deae](https://github.com/JorisJonkers-dev/identity/commit/bf7deaece3d348b01ef56b09c699c633c49ae1d8))
* an Admin gives a person a role directly, on the person page ([#90](https://github.com/JorisJonkers-dev/identity/issues/90)) ([6c608fa](https://github.com/JorisJonkers-dev/identity/commit/6c608fa6681d1a22e5f63d0c8408610cafd82213))
* Devices, where a User is signed in, each signed out on its own ([#92](https://github.com/JorisJonkers-dev/identity/issues/92)) ([bb11762](https://github.com/JorisJonkers-dev/identity/commit/bb11762a463a36469b2fcd02e7f20a320609711c))
* import Legacy Auth's people once ([#99](https://github.com/JorisJonkers-dev/identity/issues/99)) ([9b07a6c](https://github.com/JorisJonkers-dev/identity/commit/9b07a6caff9d2c92c70fb6eae00a6f2935e516e4))
* Machine Identities sign in by token exchange or client credentials ([#100](https://github.com/JorisJonkers-dev/identity/issues/100)) ([adecad6](https://github.com/JorisJonkers-dev/identity/commit/adecad6e3ff7ef783275af397c69e0af44984128))
* make the first Admin, list who waits for a Group, and alert on them ([#40](https://github.com/JorisJonkers-dev/identity/issues/40)) ([43705f3](https://github.com/JorisJonkers-dev/identity/commit/43705f3e58c04446c0215266ec854c4ad42bd61e))
* MCP clients connect as Connected Apps, with consent, rotating tokens and the Gate ([#101](https://github.com/JorisJonkers-dev/identity/issues/101)) ([909bc6f](https://github.com/JorisJonkers-dev/identity/commit/909bc6fe832efa27ddacfea92bffd66186e34f13))
* Personal Tokens, minted per Client and checked by introspection ([#62](https://github.com/JorisJonkers-dev/identity/issues/62)) ([e8360f9](https://github.com/JorisJonkers-dev/identity/commit/e8360f98c2ad6d7d2953380b1144fb7814ba8008))
* read Clients from the cluster's Client objects ([#33](https://github.com/JorisJonkers-dev/identity/issues/33)) ([fd7ff62](https://github.com/JorisJonkers-dev/identity/commit/fd7ff622594af68c546206578d53e0cfcbf28628))
* record Audit Events for sign-ins and changes to Users, Groups and Assignments ([#60](https://github.com/JorisJonkers-dev/identity/issues/60)) ([8a3cac7](https://github.com/JorisJonkers-dev/identity/commit/8a3cac7c8b40b2df7b41d3b52e73d74b7e917dcc))
* Secret Clients get their secret from identity in Vault KV, and Admins rotate it ([#66](https://github.com/JorisJonkers-dev/identity/issues/66)) ([0c230a2](https://github.com/JorisJonkers-dev/identity/commit/0c230a2ce8f271ffb245820f31a9db6e7650feb3))
* sign a User in to a Client over OIDC ([#31](https://github.com/JorisJonkers-dev/identity/issues/31)) ([72deeb8](https://github.com/JorisJonkers-dev/identity/commit/72deeb873f04fb9dfb1e181558b6495f2a791dbe))
* sign in with a passkey as the Second Factor ([#42](https://github.com/JorisJonkers-dev/identity/issues/42)) ([dcec1a1](https://github.com/JorisJonkers-dev/identity/commit/dcec1a1a0728897849556489d4f678ef1f25c017)), closes [#16](https://github.com/JorisJonkers-dev/identity/issues/16)
* sign in with an authenticator app, which Admins must use ([#41](https://github.com/JorisJonkers-dev/identity/issues/41)) ([bb078ea](https://github.com/JorisJonkers-dev/identity/commit/bb078eaebee86c09c4748d8e1a308d85e260105a))
* sign out everywhere, with back-channel logout and an Admin's force sign-out ([#44](https://github.com/JorisJonkers-dev/identity/issues/44)) ([b79b83c](https://github.com/JorisJonkers-dev/identity/commit/b79b83c5cbaad302436b671dbdee0891ccdbdb2b))
* sign up, confirm by mail, sign in and edit the Profile ([#28](https://github.com/JorisJonkers-dev/identity/issues/28)) ([6488870](https://github.com/JorisJonkers-dev/identity/commit/64888709a3bbdbab5ec46e28ed50821103f0a9e0))
* sign Users in to gated apps through the Gate, one host at a time ([#43](https://github.com/JorisJonkers-dev/identity/issues/43)) ([b37ab8a](https://github.com/JorisJonkers-dev/identity/commit/b37ab8a6f8bb0c707756c9c3c6521ee5bca297da))
* signing keys survive restarts, sealed by Vault transit, and rotate every 90 days ([#65](https://github.com/JorisJonkers-dev/identity/issues/65)) ([1ab786c](https://github.com/JorisJonkers-dev/identity/commit/1ab786c2481469a6e0f949c65a2f977a3cce1089))
* step-up, an Admin's per-app window for a fresh two-factor proof ([#91](https://github.com/JorisJonkers-dev/identity/issues/91)) ([e77f82c](https://github.com/JorisJonkers-dev/identity/commit/e77f82cd694ecc35747634ee7097d7a12f005b2a))
* task dev runs a seeded identity, with a dev-only code 000000 ([#73](https://github.com/JorisJonkers-dev/identity/issues/73)) ([b5943fc](https://github.com/JorisJonkers-dev/identity/commit/b5943fc061d7a00b852b81fb0854fc6a7500b178))
* the Gate takes Personal Tokens on gated and token hosts ([#63](https://github.com/JorisJonkers-dev/identity/issues/63)) ([2088553](https://github.com/JorisJonkers-dev/identity/commit/2088553956aeaf0e6465c030eb7cec1b43d3f9d5))
* the Launcher shows a User's apps, with icons served from identity ([#46](https://github.com/JorisJonkers-dev/identity/issues/46)) ([4bb9bad](https://github.com/JorisJonkers-dev/identity/commit/4bb9bad3bb61cf06e3eca71dfb5d3b6f6554da8e))
* the redesigned UI, on the G2 design system ([#74](https://github.com/JorisJonkers-dev/identity/issues/74)) ([55cfc19](https://github.com/JorisJonkers-dev/identity/commit/55cfc19e53ec49c4b02e4382beec202d2083ef0d))
* tools revoke their own tokens, and Connected Apps settle the review's open items ([#103](https://github.com/JorisJonkers-dev/identity/issues/103)) ([0731be0](https://github.com/JorisJonkers-dev/identity/commit/0731be081ca4b3f17bdb8b2e14a2222c2ed546ea))
* Two-factor Groups, step-up without tokens or on the Gate, and Admin person edits ([#96](https://github.com/JorisJonkers-dev/identity/issues/96)) ([b4b2a0a](https://github.com/JorisJonkers-dev/identity/commit/b4b2a0ac32e6256dfd164821b0d7d5d8d2ae6916))


### Bug Fixes

* a tool revoking its token always revokes the token it sent ([#121](https://github.com/JorisJonkers-dev/identity/issues/121)) ([2126888](https://github.com/JorisJonkers-dev/identity/commit/2126888198985c5b139f71f9fef803c9e54d2fd2)), closes [#120](https://github.com/JorisJonkers-dev/identity/issues/120)
* import no role for a retired KNOWLEDGE_API grant ([#122](https://github.com/JorisJonkers-dev/identity/issues/122)) ([67af5e9](https://github.com/JorisJonkers-dev/identity/commit/67af5e9cd8a58ebcf5f830191ca1d76a2b17e3d9))
* publish the local stack's ports below the ephemeral range ([#47](https://github.com/JorisJonkers-dev/identity/issues/47)) ([4e909b8](https://github.com/JorisJonkers-dev/identity/commit/4e909b8e4a45ee0b160ead81fbe1f31f72c7cc11))
* review findings on Connected Apps, refresh tokens and imported passwords ([#102](https://github.com/JorisJonkers-dev/identity/issues/102)) ([8aadcca](https://github.com/JorisJonkers-dev/identity/commit/8aadcca6abe92cda50c3cc5d3e8dc3faedc969e5))
* say plainly what happened and what to do next ([#39](https://github.com/JorisJonkers-dev/identity/issues/39)) ([1ae4b24](https://github.com/JorisJonkers-dev/identity/commit/1ae4b245930911064a1c0226acdec0de081fd7e0))
