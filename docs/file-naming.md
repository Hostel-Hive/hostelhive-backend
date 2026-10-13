# Module file naming

Reviewed 2026-10-13 under the user's request for filenames that reveal their purpose.

Use lowercase snake_case and a feature name. Workflow files include their layer:
student_profile_handler.go, student_profile_service.go and
student_profile_repository.go. DTOs describe payloads, such as complaint_requests.go.
Domain files describe business records, such as hostel_bed.go. Routing uses
`<module>_routes.go`. Dependency contracts use `<module>_dependencies.go`. Matching
unit tests keep the same stem plus _test.go. Already-applied migrations retain
their numbered names. Do not add extra packages merely to rename files.

All 141 renamed files had identical SHA-256 hashes before and after the move.
Packages, imports, endpoints, JSON, permissions and database schema are unchanged
by this cleanup. Existing issue #64 changes remain in the working branch.

## Rename reference

Previous paths are historical names, not current command paths.

| Previous path | Current path |
|---|---|
| `internal/modules/complaint/service/service_test.go` | `internal/modules/complaint/service/complaint_service_test.go` |
| `internal/modules/complaint/service/service.go` | `internal/modules/complaint/service/complaint_service.go` |
| `internal/modules/complaint/routes.go` | `internal/modules/complaint/complaint_routes.go` |
| `internal/modules/inventory/service/service.go` | `internal/modules/inventory/service/inventory_listing_service.go` |
| `internal/modules/inventory/service/ports.go` | `internal/modules/inventory/service/inventory_dependencies.go` |
| `internal/modules/inventory/service/management.go` | `internal/modules/inventory/service/inventory_management_service.go` |
| `internal/modules/inventory/routes.go` | `internal/modules/inventory/inventory_routes.go` |
| `internal/modules/identity/service/request_key.go` | `internal/modules/identity/service/idempotency_key_validation.go` |
| `internal/modules/identity/service/provisioning_test.go` | `internal/modules/identity/service/account_provisioning_service_test.go` |
| `internal/modules/identity/service/provisioning.go` | `internal/modules/identity/service/account_provisioning_service.go` |
| `internal/modules/identity/service/ports.go` | `internal/modules/identity/service/identity_dependencies.go` |
| `internal/modules/identity/service/activation_test.go` | `internal/modules/identity/service/account_activation_service_test.go` |
| `internal/modules/identity/service/activation.go` | `internal/modules/identity/service/account_activation_service.go` |
| `internal/modules/identity/service/accounts.go` | `internal/modules/identity/service/account_management_service.go` |
| `internal/modules/identity/routes.go` | `internal/modules/identity/identity_routes.go` |
| `internal/modules/allocation/service/service.go` | `internal/modules/allocation/service/bed_allocation_service.go` |
| `internal/modules/allocation/service/ports.go` | `internal/modules/allocation/service/allocation_dependencies.go` |
| `internal/modules/allocation/routes.go` | `internal/modules/allocation/allocation_routes.go` |
| `internal/modules/allocation/repository/postgres.go` | `internal/modules/allocation/repository/bed_allocation_repository.go` |
| `internal/modules/complaint/repository/queries.go` | `internal/modules/complaint/repository/complaint_query_repository.go` |
| `internal/modules/complaint/repository/postgres.go` | `internal/modules/complaint/repository/complaint_store_repository.go` |
| `internal/modules/complaint/repository/changes.go` | `internal/modules/complaint/repository/complaint_workflow_repository.go` |
| `internal/modules/allocation/handler/handler_test.go` | `internal/modules/allocation/handler/bed_allocation_handler_test.go` |
| `internal/modules/allocation/handler/handler.go` | `internal/modules/allocation/handler/bed_allocation_handler.go` |
| `internal/modules/leave/service/service_test.go` | `internal/modules/leave/service/leave_service_test.go` |
| `internal/modules/leave/service/service.go` | `internal/modules/leave/service/leave_service.go` |
| `internal/modules/leave/service/ports.go` | `internal/modules/leave/service/leave_dependencies.go` |
| `internal/modules/leave/routes.go` | `internal/modules/leave/leave_routes.go` |
| `internal/modules/inventory/repository/rooms.go` | `internal/modules/inventory/repository/hostel_room_repository.go` |
| `internal/modules/inventory/repository/postgres.go` | `internal/modules/inventory/repository/inventory_store_repository.go` |
| `internal/modules/inventory/repository/management.go` | `internal/modules/inventory/repository/inventory_management_repository.go` |
| `internal/modules/inventory/repository/blocks.go` | `internal/modules/inventory/repository/hostel_block_repository.go` |
| `internal/modules/inventory/repository/beds.go` | `internal/modules/inventory/repository/hostel_bed_repository.go` |
| `internal/modules/allocation/dto/request.go` | `internal/modules/allocation/dto/bed_allocation_requests.go` |
| `internal/modules/leave/repository/requests.go` | `internal/modules/leave/repository/leave_request_repository.go` |
| `internal/modules/leave/repository/postgres.go` | `internal/modules/leave/repository/leave_store_repository.go` |
| `internal/modules/allocation/domain/errors.go` | `internal/modules/allocation/domain/allocation_errors.go` |
| `internal/modules/allocation/domain/allocation.go` | `internal/modules/allocation/domain/bed_allocation.go` |
| `internal/modules/identity/repository/provisioning.go` | `internal/modules/identity/repository/account_provisioning_repository.go` |
| `internal/modules/identity/repository/lookup_test.go` | `internal/modules/identity/repository/account_lookup_repository_test.go` |
| `internal/modules/identity/repository/lookup.go` | `internal/modules/identity/repository/account_lookup_repository.go` |
| `internal/modules/identity/repository/activation.go` | `internal/modules/identity/repository/account_activation_repository.go` |
| `internal/modules/identity/repository/accounts.go` | `internal/modules/identity/repository/account_management_repository.go` |
| `internal/modules/complaint/handler/handler.go` | `internal/modules/complaint/handler/complaint_handler.go` |
| `internal/modules/leave/handler/handler.go` | `internal/modules/leave/handler/leave_handler.go` |
| `internal/modules/inventory/handler/management_test.go` | `internal/modules/inventory/handler/inventory_management_handler_test.go` |
| `internal/modules/inventory/handler/management.go` | `internal/modules/inventory/handler/inventory_management_handler.go` |
| `internal/modules/inventory/handler/handler_test.go` | `internal/modules/inventory/handler/inventory_listing_handler_test.go` |
| `internal/modules/inventory/handler/handler.go` | `internal/modules/inventory/handler/inventory_listing_handler.go` |
| `internal/modules/leave/dto/request.go` | `internal/modules/leave/dto/leave_requests.go` |
| `internal/modules/identity/handler/provisioning_test.go` | `internal/modules/identity/handler/account_provisioning_handler_test.go` |
| `internal/modules/identity/handler/provisioning.go` | `internal/modules/identity/handler/account_provisioning_handler.go` |
| `internal/modules/identity/handler/me_test.go` | `internal/modules/identity/handler/current_account_handler_test.go` |
| `internal/modules/identity/handler/me.go` | `internal/modules/identity/handler/current_account_handler.go` |
| `internal/modules/identity/handler/activation_test.go` | `internal/modules/identity/handler/account_activation_handler_test.go` |
| `internal/modules/identity/handler/accounts_test.go` | `internal/modules/identity/handler/account_management_handler_test.go` |
| `internal/modules/identity/handler/accounts.go` | `internal/modules/identity/handler/account_management_handler.go` |
| `internal/modules/inventory/dto/request.go` | `internal/modules/inventory/dto/inventory_requests.go` |
| `internal/modules/leave/domain/leave.go` | `internal/modules/leave/domain/leave_request.go` |
| `internal/modules/student/service/import.go` | `internal/modules/student/service/student_csv_import_service.go` |
| `internal/modules/student/service/images_test.go` | `internal/modules/student/service/student_image_service_test.go` |
| `internal/modules/student/service/images.go` | `internal/modules/student/service/student_image_service.go` |
| `internal/modules/student/routes.go` | `internal/modules/student/student_routes.go` |
| `internal/modules/student/service/ports.go` | `internal/modules/student/service/student_dependencies.go` |
| `internal/modules/student/service/import_test.go` | `internal/modules/student/service/student_csv_import_service_test.go` |
| `internal/modules/student/service/qr.go` | `internal/modules/student/service/student_qr_service.go` |
| `internal/modules/student/service/qr_test.go` | `internal/modules/student/service/student_qr_service_test.go` |
| `internal/modules/complaint/dto/request.go` | `internal/modules/complaint/dto/complaint_requests.go` |
| `internal/modules/inventory/domain/room.go` | `internal/modules/inventory/domain/hostel_room.go` |
| `internal/modules/inventory/domain/query.go` | `internal/modules/inventory/domain/inventory_filters.go` |
| `internal/modules/inventory/domain/errors.go` | `internal/modules/inventory/domain/inventory_errors.go` |
| `internal/modules/inventory/domain/block.go` | `internal/modules/inventory/domain/hostel_block.go` |
| `internal/modules/inventory/domain/bed.go` | `internal/modules/inventory/domain/hostel_bed.go` |
| `internal/modules/identity/dto/request.go` | `internal/modules/identity/dto/account_management_requests.go` |
| `internal/modules/identity/dto/provisioning.go` | `internal/modules/identity/dto/account_provisioning_requests.go` |
| `internal/modules/student/service/validation_test.go` | `internal/modules/student/service/student_profile_validation_test.go` |
| `internal/modules/student/service/validation.go` | `internal/modules/student/service/student_profile_validation.go` |
| `internal/modules/student/service/service.go` | `internal/modules/student/service/student_profile_service.go` |
| `internal/modules/student/domain/image.go` | `internal/modules/student/domain/student_image.go` |
| `internal/modules/student/domain/guardian.go` | `internal/modules/student/domain/student_guardian.go` |
| `internal/modules/student/domain/errors.go` | `internal/modules/student/domain/student_errors.go` |
| `internal/modules/student/domain/import.go` | `internal/modules/student/domain/student_import.go` |
| `internal/modules/student/domain/student.go` | `internal/modules/student/domain/student_profile.go` |
| `internal/modules/identity/domain/accounts.go` | `internal/modules/identity/domain/account_management.go` |
| `internal/modules/student/handler/images_test.go` | `internal/modules/student/handler/student_image_handler_test.go` |
| `internal/modules/student/handler/images.go` | `internal/modules/student/handler/student_image_handler.go` |
| `internal/modules/student/handler/handler_test.go` | `internal/modules/student/handler/student_profile_handler_test.go` |
| `internal/modules/attendance/service/scans_test.go` | `internal/modules/attendance/service/attendance_scan_service_test.go` |
| `internal/modules/student/handler/import_test.go` | `internal/modules/student/handler/student_csv_import_handler_test.go` |
| `internal/modules/attendance/service/scans.go` | `internal/modules/attendance/service/attendance_scan_service.go` |
| `internal/modules/student/handler/import.go` | `internal/modules/student/handler/student_csv_import_handler.go` |
| `internal/modules/student/handler/handler.go` | `internal/modules/student/handler/student_profile_handler.go` |
| `internal/modules/attendance/service/realtime.go` | `internal/modules/attendance/service/attendance_realtime_service.go` |
| `internal/modules/attendance/service/ports.go` | `internal/modules/attendance/service/attendance_dependencies.go` |
| `internal/modules/attendance/service/devices_test.go` | `internal/modules/attendance/service/scanner_device_service_test.go` |
| `internal/modules/attendance/service/devices.go` | `internal/modules/attendance/service/scanner_device_service.go` |
| `internal/modules/attendance/service/dashboard_test.go` | `internal/modules/attendance/service/attendance_dashboard_service_test.go` |
| `internal/modules/attendance/service/dashboard.go` | `internal/modules/attendance/service/attendance_dashboard_service.go` |
| `internal/modules/student/handler/qr.go` | `internal/modules/student/handler/student_qr_handler.go` |
| `internal/modules/attendance/routes.go` | `internal/modules/attendance/attendance_routes.go` |
| `internal/modules/identity/domain/user.go` | `internal/modules/identity/domain/user_account.go` |
| `internal/modules/identity/domain/role.go` | `internal/modules/identity/domain/account_role.go` |
| `internal/modules/identity/domain/provisioning.go` | `internal/modules/identity/domain/account_provisioning.go` |
| `internal/modules/identity/domain/errors.go` | `internal/modules/identity/domain/identity_errors.go` |
| `internal/modules/student/handler/qr_test.go` | `internal/modules/student/handler/student_qr_handler_test.go` |
| `internal/modules/attendance/repository/scans.go` | `internal/modules/attendance/repository/attendance_scan_repository.go` |
| `internal/modules/attendance/repository/realtime.go` | `internal/modules/attendance/repository/attendance_realtime_repository.go` |
| `internal/modules/attendance/repository/devices.go` | `internal/modules/attendance/repository/scanner_device_repository.go` |
| `internal/modules/attendance/repository/dashboard.go` | `internal/modules/attendance/repository/attendance_dashboard_repository.go` |
| `internal/modules/staff/dto/request.go` | `internal/modules/staff/dto/staff_profile_requests.go` |
| `internal/modules/attendance/dto/scan.go` | `internal/modules/attendance/dto/attendance_scan_requests.go` |
| `internal/modules/attendance/dto/device.go` | `internal/modules/attendance/dto/scanner_device_requests.go` |
| `internal/modules/attendance/dto/dashboard.go` | `internal/modules/attendance/dto/attendance_dashboard_requests.go` |
| `internal/modules/attendance/domain/scan.go` | `internal/modules/attendance/domain/attendance_scan.go` |
| `internal/modules/attendance/domain/realtime.go` | `internal/modules/attendance/domain/attendance_realtime.go` |
| `internal/modules/attendance/domain/errors.go` | `internal/modules/attendance/domain/attendance_errors.go` |
| `internal/modules/attendance/handler/authentication.go` | `internal/modules/attendance/handler/scanner_authentication_middleware.go` |
| `internal/modules/attendance/domain/device.go` | `internal/modules/attendance/domain/scanner_device.go` |
| `internal/modules/attendance/handler/dashboard.go` | `internal/modules/attendance/handler/attendance_dashboard_handler.go` |
| `internal/modules/attendance/domain/dashboard.go` | `internal/modules/attendance/domain/attendance_dashboard.go` |
| `internal/modules/staff/routes.go` | `internal/modules/staff/staff_routes.go` |
| `internal/modules/attendance/handler/dashboard_test.go` | `internal/modules/attendance/handler/attendance_dashboard_handler_test.go` |
| `internal/modules/staff/repository/self.go` | `internal/modules/staff/repository/staff_self_profile_repository.go` |
| `internal/modules/staff/repository/postgres.go` | `internal/modules/staff/repository/staff_profile_repository.go` |
| `internal/modules/staff/service/ports.go` | `internal/modules/staff/service/staff_dependencies.go` |
| `internal/modules/staff/service/self.go` | `internal/modules/staff/service/staff_self_profile_service.go` |
| `internal/modules/attendance/handler/devices_test.go` | `internal/modules/attendance/handler/scanner_device_handler_test.go` |
| `internal/modules/attendance/handler/devices.go` | `internal/modules/attendance/handler/scanner_device_handler.go` |
| `internal/modules/staff/service/service.go` | `internal/modules/staff/service/staff_profile_service.go` |
| `internal/modules/attendance/handler/realtime.go` | `internal/modules/attendance/handler/attendance_realtime_handler.go` |
| `internal/modules/attendance/handler/scans_test.go` | `internal/modules/attendance/handler/attendance_scan_handler_test.go` |
| `internal/modules/attendance/handler/scans.go` | `internal/modules/attendance/handler/attendance_scan_handler.go` |
| `internal/modules/staff/domain/staff.go` | `internal/modules/staff/domain/staff_profile.go` |
| `internal/modules/staff/domain/errors.go` | `internal/modules/staff/domain/staff_errors.go` |
| `internal/modules/staff/handler/handler.go` | `internal/modules/staff/handler/staff_profile_handler.go` |
| `internal/modules/staff/handler/handler_test.go` | `internal/modules/staff/handler/staff_profile_handler_test.go` |
| `internal/modules/staff/handler/self.go` | `internal/modules/staff/handler/staff_self_profile_handler.go` |
| `internal/modules/student/repository/qr.go` | `internal/modules/student/repository/student_qr_repository.go` |
| `internal/modules/student/dto/request.go` | `internal/modules/student/dto/student_profile_requests.go` |
| `internal/modules/student/repository/postgres.go` | `internal/modules/student/repository/student_profile_repository.go` |
| `internal/modules/student/repository/images.go` | `internal/modules/student/repository/student_image_repository.go` |

## Verification result

Local checks passed on 2026-10-13: formatting, dependency verification, vet, build, all unit/architecture tests and all eleven required PostgreSQL integration packages without skips. Statement coverage remains 85.3% overall (3322/3893). CI race checks and live provider verification remain separate.
