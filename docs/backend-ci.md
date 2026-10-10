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
