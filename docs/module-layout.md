# Module layout and file naming

Reviewed 2026-10-14. Modules follow the existing modular monolith. One cohesive
file per folder is valid; file counts and size alone do not determine quality.

| Layer | Responsibility |
|---|---|
| domain | Business records, states, filters and errors |
| dto | Write request payloads |
| handler | HTTP parsing, response mapping and middleware |
| service | Use cases, validation and named dependency contracts |
| repository | SQL, authority checks, locks and transactions |
| `<module>_routes.go` | Route registration and explicit role policies |

Filenames identify the feature and its layer. Domain records use business names;
DTOs use *_requests.go or *_queries.go. Unit tests use the same stem with _test.go.
See [all renamed paths](file-naming.md).

## Current production files

Paths are relative to internal/modules. Unit tests stay alongside implementation
and are omitted here. Database tests remain under tests/integration.

```text
allocation/
  domain/
    allocation_errors.go
    bed_allocation.go
  dto/
    bed_allocation_requests.go
  handler/
    bed_allocation_handler.go
  repository/
    bed_allocation_repository.go
  service/
    allocation_dependencies.go
    bed_allocation_service.go
  allocation_routes.go
attendance/
  domain/
    attendance_dashboard.go
    attendance_errors.go
    attendance_realtime.go
    attendance_scan.go
    scanner_device.go
  dto/
    attendance_dashboard_requests.go
    attendance_scan_requests.go
    scanner_device_requests.go
  handler/
    attendance_dashboard_handler.go
    attendance_realtime_handler.go
    attendance_scan_handler.go
    scanner_authentication_middleware.go
    scanner_device_handler.go
  repository/
    attendance_dashboard_repository.go
    attendance_realtime_repository.go
    attendance_scan_repository.go
    scanner_device_repository.go
  service/
    attendance_dashboard_service.go
    attendance_dependencies.go
    attendance_realtime_service.go
    attendance_scan_service.go
    scanner_device_service.go
  attendance_routes.go
complaint/
  domain/
    complaint_image.go
    complaint.go
  dto/
    complaint_requests.go
  handler/
    complaint_handler.go
    complaint_image_handler.go
  repository/
    complaint_image_cleanup_repository.go
    complaint_image_repository.go
    complaint_query_repository.go
    complaint_store_repository.go
    complaint_workflow_repository.go
  service/
    complaint_image_dependencies.go
    complaint_image_service.go
    complaint_service.go
  complaint_routes.go
identity/
  domain/
    account_management.go
    account_provisioning.go
    account_role.go
    identity_errors.go
    user_account.go
  dto/
    account_management_requests.go
    account_provisioning_requests.go
  handler/
    account_management_handler.go
    account_provisioning_handler.go
    current_account_handler.go
  repository/
    account_activation_repository.go
    account_lookup_repository.go
    account_management_repository.go
    account_provisioning_repository.go
  service/
    account_activation_service.go
    account_management_service.go
    account_provisioning_service.go
    idempotency_key_validation.go
    identity_dependencies.go
  identity_routes.go
inventory/
  domain/
    hostel_bed.go
    hostel_block.go
    hostel_room.go
    inventory_errors.go
    inventory_filters.go
  dto/
    inventory_requests.go
  handler/
    inventory_listing_handler.go
    inventory_management_handler.go
  repository/
    hostel_bed_repository.go
    hostel_block_repository.go
    hostel_room_repository.go
    inventory_management_repository.go
    inventory_store_repository.go
  service/
    inventory_dependencies.go
    inventory_listing_service.go
    inventory_management_service.go
  inventory_routes.go
leave/
  domain/
    leave_request.go
  dto/
    leave_requests.go
  handler/
    leave_handler.go
  repository/
    leave_request_repository.go
    leave_store_repository.go
  service/
    leave_dependencies.go
    leave_service.go
  leave_routes.go
notification/
  domain/
    notification.go
  dto/
    notification_queries.go
  handler/
    notification_inbox_handler.go
  repository/
    notification_delivery_repository.go
    notification_inbox_repository.go
  service/
    notification_dependencies.go
    notification_inbox_service.go
  notification_routes.go
staff/
  domain/
    staff_errors.go
    staff_profile.go
  dto/
    staff_profile_requests.go
  handler/
    staff_profile_handler.go
    staff_self_profile_handler.go
  repository/
    staff_profile_repository.go
    staff_self_profile_repository.go
  service/
    staff_dependencies.go
    staff_profile_service.go
    staff_self_profile_service.go
  staff_routes.go
student/
  domain/
    student_errors.go
    student_guardian.go
    student_image.go
    student_import.go
    student_profile.go
  dto/
    student_profile_requests.go
  handler/
    student_csv_import_handler.go
    student_image_handler.go
    student_profile_handler.go
    student_qr_handler.go
  repository/
    student_image_repository.go
    student_profile_repository.go
    student_qr_repository.go
  service/
    student_csv_import_service.go
    student_dependencies.go
    student_image_service.go
    student_profile_service.go
    student_profile_validation.go
    student_qr_service.go
  student_routes.go
```

## Boundaries preserved

Identity owns accounts and roles; Student/Staff own their profile workflows.
Inventory owns blocks, rooms and beds; Allocation owns student-to-bed history.
Attendance owns scanners, scans, dashboard queries and realtime coordination.
Leave approval preserves scanned presence. Complaint owns scoped submissions,
notes, staff resolution/reopening and private photos. Notification owns the inbox and complaint-alert delivery; external push remains follow-up work.

Provider adapters remain in internal/platform. Handlers/services do not import
concrete database/provider adapters. App wiring remains in internal/app. Current
permission/eligibility checks and concurrency locks stay inside transactions.
The rename does not create new Go packages, change responsibilities or rename
already-applied migrations.

## Verification

Run .\scripts\test-backend-coverage.ps1 for existing formatting, static, build,
unit/integration and coverage checks in a disposable PostgreSQL database. CI also
checks migration rollback/reapply and race detection. Live provider/staging tests
remain separate. SHA-256 checks confirm each moved Go file retained its contents.

Issue #66 adds complaint_image_handler/service/repository files and cleanup worker. Shared image_normalization.go validates both Student profile and complaint photos; no cross-module repository imports.

Issue #68 adds notification/domain/notification.go, dto/notification_queries.go, service/notification_inbox_service.go and notification_dependencies.go, handler/notification_inbox_handler.go, repository/notification_inbox_repository.go and notification_delivery_repository.go, and notification_routes.go. It uses PostgreSQL event triggers and a worker, without importing another module's concrete repository.
