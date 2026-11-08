# Changelog

## [0.1.1](https://github.com/amoshydra/thrum/compare/v0.1.0...v0.1.1) (2026-11-08)


### Documentation

* recommend --virtual-sink in the quick start ([08ea984](https://github.com/amoshydra/thrum/commit/08ea98460a422cfb684f56016219649945333f76))

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
