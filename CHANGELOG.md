# Changelog

## Unreleased

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
