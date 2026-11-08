# Changelog

## 1.0.0 (2026-11-08)


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
