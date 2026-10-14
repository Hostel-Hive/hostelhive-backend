# Backend CI and coverage â€” issue #42

The workflow runs on PRs to develop/main, pushes to those branches and manual
dispatch. It requires no real Firebase, R2 or deployment credentials. All database
fixtures run serially (`-p 1`) against disposable PostgreSQL migrated from scratch.
The Linux job exercises full rollback/reapply, Go race tests, gofmt, go mod verify,
vet and build. A Windows job checks offline password helpers on PowerShell 5.1.
Coverage covers cmd and internal packages, including repository code exercised by
integration tests. The evidence parser rejects skipped, failed or missing database
package runs. Race detection requires CGO and a C compiler; CI has both.

## Local checks (PowerShell, Docker Desktop running)

```powershell
Set-Location 'C:\Users\TASHEEN\Desktop\Projects\hostelhive-backend'
git branch --show-current
.\scripts\install-migrate.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-firebase-password-tools.ps1
.\scripts\test-backend-coverage.ps1
git diff --check
```

The coverage script needs Go and Python 3 on PATH. It creates and removes its own
container and restores environment variables. It does not migrate the development
database or stop the running API. Use `-Race` if your local compiler supports Go
race detection; the Linux CI race result remains required. Generated files are
ignored under artifacts/: coverage.md (module summary), coverage.html (source
highlighting), coverage.out and tests.jsonl. Open the HTML locally to inspect
uncovered statements. Do not upload arbitrary live test logs or credentials.

## Review and evidence

After pushing and opening the PR, both Backend CI jobs must pass. Open Actions,
select the run for that commit, inspect the coverage summary and download its
coverage artifact. Record the run URL, commit, module percentages, integration
pass/no-skip result and review findings in an issue comment. Reports are retained
14 days; save required evidence with your team before expiry. Coverage measures
executed statements, not all branches or complete requirements. There is no
invented percentage gate; review important uncovered paths explicitly.

Once this workflow has run, configure the develop/main rulesets to require
`Go checks and database coverage` and `Windows PowerShell helper contracts`,
plus teammate review. This change adds checks; it does not itself change GitHub
repository protection settings. No publishing, deployment or production secrets
are part of this workflow. Live Firebase, private R2, staging and HTTPS evidence
belong to the linked hostelhive-infra issue #1.

## Commit and PR

```powershell
git status --short
git add internal/workers/student_images_test.go .github/workflows/backend-ci.yml scripts/check-go-format.py scripts/coverage-report.py scripts/test_coverage_report.py scripts/test-backend-coverage.ps1 docs/backend-ci.md docs/identity-requirements.md README.md .gitignore
git diff --cached --check
git commit -m "ci(backend): verify modules and retain coverage evidence" -m "Refs #42"
git push -u origin feature/42-backend-ci-coverage
```

PR base develop, compare feature/42-backend-ci-coverage. Title:
`ci(backend): verify identity, staff and students`. Description:

```markdown
Add automated Go checks, serial disposable-database integration tests,
race detection, offline PowerShell tests and module coverage artifacts.
Refs #42
```

Record actual results, link the PR through the issue Development sidebar and
request teammate review. Merge only after checks/review; close #42 after evidence
is recorded. Keep infrastructure issue #1 open until live staging verification.

## Measured local baseline (10 October 2026)

All required PostgreSQL integration packages passed without skips on an isolated
database. Statement coverage: identity 89.4%, staff 83.9%, students 86.5%, middleware
100%, Firebase adapter 91.0%; all application code 86.1%. These are local non-race
results, not a hosted Actions run or staging claim. The Linux CI race run remains
to be recorded after the PR is pushed. Keep actual reports with issue evidence.

References:
- https://github.com/actions/setup-go
- https://docs.github.com/en/actions/tutorials/use-containerized-services/create-postgresql-service-containers

Coverage review found the student image-cleanup worker untested. Added regression
checks for retry after failure/empty queue, bounded batches and cancelling in-flight
storage work. No production behavior or profile permissions were changed.

## CI migration metadata regression

