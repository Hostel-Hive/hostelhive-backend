# Issue95: Student invitations and account setup

Status: backend implemented on feature/95-student-invitations; live mailbox and
Flutter acceptance remain pending. See [the implemented contract](../student-invitations.md).

## Title

```text
[DDP-#95] feat(identity): add student invitations and university-email account setup
```

## Description

Allow Admins to register approved Students using university email addresses,
individually or through a new bulk-registration workflow. Send an account-setup
invitation so each Student chooses a separate HostelHive password before using
protected application features. University SSO and university-issued passwords
are not part of this workflow.

### Scope

- Add Admin-only individual Student invitation and bulk account/profile registration.
- Use the exact approved university email; configure permitted institutional domains.
- Send setup invitations through Firebase-hosted password setup email.
- Let Students choose their own HostelHive password without Admin knowing it.
- Persist safe invitation/setup status, delivery intent, retries and audit metadata.
- Enforce incomplete-setup restrictions on the backend and expose minimal setup
  status for the clients through /me and /me/setup/complete.
- Provide authorized, rate-limited invitation resend and safe failure recovery.
- Return per-row bulk results and preserve existing profile/guardian validation.
- Update the SRS/SDS amendment, mobile guide, Postman collection and CI tests.

### Behaviour and compatibility

- Firebase manages passwords and credential action codes; PostgreSQL does not
  store passwords, password hashes or usable setup links/action codes.
- University email is the login identifier; the password is specific to HostelHive.
- Do not accept or request the university-issued password. No SSO integration.
- Use server-verifiable Firebase setup evidence, matching the reserved UID and
  approved email; client flags or email-domain checks cannot complete setup.
- Normal protected Student access requires completed setup plus existing active
  account, role and profile checks. Setup never reactivates a disabled account.
- Retries must not duplicate identities/profiles, reset established credentials,
  overwrite archived records or change roles.
- Keep existing active accounts, staff provisioning and #38's profile-only CSV
  importer working. Define a separate contract for bulk account registration.

### Acceptance criteria

- Only authorized Admins can register/invite/resend; CSV cannot select roles or
  active/setup state, and malformed rows have safe actionable results.
- The approved email receives a setup invitation and the Student can set their
  personal HostelHive password and sign in. Admin does not see that password.
- University system credentials are never required or changed.
- Pending setup and unapproved Firebase-only signup cannot access protected features.
- Setup cannot target another identity or bypass deactivation/profile eligibility.
- Delivery failures, expired/reused links, retries, concurrent duplicates and
  interrupted bulk rows are handled without false completion or lost history.
- Logs, database records, API responses and bulk reports expose no password or
  usable setup secret. Provider acceptance and actual email receipt are distinguished.
- Existing accounts and staff permissions retain their current behaviour.
- Automated unit/database/provider tests, manual API/email verification and CI pass;
  record Flutter integration separately until actually verified.

### Out of scope

- University single sign-on or university-password synchronization.
- Collecting university-issued passwords or introducing local password storage.
- Public self-registration granting HostelHive access.
- Replacing general Firebase password recovery.
- Implementing Flutter screens in the Go repository.

Traceability: FR001-FR005, FR006/FR007 and UC004, with the approved Student account
onboarding amendment dated 16 October 2026. Onboarding extensions must not be
mislabelled as new official FR identifiers.
