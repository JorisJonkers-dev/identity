# Changelog

## 0.1.0 (2026-10-17)


### Features

* a Tokens page, as the design canvas drew it, with the Launcher's warning ([#64](https://github.com/JorisJonkers-dev/identity/issues/64)) ([3aa172f](https://github.com/JorisJonkers-dev/identity/commit/3aa172f326a928612dee00f69ef9b40923770376))
* Admins assign Groups to App Roles, and tokens carry only that Client's roles ([#35](https://github.com/JorisJonkers-dev/identity/issues/35)) ([6fbe553](https://github.com/JorisJonkers-dev/identity/commit/6fbe5534d3d0b174941a8193539731272f5f0716))
* an admin dashboard and Audit page, as the design canvas drew them ([#61](https://github.com/JorisJonkers-dev/identity/issues/61)) ([bf7deae](https://github.com/JorisJonkers-dev/identity/commit/bf7deaece3d348b01ef56b09c699c633c49ae1d8))
* make the first Admin, list who waits for a Group, and alert on them ([#40](https://github.com/JorisJonkers-dev/identity/issues/40)) ([43705f3](https://github.com/JorisJonkers-dev/identity/commit/43705f3e58c04446c0215266ec854c4ad42bd61e))
* Personal Tokens, minted per Client and checked by introspection ([#62](https://github.com/JorisJonkers-dev/identity/issues/62)) ([e8360f9](https://github.com/JorisJonkers-dev/identity/commit/e8360f98c2ad6d7d2953380b1144fb7814ba8008))
* read Clients from the cluster's Client objects ([#33](https://github.com/JorisJonkers-dev/identity/issues/33)) ([fd7ff62](https://github.com/JorisJonkers-dev/identity/commit/fd7ff622594af68c546206578d53e0cfcbf28628))
* record Audit Events for sign-ins and changes to Users, Groups and Assignments ([#60](https://github.com/JorisJonkers-dev/identity/issues/60)) ([8a3cac7](https://github.com/JorisJonkers-dev/identity/commit/8a3cac7c8b40b2df7b41d3b52e73d74b7e917dcc))
* sign a User in to a Client over OIDC ([#31](https://github.com/JorisJonkers-dev/identity/issues/31)) ([72deeb8](https://github.com/JorisJonkers-dev/identity/commit/72deeb873f04fb9dfb1e181558b6495f2a791dbe))
* sign in with a passkey as the Second Factor ([#42](https://github.com/JorisJonkers-dev/identity/issues/42)) ([dcec1a1](https://github.com/JorisJonkers-dev/identity/commit/dcec1a1a0728897849556489d4f678ef1f25c017)), closes [#16](https://github.com/JorisJonkers-dev/identity/issues/16)
* sign in with an authenticator app, which Admins must use ([#41](https://github.com/JorisJonkers-dev/identity/issues/41)) ([bb078ea](https://github.com/JorisJonkers-dev/identity/commit/bb078eaebee86c09c4748d8e1a308d85e260105a))
* sign out everywhere, with back-channel logout and an Admin's force sign-out ([#44](https://github.com/JorisJonkers-dev/identity/issues/44)) ([b79b83c](https://github.com/JorisJonkers-dev/identity/commit/b79b83c5cbaad302436b671dbdee0891ccdbdb2b))
* sign up, confirm by mail, sign in and edit the Profile ([#28](https://github.com/JorisJonkers-dev/identity/issues/28)) ([6488870](https://github.com/JorisJonkers-dev/identity/commit/64888709a3bbdbab5ec46e28ed50821103f0a9e0))
* sign Users in to gated apps through the Gate, one host at a time ([#43](https://github.com/JorisJonkers-dev/identity/issues/43)) ([b37ab8a](https://github.com/JorisJonkers-dev/identity/commit/b37ab8a6f8bb0c707756c9c3c6521ee5bca297da))
* signing keys survive restarts, sealed by Vault transit, and rotate every 90 days ([#65](https://github.com/JorisJonkers-dev/identity/issues/65)) ([1ab786c](https://github.com/JorisJonkers-dev/identity/commit/1ab786c2481469a6e0f949c65a2f977a3cce1089))
* the Gate takes Personal Tokens on gated and token hosts ([#63](https://github.com/JorisJonkers-dev/identity/issues/63)) ([2088553](https://github.com/JorisJonkers-dev/identity/commit/2088553956aeaf0e6465c030eb7cec1b43d3f9d5))
* the Launcher shows a User's apps, with icons served from identity ([#46](https://github.com/JorisJonkers-dev/identity/issues/46)) ([4bb9bad](https://github.com/JorisJonkers-dev/identity/commit/4bb9bad3bb61cf06e3eca71dfb5d3b6f6554da8e))


### Bug Fixes

* publish the local stack's ports below the ephemeral range ([#47](https://github.com/JorisJonkers-dev/identity/issues/47)) ([4e909b8](https://github.com/JorisJonkers-dev/identity/commit/4e909b8e4a45ee0b160ead81fbe1f31f72c7cc11))
* say plainly what happened and what to do next ([#39](https://github.com/JorisJonkers-dev/identity/issues/39)) ([1ae4b24](https://github.com/JorisJonkers-dev/identity/commit/1ae4b245930911064a1c0226acdec0de081fd7e0))
