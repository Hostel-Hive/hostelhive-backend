# Student registration, invitations and password setup

Decision date: 16 October 2026 (Asia/Colombo).
Status: approved product direction by Tasheen, implemented in backend issue95
on feature/95-student-invitations. Real mailbox and Flutter end-to-end acceptance
remain pending. Supervisor review and controlled incorporation into the original
SRS/SDS PDFs remain pending. This amendment does not rewrite those PDFs.

Implementation contract: [Student invitations and setup](student-invitations.md).
The requirements below describe the approved direction; the contract gives exact
implemented endpoints, limits and proof rules.

## Agreed process

1. Admin approves the Student for HostelHive and registers their university email,
   individually or through the proposed bulk-registration workflow.
2. HostelHive sends a private account-setup invitation to that approved email.
3. The Student opens the invitation and chooses their own HostelHive password
   through a Firebase-managed credential setup flow.
4. After verified setup, the Student signs in using the university email and their
   separate HostelHive password. Current local account and profile eligibility
   still govern access to hostel features.
5. The Student can later change or reset the HostelHive password through Firebase.
   This does not change their university email/system password.

University SSO is out of scope. HostelHive does not request, import, store, verify
or synchronize the university-issued password. An institutional email address does
not automatically connect Firebase to the university login system. No university
identity-provider integration is required for this selected flow.

Admin does not choose or receive the Student's personal HostelHive password.
If provider creation needs a bootstrap secret internally, it must be random,
unknown to Admin/Student and excluded from CSV, logs, durable jobs and responses.
No shared default password or emailed temporary password is part of this design.

## SRS clarification

Traceability: FR001 (registration), FR002 (login), FR003 (authorization), FR004
(password reset/change), FR005 (Admin account status), FR006/FR007 (Student records)
and UC004 (bulk Student import). Onboarding/invitation lifecycle is an approved
scope clarification/extension; do not invent a new official FR number.

- New Student registration uses the exact Admin-approved institutional email.
  Possessing an address in a university domain is not sufficient for access.
- Institution/domain validation must be configurable. stu.kln.ac.lk is the example
  supplied by the user, not evidence of every permitted faculty/domain. Confirm
  the deployment allowlist before enforcing it on newly registered Students.
- Only Admin can approve, register or invite new Students, including bulk actions.
  Public Firebase signup or an email-verification flag alone grants no hostel role,
  profile, allocation or application access.
- A Student sets their personal HostelHive password before normal feature access.
  Setup state must be enforced by the backend, not just a Flutter screen/flag.
- Authentication remains email-based. Student-ID login is not added by this change.
- Staff provisioning and current account/profile permissions remain separate.

This supersedes the unimplemented proposal to give Students a temporary password
and force them to change it after first login. When invitations are enabled, Student creation uses the new invitation endpoint;
legacy password-taking Student provisioning returns409. Staff provisioning stays available.

## SDS design requirements

Firebase remains the only user credential authority. Go verifies Firebase tokens;
PostgreSQL controls approved accounts, roles, active status, setup lifecycle and
resource ownership. Never add local password hashes or an application-managed
password-reset/action-token store.

Separate Admin-controlled is_active from setup completion. A pending invitation
or a successful password reset must not activate a deactivated account. Invited
accounts cannot use protected hostel features until setup is verified; only the
minimal setup/status operations necessary for onboarding may be allowed.

Provision through a durable idempotent reservation and retain one intended
Firebase UID per approved account. Before enabling access, verify the reserved
identity and exact approved email and use server-verifiable evidence of completed
Firebase-managed setup. Do not accept a client setup_complete boolean, a fresh
token issue time or an email-domain check as proof that setup finished. An email
verification flag alone does not prove password setup. Issue95 verifies a trusted passwordUpdatedAt increase after the committed bootstrap
baseline, exact UID/email and fresh unrevoked password sign-in; no client flag can
complete setup. See the implementation contract for the five-minute proof window.

Store invitation intent, delivery attempts and safe lifecycle/audit metadata.
Do not persist or expose plaintext passwords or usable setup links/action codes
in PostgreSQL, logs, CSV reports or Admin responses. Deliver setup material only
to the approved recipient through the chosen Firebase-backed delivery design.
Firebase hosts the action page and handles expired/reused codes. Live code/receipt
verification remains pending; no fixed expiry or resend invalidation is promised.

Support bounded delivery retries and authorized, rate-limited resend. Retrying
must not create duplicate accounts/profiles, reset an established password,
change roles or revive disabled accounts. Expired/reused invitations and stale
resends must not grant access. Provider acceptance is not proof of email receipt.
Record partial provider/database failures without reporting completed setup.

The identity module owns account/invitation lifecycle; the Student module owns
profile and guardian validation. Compose them through service interfaces rather
than importing another module's concrete repository. Firebase adapters handle
provider calls; the HTTP layer does not become a second password authority.

## Bulk registration and the existing CSV importer

The implemented #38 endpoint, POST /api/v1/students/import, accepts existing active
user_id references and creates profiles. It does not create Firebase accounts,
accept university emails as account identifiers or send invitations. Its current
contract remains unchanged by issue95.

The separate issue95 bulk-registration workflow accepts approved Student identity and
profile data including university email and index number, validate guardian and
profile fields, handle valid rows independently, and return a safe per-row report.
Keep passwords, roles and client-selected active/setup flags out of this CSV.
Duplicate emails/index numbers, existing accounts and archived profiles must be
reported without overwriting history, silently linking another identity or
resetting credentials. Support interruption and safe retry of partial rows.

Issue95 defines /api/v1/student-invitations/import and a separate email-based
template. The existing user_id template is not reinterpreted.

## Compatibility and client work

- Preserve current active accounts and their passwords. No forced reset, email
  replacement or retrospective university-domain lockout is authorized here.
- Existing staff setup remains unchanged. Admin deactivation and profile archival
  continue to deny protected Student features.
- Flutter needs invitation-link handling, Student password setup, pending/error
  states and fresh sign-in/token handling, then the existing eligible-account
  checks. These client screens and live email tests remain future work.
- Remove the proposed temporary-password onboarding flow from future plans.
  The updated API guide and Postman collection include setup completion; the
  dedicated invitation contract covers Admin registration and import.

## Acceptance evidence

Verify Admin-only individual/bulk registration and resend, ownership, exact-email
and UID binding, configurable institutional-domain validation, no access from
unapproved signup, setup gating, Firebase failure/expiry/reuse, safe retries and
partial-row recovery, deactivation during setup, secret exclusion, legacy-account
preservation, real email receipt and Flutter setup/sign-in. Existing role,
profile, QR, allocation, attendance and notification regression checks must pass.

Backend implementation and automated checks are tracked under issue95. This
amendment does not certify live email receipt, Flutter integration or deployment.
