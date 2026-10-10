# Module layout review

Reviewed on 10 October 2026 against HostelHive_Backend_Folder_Structure.pdf.
The PDF is a reference architecture, not a required file count or a list of
features that must already exist. The backend follows its modular monolith and
layer boundaries. It adds staff and inventory for the features implemented since
the reference was written. Allocation is also checked as part of issue #46.

## Responsibilities and file boundaries

| Layer | Contents |
| --- | --- |
| `domain` | Business records, role definitions and module error values |
| `dto` | HTTP write payloads, including inventory creation and renaming |
| `handler` | HTTP parsing, response status mapping and calls to service contracts |
| `service` | Use cases, input normalization and dependency interfaces in `ports.go` |
| `repository` | SQL, transactions, locks and database error mapping |
| `routes.go` | Route registration and explicit authentication/role policies |

One file in a folder is valid when it has one cohesive responsibility. Small
handlers and DTOs do not need artificial per-endpoint files. Separate files are
useful for different workflows or resource queries. Files within the same folder
still form one Go package; this review does not add subpackages for every entity.

## Current production files

Names below are relative to `internal/modules`. Unit test files are omitted for
readability; they remain beside the implementation. Database integration tests
remain under `tests/integration`, and dependency checks under `tests/architecture`.

```text
identity/
  domain/
    user.go              Account record
    role.go              Role constants and validation
    accounts.go          Account page and management result
    provisioning.go      Provisioning reservation
    errors.go            Account, provisioning and activation errors
  dto/
    request.go           Account-management payloads
    provisioning.go      Provisioning payload
  handler/
    me.go                Current account
    accounts.go          Admin account management
    activation.go        Reactivation
    provisioning.go      Account creation
  service/
    ports.go             Repository and provider interfaces
    accounts.go          Account-management use cases
    activation.go        Reactivation coordination
    provisioning.go      Provisioning coordination
    request_key.go       Idempotency key validation
  repository/
    lookup.go            Current account lookup
    accounts.go          Management transactions and revocation work
    activation.go        Activation transaction
    provisioning.go      Provisioning reservations and completion
  routes.go

student/
  domain/
    student.go           Profile, list filters and page
    guardian.go          Guardian records
    image.go             Image metadata and limits
    import.go            CSV row results and report
    errors.go
  dto/request.go
  handler/
    handler.go           Profile CRUD and listing
    images.go            Image upload, retrieval and removal
    import.go            CSV import
    qr.go                Authenticated QR PNG retrieval
  service/
    ports.go             Profile, image, object-storage and import interfaces
    service.go           Profile use cases
    validation.go        Profile and guardian validation
    images.go            Image normalization and storage coordination
    import.go            CSV parsing and per-row import
    qr.go                Random identifier generation and QR rendering
  repository/
    postgres.go          Profile/guardian transactions and import persistence
    images.go            Image attachment and cleanup persistence
    qr.go                Eligibility, ownership and stable issuance transaction
  routes.go

staff/
  domain/
    staff.go             Profile and list results
    errors.go
  dto/request.go
  handler/
    handler.go           Admin profile management and own-profile viewing
    self.go              Own-profile name creation/update
  service/
    ports.go
    service.go           Admin management and reads
    self.go              Own-profile name validation
  repository/
    postgres.go          Admin writes, reads and shared scanning
    self.go              Ownership-checked self-service transaction
  routes.go

inventory/
  domain/
    block.go
    room.go
    bed.go
    query.go             Shared filters and paginated results
    errors.go
  dto/request.go         Create and rename payloads; parents excluded from updates
  handler/
    handler.go           Block, room and bed listing HTTP handlers
    management.go        Admin-only create and rename handlers
  service/
    ports.go
    service.go           Listing validation and delegation
    management.go        Create/rename validation and normalization
  repository/
    postgres.go          Store, optional filters and snapshot pagination helper
    management.go        Fresh Admin checks, write transaction and error mapping
    blocks.go            Block queries
    rooms.go             Room queries and capacity/occupancy aggregation
    beds.go              Bed queries and availability
  routes.go

allocation/
  domain/
    allocation.go        Allocation record, filters and page
    errors.go
  dto/request.go
  handler/handler.go
  service/
    ports.go
    service.go           Assignment, transfer, revocation and list use cases
  repository/postgres.go Atomic allocation transactions and history queries
  routes.go
```

## Boundaries preserved

- Identity owns common accounts, roles and account state. Firebase adapters stay
  in `internal/platform/firebase`; credentials are not moved into profile modules.
- Student owns student-specific fields, guardians, images and profile imports.
  Guardians remain in the profile transaction rather than becoming an independent
  module or repository with a separate transaction.
- Staff owns staff profile details. Self-service derives ownership from the
  authenticated account; only Admin management can change designation.
- Inventory owns physical blocks, rooms and beds. Allocation owns the changing
  student-to-bed relationship and its history. Sharing the database does not mean
  these workflows must share a module.
- SQL authorization, eligibility checks and concurrency locks stay inside their
  transactions. File organization must not turn atomic checks into earlier reads.
- Response JSON and exported types keep their contracts. No migration versions,
  endpoints or permissions are changed by this organization review.

Future attendance, leave, complaint, notice and report modules are added when
implemented. The reference's future adapters and sample migration filenames do
not require empty folders or renaming already-applied migrations.

## Verification

Run the complete build, static and disposable-database checks from the repository:

```powershell
.\scripts\test-backend-coverage.ps1
```

The script creates and removes a separate database and generates local evidence
in `artifacts/coverage.md`, `artifacts/coverage.html` and `artifacts/tests.jsonl`.
The dependency test checks for concrete adapters in handlers/services and for
imports of another module's repository. Live Firebase and staging checks remain
separate from these local tests.

Recorded local results, 10 October 2026: formatting, dependency verification,
static checks, build, module and architecture tests passed. The full coverage run
passed all eight required PostgreSQL integration packages with no skipped tests.
Overall statement coverage remained 1751/2011 (87.1%): identity 89.4%, staff 83.9%,
student 86.5%, allocation 93.1%. This was a local non-race run; live provider,
PR CI and staging verification are separate.
