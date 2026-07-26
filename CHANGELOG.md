# Changelog

All notable changes to tgc are documented in this file.

## [0.2.0](https://github.com/grigoreo-dev/tgc/compare/v0.1.1...v0.2.0) (2026-07-26)


### ⚠ BREAKING CHANGES

* **read:** remove --search; search lives in tgc search --chat (tgc-ruqa)
* **search:** unified search command; drop --messages (tgc-hbqz)

### Features

* awaitBuffer debounce/timeout/dedup engine (tgc-78j) ([5d8bc96](https://github.com/grigoreo-dev/tgc/commit/5d8bc966787929daa4d9ae640dceccd5f88bcc28))
* **cli:** route chats through Peer.OutputMap when --pretty (tgc-w6uu) ([529cfaa](https://github.com/grigoreo-dev/tgc/commit/529cfaa582383c9720cf83d1dbc77f93ebe34ce8))
* **cli:** tgc completion + tgc self setup (tgc-9sz) ([2bd1e93](https://github.com/grigoreo-dev/tgc/commit/2bd1e93f0b9516cc5d1f0762b7623b63ce60fd24))
* ConnectWatch enables live updates for await (tgc-3n9) ([f636fb1](https://github.com/grigoreo-dev/tgc/commit/f636fb15bee9ab27cc9c34beeef964286c876adc))
* **info:** expose premium flag for user cards ([a54ea59](https://github.com/grigoreo-dev/tgc/commit/a54ea59fa8072ec2704e5189435440159513b934))
* **install:** auto-run tgc self setup with TGC_NO_SETUP opt-out (tgc-9sz) ([6697f12](https://github.com/grigoreo-dev/tgc/commit/6697f127367004082d54d9019ed8598302aa87cb))
* **markup:** inline RichText -&gt; Markdown renderer + escaping (tgc-6x9) ([f940626](https://github.com/grigoreo-dev/tgc/commit/f9406267821a6210cc302204626581a13bdc11ad))
* **markup:** PageBlock -&gt; Markdown + RenderRichMessage entry point (tgc-2ub) ([fbba938](https://github.com/grigoreo-dev/tgc/commit/fbba93894bb5146641365424c50cc6d846c7d66d))
* **markup:** table rendering + real [@richtextdemobot](https://github.com/richtextdemobot) golden fixture (tgc-by1) ([6753a6b](https://github.com/grigoreo-dev/tgc/commit/6753a6bd23b108a26a45019a34fa1500bd9b1d00))
* ops.Await drain+live+collectBatch orchestration (tgc-acj) ([3038c32](https://github.com/grigoreo-dev/tgc/commit/3038c324e27f764f15d244243c5b5b1581588ec2))
* ops.MarkRead read-receipt up to max id (tgc-oyk) ([e85eb6c](https://github.com/grigoreo-dev/tgc/commit/e85eb6c70b56127ea829fa836fe0affd74b89922))
* **ops:** auto-fetch Part rich bodies in global SearchMessages (tgc-fl8.5) ([05061b2](https://github.com/grigoreo-dev/tgc/commit/05061b2ef6904d3b6615e2837d89e2fa78cdc2f3))
* **ops:** expose grouped_id for album/media-group messages (tgc-fwe) ([cd4d6f9](https://github.com/grigoreo-dev/tgc/commit/cd4d6f92f8d3e8919bafe3ee29b17f4114bac3b1))
* **ops:** Part=true rich auto-fetch with budget + FLOOD_WAIT stop (tgc-kme) ([6333e6e](https://github.com/grigoreo-dev/tgc/commit/6333e6e492df0294d2a5d27edb6a7486e43ce7c3))
* **ops:** render rich in await (drain may fetch, live stays non-blocking) (tgc-5ai) ([df2f693](https://github.com/grigoreo-dev/tgc/commit/df2f69380ef53139bfb94af72125c00b81cb4912))
* **ops:** render RichMessage into text + rich flag in messageToMap (tgc-bhb) ([f462473](https://github.com/grigoreo-dev/tgc/commit/f46247300ad97cde29733765165b9e32e7a8a187))
* **output:** omit nil fields from pretty map rendering (tgc-3tl3) ([ceed3b6](https://github.com/grigoreo-dev/tgc/commit/ceed3b6f7a28c9c8620d4e88914991ebd9c90530))
* **read:** remove --search; search lives in tgc search --chat (tgc-ruqa) ([a9e9379](https://github.com/grigoreo-dev/tgc/commit/a9e9379a5ea4d05866ab207a06004611acc3331d))
* **resolve:** add Peer.OutputMap safe projection (tgc-m7f1) ([611d812](https://github.com/grigoreo-dev/tgc/commit/611d812869fbe63d1304b35562760860e51c5210))
* **search:** in-chat engine + unified Search orchestrator (tgc-ssbj) ([bfe59f0](https://github.com/grigoreo-dev/tgc/commit/bfe59f09ed1de0463ebd604e14d0977070b9deff))
* **search:** messages section — kind + date flags on SearchGlobal (tgc-gpa2) ([8c68332](https://github.com/grigoreo-dev/tgc/commit/8c68332c591e1217be2da54df4787430f57e698e))
* **search:** peer section — kind filter + AccessHash-free rows (tgc-9ce) ([280bbc3](https://github.com/grigoreo-dev/tgc/commit/280bbc3bbd7b198438c981af5ffc6d84cf043ceb))
* **search:** SearchOpts + flag validation matrix (tgc-pqy) ([ad106ae](https://github.com/grigoreo-dev/tgc/commit/ad106ae84d1f40decc569aa486bbfdd30bcdf021))
* **search:** unified search command; drop --messages (tgc-hbqz) ([26d6ae8](https://github.com/grigoreo-dev/tgc/commit/26d6ae868f33fde11dd0214980118697528800b8))
* **selfupdate:** refresh managed completions after update (tgc-9sz) ([d8b508e](https://github.com/grigoreo-dev/tgc/commit/d8b508e8f5d441d57bc7c1c67d95219f830037a0))
* **setup:** rc-block engine and shell path resolution (tgc-9sz) ([637a35a](https://github.com/grigoreo-dev/tgc/commit/637a35afebb7507fc63952b65689cf5695bb9200))
* **setup:** Run/Remove/RefreshMarked orchestration (tgc-9sz) ([94b4ef9](https://github.com/grigoreo-dev/tgc/commit/94b4ef9ad63d2dd4cfe644eae92d19bda4bca2dc))
* tgc await command + send --await-reply (tgc-6vk) ([211f244](https://github.com/grigoreo-dev/tgc/commit/211f24460fc3003c09f671204681ef1135104ab4))


### Bug Fixes

* await peer-id/megagroup/filter correctness (C1/I1/I2/I3) + M5/M6 (tgc-bzi.1) ([444c62e](https://github.com/grigoreo-dev/tgc/commit/444c62eff373f8463ccdb0d98106655759fcbe85))
* **cli:** non-sticky self setup flags (tgc-9sz) ([619ced5](https://github.com/grigoreo-dev/tgc/commit/619ced53c9c8a04ed52225d4401fc9c3511dc7aa))
* **e2e:** await_bg sets global AWAIT_PID (command-subst subshell left outfile empty) (tgc-zoc) ([fab6afc](https://github.com/grigoreo-dev/tgc/commit/fab6afc172dea9a03025910549a3a38912ed48e3))
* **e2e:** grep -F for nonce match in 02-await bot-await assertion (tgc-zoc) ([805391c](https://github.com/grigoreo-dev/tgc/commit/805391c43b90d745580198df383c5e0a7d98a9ca))
* **init:** init in HOME targets global config dir (tgc-hoq) ([932ec06](https://github.com/grigoreo-dev/tgc/commit/932ec0653712302e241e5135d537b2dce7f8a38a))
* **markup:** checkbox-aware list items + tests for details/depth-cap (tgc-2ub) ([8df45d4](https://github.com/grigoreo-dev/tgc/commit/8df45d43e1aad5ddf98765772293f07340748ec0))
* **markup:** encode Unicode whitespace in rich link/math targets (tgc-fl8.1) ([b7fb39f](https://github.com/grigoreo-dev/tgc/commit/b7fb39f999c51d34bd5e51afa5e9694f85710cfe))
* **markup:** escape untrusted RichMessage link/math targets (tgc-fl8.1) ([6240fa3](https://github.com/grigoreo-dev/tgc/commit/6240fa3ea7c2302c0b52f97f801a516873948279))
* **markup:** math escape invariant + link % encoding (tgc-fl8.1) ([9f73a96](https://github.com/grigoreo-dev/tgc/commit/9f73a9656a16d7dcaa080c35fbb625a0c7345b3d))
* **markup:** UTF-8-safe RichMessage size-cap truncation (tgc-fl8.2) ([887dde3](https://github.com/grigoreo-dev/tgc/commit/887dde3c10c899a0e5b13157aa4ca1ee2a899a6d))
* **ops:** bots skip rich_message path so text is not silently dropped (tgc-9xa) ([c75a313](https://github.com/grigoreo-dev/tgc/commit/c75a313bfacf31a3c867f2d47671715063e40e4e))
* **ops:** flag rich_truncated for Part=true on no-fetch paths (tgc-fl8) ([74513b1](https://github.com/grigoreo-dev/tgc/commit/74513b1d8154f86c82ef0eb681d6825d8f638163))
* **ops:** key-attach global rich map index (no positional pairing) (tgc-fl8.5) ([2dc696a](https://github.com/grigoreo-dev/tgc/commit/2dc696a6073be48aba1a405585d837442eacfc14))
* **ops:** peer-kind keys and peer-aware rich full-fetch matching (tgc-fl8.5) ([a848a61](https://github.com/grigoreo-dev/tgc/commit/a848a611acbf522bd7afdb25f83dd3c00cfd6717))
* **ops:** preserve plain prefix and mention resolve on rich autofetch (tgc-fl8.3, tgc-fl8.4) ([9008791](https://github.com/grigoreo-dev/tgc/commit/900879100fc3b4794a4a05f85a1d141d836e4243))
* **release:** address final review production blockers ([6939a0c](https://github.com/grigoreo-dev/tgc/commit/6939a0c1ad60dc6fe2e89f13b6b426fb6a01e17b))
* **release:** exclude component name from tags ([43294ad](https://github.com/grigoreo-dev/tgc/commit/43294ad34bf61a5b09c8b5dd80d63cc31e4a774d))
* **release:** hand off draft release from release-please to goreleaser ([f418972](https://github.com/grigoreo-dev/tgc/commit/f418972e8eeac142e012f2f2f45cd8465caab8b0))
* **release:** run release please after verification ([#8](https://github.com/grigoreo-dev/tgc/issues/8)) ([7274e46](https://github.com/grigoreo-dev/tgc/commit/7274e46c7398b95d1e28c1c917efe53fd3d825f2))
* **search:** final-review fixes — e2e scripts, hard-fail date validation, README table escaping (tgc-vyp) ([90e2899](https://github.com/grigoreo-dev/tgc/commit/90e28999e0ed0758aecce3f9109573674ca2fa7d))
* **selfupdate:** bound archive extraction size (G110 decompression-bomb guard) (tgc-dlk3) ([9960af3](https://github.com/grigoreo-dev/tgc/commit/9960af31aa8db03b6e4b48183fca7b308778a806))
* **setup:** line-anchored markers and single-block invariant (tgc-9sz) ([3182d7a](https://github.com/grigoreo-dev/tgc/commit/3182d7a012726544db70a8d666eedfea49df9d9a))
* **setup:** never rewrite unmarked files on Run (tgc-9sz) ([b8d452c](https://github.com/grigoreo-dev/tgc/commit/b8d452cd607db14d262e865490e0de9c1e2772d3))
* **setup:** preserve rc permissions and symlink identity (tgc-9sz) ([c385e94](https://github.com/grigoreo-dev/tgc/commit/c385e944457ccc44a87c3489d726a0bd4b7e0e6a))
* update-check cache lives in GlobalDir, not config.Dir() (tgc-1zm) ([46bef88](https://github.com/grigoreo-dev/tgc/commit/46bef880a534ccd696de6e569773c2715b99c5e0))

## [0.1.1] - 2026-07-18

### Added

- Local `./.tgc` project configuration, `tgc init`, and `tgc config path`.

### Fixed

- Local configuration discovery stops before `$HOME`.

## [0.1.0] - 2026-07-18

### Added

- First public release of the agent-first Telegram CLI, including installation and self-update support.

[0.1.1]: https://github.com/grigoreo-dev/tgc/releases/tag/v0.1.1
[0.1.0]: https://github.com/grigoreo-dev/tgc/releases/tag/v0.1.0