The database user and application schema are both named hostelhive. PostgreSQL's
default search_path prefers a schema matching the username once it exists.
Every migrate CLI call must explicitly use the quoted
public.schema_migrations metadata table, as scripts/migrate.ps1 already does.
Otherwise a later CLI call can create an empty metadata table in hostelhive,
report no change on rollback and attempt to recreate the existing schema.
CI now pins metadata and asserts the schema is absent and public metadata empty
after rollback. This rollback runs only on the disposable CI database.

Issue #46 adds allocation to the module coverage summary and mandatory database
package passes. Existing recorded #42 percentages are historical; inspect the
current run artifacts for current coverage.

Issue #48 adds inventory to the module statement-coverage summary and extends
the existing required inventory database suite with HTTP management, scoped
uniqueness, concurrent creation, fresh-role checks and allocation-preservation tests.

Issue #50 extends the required student integration package with QR ownership,
eligibility, concurrent lazy issuance, persisted uniqueness and retrieval checks.
Unit tests decode the generated PNG using an independent QR decoder. CI's existing
rollback/reapply sequence includes migration 000011. Live Firebase and hardware
scanner verification are separate evidence; backend PNG decoding does not prove
ESP32 firmware integration or gate operation.

Issue #53 requires the ninth integration package, attendance, and includes it in
the module coverage summary. It verifies scanner hash-only persistence, Admin
authorization, device/user credential separation, rotation, deactivation, audit
events and concurrent credential updates. Migration 000012 is included in CI's
existing full rollback/reapply check. No firmware or hosted HTTPS test is implied.

Issue #55 extends attendance integration with direction audit, online server times,
cross-device cooldown, first IN/OUT, non-toggling duplicates, event retries,
concurrent deduplication, eligibility, read permissions and stale-authentication
checks. Migration 000013 is included in rollback/reapply. Hardware, live Firebase
and staging HTTPS remain separate verification.

Issue #57 extends the required attendance suite with eligible IN/OUT/UNKNOWN lists
and counts, literal search, deterministic pagination, role restrictions, fresh
transaction authority, preserved history and a presence change between the count
and page queries to verify snapshot consistency. There is no new migration.

Issue #60 adds migration 000014, durable publication/grant integration tests and
real SDK conditional-write tests. Its Firebase RTDB emulator CI job uses Node 22,
Java 21 and pinned npm dependencies. Rules tests check default denial, authorized
staff reads, denied client writes, expiry and established listener cancellation.
Wall-clock expiry alone is not assumed to cancel an existing subscription.

Issue #62 adds leave service/HTTP/PostgreSQL workflow tests. Coverage reporting includes the leave module and requires tests/integration/leave to pass without skips; CI migration rollback/reapply includes version 15.

Issue #64 adds complaint validation and API/PostgreSQL lifecycle, scope and concurrency tests. Coverage reporting requires tests/integration/complaint to pass without skips; migration rollback/reapply includes version 16.

Issue #66 adds complaint_images_integration_test.go to the required complaint package. CI applies/rolls back/reapplies migration 17 and verifies permission scope, failed uploads, cleanup, frozen evidence and concurrent three-image caps using storage fakes.

Issue #68 adds the required notification PostgreSQL integration package and its module coverage. All twelve integration packages must pass without skips; the existing workflow includes migration 18 rollback/reapply and all notification tests.

Issue #70 adds schema 18-to-19 preservation fixtures for existing complaint inbox/read/pending delivery state, plus leave notification recipient/privacy/concurrency and attendance regression checks in the existing notification integration package.

Issue #72 adds the required announcement integration package (thirteen total), announcement module coverage, schema19-to20 mixed-inbox preservation, audience/receipt/publication/concurrency tests and existing notification regressions.

Issue #74 adds the required lateaccess integration package (fourteen total), policy/configuration tests and a populated schema20->21 mixed-inbox preservation check.

Issue #76 extends the attendance integration suite with single-use permission/incident checks and populated schema21->22 preservation of approvals, scans, presence and all four existing inbox sources.
