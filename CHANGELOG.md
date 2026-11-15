# Changelog

## [0.4.0](https://github.com/amoshydra/sinkrill/compare/v0.3.1...v0.4.0) (2026-11-15)


### ⚠ BREAKING CHANGES

* make the Sinkrill output device the default

### Features

* let bare sinkrill attach to a running stream ([8ad879b](https://github.com/amoshydra/sinkrill/commit/8ad879bfe9169c11eb7a546eaae384a6dc95d488))
* let the panel start the stream it is watching ([73b2006](https://github.com/amoshydra/sinkrill/commit/73b2006b2519a650653d81f2bdf1e4543893cc23))
* make the Sinkrill output device the default ([c4352ee](https://github.com/amoshydra/sinkrill/commit/c4352ee691d901fb34d13a03b37681b1bfabcc1e))
* open the panel by default, and explain silence ([a7a6bbb](https://github.com/amoshydra/sinkrill/commit/a7a6bbb8321a95698c5f20e6db2695cb8c58d08e))


### Bug Fixes

* let tui --attach name the room to control ([bf81f03](https://github.com/amoshydra/sinkrill/commit/bf81f0359d901e4f21b96b9a4baf194f6d5d2c1e))
* reject a source the virtual sink would ignore ([11b3d8f](https://github.com/amoshydra/sinkrill/commit/11b3d8ff6683ec4111f1a4f41aa72524b11e95ad))
* say what an attached panel actually attached to ([9865caa](https://github.com/amoshydra/sinkrill/commit/9865caa391f60cde24b9dfa465a1864efae20e53))

## [0.3.1](https://github.com/amoshydra/sinkrill/compare/v0.3.0...v0.3.1) (2026-11-14)


### Features

* add `sinkrill cleanup` for what a hard kill leaves behind ([2a4b299](https://github.com/amoshydra/sinkrill/commit/2a4b2991dbf9d4c2f6224f6ffc2fb3fdb4ba161e))
* add `sinkrill tui`, a live control panel ([8a41408](https://github.com/amoshydra/sinkrill/commit/8a41408d339c59553a22c2cd6b68aae5be2d1c50))

## [0.3.0](https://github.com/amoshydra/sinkrill/compare/v0.2.1...v0.3.0) (2026-11-09)


### ⚠ BREAKING CHANGES

* the default station name is no longer "Sinkrill". Anything matching on the old default (a Sonos favourite, a saved playlist, or a script reading /status) will see the new name.

### Features

* add `sinkrill retune` as the manual keepalive ([d21e410](https://github.com/amoshydra/sinkrill/commit/d21e410c5cf10941d33d3324c6e3db116b8f3b9e))
* add volume and status commands, explain port clashes ([1d9634a](https://github.com/amoshydra/sinkrill/commit/1d9634a9119ddf825b9a9fcef923c15cec55bbc1))
* name the stream after the machine by default ([a6fbcd9](https://github.com/amoshydra/sinkrill/commit/a6fbcd9c22a4b0626392889a3a9cd5ee509d98eb))


### Bug Fixes

* check the port before discovering speakers ([f08efa6](https://github.com/amoshydra/sinkrill/commit/f08efa6103a8ae6896600b9120b3dfe70f605987))
* detect address-in-use on windows ([8358264](https://github.com/amoshydra/sinkrill/commit/8358264380ca840251fe210e4ef538390dff8be2))
* do not let a second instance silence the first ([d548207](https://github.com/amoshydra/sinkrill/commit/d5482070e95e065266690a1d1f5ade3a173e23c2))
* don't mistake another JSON service for a running instance ([5d19a37](https://github.com/amoshydra/sinkrill/commit/5d19a3789a4ff5719d3eb2670bbe4b50eefb9760))

## [0.2.1](https://github.com/amoshydra/sinkrill/compare/v0.2.0...v0.2.1) (2026-11-09)


### Bug Fixes

* keep the newest audio when a chunk exceeds the client queue ([27f536c](https://github.com/amoshydra/sinkrill/commit/27f536c0b42f8b9f396e2ba4de73df01844186d4))
* point the user unit at /usr/bin/sinkrill ([719eba1](https://github.com/amoshydra/sinkrill/commit/719eba1acd2b0448097e43d0a829af903c4e5e19))

## [0.2.0](https://github.com/amoshydra/sinkrill/compare/v0.1.0...v0.2.0) (2026-11-08)


### ⚠ BREAKING CHANGES

* the Go module path, binary, systemd unit, and AUR package are renamed. `go install github.com/amoshydra/thrum@latest` no longer resolves; use github.com/amoshydra/sinkrill instead.

### Features

* hint when no virtual sink is configured ([0a1c9c9](https://github.com/amoshydra/sinkrill/commit/0a1c9c93033c0a323a5b04bdc072e15ac70ee97a))
* rename the project to Sinkrill ([bc335c5](https://github.com/amoshydra/sinkrill/commit/bc335c592d3610d37d1f0531936581a2fa9ff87e))

## 0.1.0 (2026-11-08)


### ⚠ BREAKING CHANGES

* rebrand as Thrum

### Features

* add 'thrum doctor' and document silence recovery ([cd944ad](https://github.com/amoshydra/thrum/commit/cd944adce624ec0ac57cbb84cbd1336c13ad5e62))
* rebrand as Thrum ([837494f](https://github.com/amoshydra/thrum/commit/837494fdd8b5bb35f5f5b7bb318065547cbbb88d))
* rewrite in Go as a single static binary ([0092549](https://github.com/amoshydra/thrum/commit/0092549b0ca1455d42cd52bf3d12365e962bff32))
* stream PC audio to Sonos with low-latency WAV mode ([37bf4f6](https://github.com/amoshydra/thrum/commit/37bf4f6513d599d91ce47fd9355dd84153512a41))


### Bug Fixes

* bound per-listener queue so live audio cannot drift ([77afa32](https://github.com/amoshydra/thrum/commit/77afa325487241f935335a6596fb0a2eae78ffbf))
* only treat zero-byte capture as stalled ([986d29a](https://github.com/amoshydra/thrum/commit/986d29a5db0df127f475ccd22c2146938cccae44))
* restart stalled capture and wait for the sink monitor ([447c6bd](https://github.com/amoshydra/thrum/commit/447c6bd8e501c7f2e03411ee0fd1a7e69f7d87b5))


### Documentation

* fix banner wave placement ([ca41a72](https://github.com/amoshydra/thrum/commit/ca41a72951ea0ea9e3f86d112ff4dfa833bb3504))


### Chores

* fix leftover sonos references in help text ([9ac56e0](https://github.com/amoshydra/thrum/commit/9ac56e0368d30e93f9d04ae709d2185b550c8fee))


### CI/CD

* add release-please for tagging and releases ([7c0b2f2](https://github.com/amoshydra/thrum/commit/7c0b2f2a4011206d65f7905f6b755fcee21440eb))
* bootstrap the first release at 0.1.0 ([b1fff6b](https://github.com/amoshydra/thrum/commit/b1fff6b16551db32dcdda8923a90c55fd4b71ac0))
* use pre-1.0 versioning for release-please ([abdb80d](https://github.com/amoshydra/thrum/commit/abdb80d64e163cf46e582ad555cb68acb32d57f4))
