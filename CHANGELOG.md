# Changelog

## Unreleased

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
