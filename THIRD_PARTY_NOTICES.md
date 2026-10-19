# Third-party notices

## AntennaPod-derived test fixtures

`testdata/antennapod/legacy-2.0.1.sql` and
`testdata/antennapod/current-3.11.4.sql` reconstruct three `CREATE TABLE`
constants from AntennaPod source code and add synthetic test rows.

- Upstream project: AntennaPod
- Copyright holders: AntennaPod contributors, as identified by the upstream
  repository history
- License: GNU General Public License version 3 only (`GPL-3.0-only`)
- Upstream repository: <https://github.com/AntennaPod/AntennaPod>
- Exact upstream versions and source files:
  - tag [`2.0.1`](https://github.com/AntennaPod/AntennaPod/tree/e1ff4c8763c604c014a62c4c4905737cb92ebebb),
    commit `e1ff4c8763c604c014a62c4c4905737cb92ebebb`, file
    `core/src/main/java/de/danoeh/antennapod/core/storage/PodDBAdapter.java`
  - tag [`3.11.4`](https://github.com/AntennaPod/AntennaPod/tree/6c3492af7239b46e8c9933ff04984fcdd07c3a6f),
    commit `6c3492af7239b46e8c9933ff04984fcdd07c3a6f`, file
    `storage/database/src/main/java/de/danoeh/antennapod/storage/database/PodDBAdapter.java`
  - `develop` snapshot, commit
    [`af577fef0848fe853d1ff960284e9c792f050712`](https://github.com/AntennaPod/AntennaPod/tree/af577fef0848fe853d1ff960284e9c792f050712)
- Upstream license text:
  <https://github.com/AntennaPod/AntennaPod/blob/6c3492af7239b46e8c9933ff04984fcdd07c3a6f/LICENSE>

The complete corresponding source for the retained fixture material, including
these fixture transformations, is conveyed under GPLv3 section 6 through the
public [Beamfall plugin-podcasts repository](https://github.com/Beamfall/plugin-podcasts).
The surrounding plugin is licensed `AGPL-3.0-only`; distribution of this
GPLv3/AGPLv3 combination is permitted by GPLv3/AGPLv3 section 13. No claim is
made that AntennaPod or its original source code has been relicensed.

`testdata/antennapod/unsupported-schema.sql` is an original synthetic fixture
and contains no AntennaPod-derived schema.
