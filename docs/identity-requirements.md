# Identity backend requirements and remaining verification

Sources: HostelHive SRS FR001–FR005 and ADR-002 in the design repository.
This records implemented backend behavior; it is not a claim that the full
application or all course Definition of Done gates have passed.

| Requirement | Backend behavior | Remaining system verification |
|---|---|---|
| FR001 registration | Administrator provisioning, validated roles, durable idempotency and recovery | Live Firebase provisioning and teammate review |
| FR002 secure login | Firebase revoked-token verification and current PostgreSQL active-account lookup | Frontend sign-in/refresh flows and live expired/revoked token checks |
| FR003 role access | Explicit role guards, current database role, admin-only account management and last-admin protection | Verify the complete endpoint permission matrix as new modules are added |
| FR004 reset/change password | Firebase owns credentials, reset and change workflows; no local password/reset-token store | Implement Firebase client flows in frontend; verify password policy, reset/change and session behavior with the configured project |
| FR005 activation/deactivation | Immediate local deactivation, durable Firebase disable/revoke retry; administrator reactivation after completed revocation, retaining role | Live deactivate/reactivate/fresh-sign-in checks and review |

Automated activation checks cover authorization, malformed inputs, provider
failure, identity matching, pending revocation, incomplete provisioning,
concurrent idempotence and preserving current role. Account integration tests
run against disposable PostgreSQL. Firebase SDK tests use an offline transport.

Before declaring the identity backend done, record live verification, peer
review and applicable CI/coverage/staging evidence. The repository currently has
no GitHub Actions workflow; CI and measured coverage need a separate foundation
task. Do not mark unverified checks complete in a ticket.

RTDB grant cleanup must be integrated when that subsystem is implemented.
Firebase authentication security equivalence and project password/abuse settings
must be confirmed against ADR-002 and the course requirements.

Next audit student FR006–FR009 against the existing implementation, including
image support and search/filter. The user resolved the SRS requirement-table Warden versus UC003 Admin
permission discrepancy on 7 October 2026 by selecting Admin and Warden;
issue #30 implements and documents that policy. Bulk import and
self-service are not assumed requirements without an agreed source.
