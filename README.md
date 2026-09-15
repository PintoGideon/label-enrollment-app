# Labeltron Enrollment

Planning and implementation workspace for an end-to-end Labeltron workflow:

```text
Capture -> verified S3 upload -> Rust stitching -> review -> approve -> APID enrollment -> reconcile/report
```

The proposed design extends the existing Windows/PyQt6 application, reuses
`dustid/labeltron-two-stitcher`, and enrolls through direct APID HTTP calls.
It does not bundle or invoke `clid`.

## Documents

- [Detailed design](DESKTOP_APP_PLAN.md)
- [System architecture and ASCII diagrams](SYSTEM_DESIGN.md)
- [Implementation slices, dependencies, and TODOs](IMPLEMENTATION_SLICES.md)
- [Source revisions, evidence, and verification limits](SOURCES.md)
- [Historical plan — superseded](PLAN.md)

## Development status

Application implementation has not started. The backlog begins with **S00:
lock the pilot scope and unblock access**. Required decisions and approved
nonproduction fixtures/access must be resolved before starting S01.

Use a separate branch for each slice. Keep tests, review evidence, and scope
changes with that slice; do not mark proposed or mocked behavior as validated.
Creating this repository does not yet decide where the Workflow backend will
live; that remains an S00 decision.

## Local references and sensitive data

`reference/` contains local, independently managed source checkouts for study.
It is excluded by `.gitignore` and is not part of this repository's commits or
GitHub content. Source provenance is recorded in `SOURCES.md` instead.

Do not force-add reference checkouts, credentials, raw captures, or private
fixture data. Follow each reference repository's instructions; in particular,
do not read or change the stitcher's protected `manifests/` directory.
