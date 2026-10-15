# Changelog

## Unreleased

- #89: add optional FCM push for leave/late-access decisions and unapproved attendance incidents. Add owned device registration, durable bounded retries, token rotation/logout and current-permission checks, generic private payloads and migration25 preservation tests. Phone setup and live delivery verification remain pending.

- #87: add Admin/Warden PDF exports for attendance, daily totals, leaves and complaints. Reuse source snapshots/filters, repeat printable headings, wrap/continue long records and include summaries/page numbers. Bound exports to 1000 rows, 200 pages and 16 MiB; add independent decoding, database and visual verification. Document complex-script code point display. No new migration.

- #85: add Admin/Warden Excel exports for attendance movements/daily totals, leave and complaint reports. Include snapshot-consistent full results up to 5000 rows, metadata/totals, literal text safety and bounded concurrent downloads. Pin Excelize v2.11.0; add adapter, integration and workbook validation coverage. No new migration.

- #83: add Admin/Warden complaint reports with submission-date, Student, category and current-status filters; full-period status/category totals and recorded resolution details. Preserve reopened/legacy semantics, private images and source workflows; no new migration.

- #81: add Admin/Warden leave request reports by local submission date, Student and current status, with full-period status totals and preserved decision/history details. Add migration 24 report index and regression coverage; leave and attendance workflows remain unchanged.

- #78: add protected attendance movement/daily reports with local-date filters, recorded-only statistics, zero-activity days and preserved legacy history. Add migration 23's partial report index and required report integration coverage.

- #76: atomically match and consume approved late-access permissions within30 minutes of a recorded scan; record unapproved incidents, preserve IN/OUT movement and notify Admin/Warden.

- #74: Student early-exit/late-entry requests, Admin/Warden decisions, durable inbox alerts and configurable hostel time rules; attendance stays scan-based.

- #72: publish global hostel announcements, Student feed/details, durable inbox
  alerts and staff delivery/read summaries; migration 20 preserves the mixed inbox.

- #70: add leave submission/decision alerts to the shared inbox, preserve complaint
  read/delivery state during migration 19, and verify upgrade compatibility in CI.

- #68: add durable complaint alerts, private inbox, unread counts and idempotent
  mark-read with recipient scope checks and migration 18.

- Add private complaint photos, Pending-only Student changes, scoped reads, safe
  retries, three-image caps and durable cleanup (issue #66; migration 17).
  Share JPEG/PNG normalization with Student profile images; audit photo changes.

- Rename module Go files and matching unit tests to identify their feature and
  layer. Update documentation paths and the coverage fixture; preserve Go contents.

- Add complaint submission, progress notes, staff resolution/reopening and audit
  history with ownership, category scope, safe retries and revision protection
  (issue #64; migration 16). Photos and notifications remain follow-up work.

- Add Student leave requests and Admin/Warden decisions with permanent history,
  ownership checks, safe retries and overlap protection (issue #62; migration 15).
  Leave approval preserves scan-based presence. Add leave integration CI evidence.

- Add versioned Firebase RTDB attendance counts/change notifications (issue #60).
  Migration 000014 adds a transactional publication queue and expiring staff
  grants. Retry/reconciliation preserve PostgreSQL authority; default-deny rules
  and a new emulator CI job verify access and established-listener revocation.
  Realtime is opt-in; live Firebase acceptance requires a configured database.

- Add Admin/Warden attendance dashboard lists and IN/OUT/UNKNOWN counts (issue #57).
  Support literal name/index search and pagination over active, unarchived Student
  profiles. Page totals and rows use one snapshot; historical attendance is kept.
  No new migration is required; realtime publication remains separate.

- Add Admin scanner direction, online QR attendance events and student presence
  (issue #55; migration 000013). Device-scoped event IDs protect retries;
  transactional authority/eligibility checks and cross-device cooldown prevent
  duplicate movements. Admin/Warden can read presence and paginated history.
  Offline capture, realtime projections and hardware verification remain separate.

- Add Admin scanner registration, listing, credential rotation and deactivation
  plus an independently authenticated device-check endpoint (issue #53).
  Migration 000012 stores SHA-256 verifiers for random 256-bit device secrets and
  atomic Admin audit events. No Firebase credentials or plaintext device secrets
  are stored in the scanner tables. Attendance scan ingestion remains separate.

- Add stable student QR PNG retrieval for students themselves and Admin/Warden
  (issue #50; FR010). Migration 000011 stores one unique random identifier per
  profile. Recheck current roles, ownership and eligibility during lazy issuance.
  Test independent PNG decoding, concurrent issuance, persistence and permissions.

- Add Admin-only block, room and bed creation and renaming (issue #48).
  Preserve parent IDs, occupancy and allocation history; enforce scoped uniqueness
  and recheck current Admin authority inside each write transaction.

- Organize identity, student, staff and inventory files by responsibility; separate
  domain errors and service ports, staff self-service and inventory resource queries.

- Add Admin/Warden student bed assignment, atomic transfer and explicit revocation
  with allocation history and local actor auditing (issue #46; FR019–FR020).
- Extend block, room and bed listings to Admins for allocation selection.
- Add allocation permission, validation, history and concurrency tests and require
  allocation database execution in CI coverage evidence.
- Apply migration 000010 for allocation audit columns before running this version.
