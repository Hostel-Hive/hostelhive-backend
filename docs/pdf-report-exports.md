# PDF report exports - issue #87

Implements the PDF portion of SRS FR031 for the same source reports as Excel
exports (#85). Existing JSON and Excel endpoints retain their contracts.

## API

Active Admin and Warden accounts can GET:

| Route | Optional filters |
|---|---|
| /api/v1/reports/attendance/export/pdf | student_id, direction |
| /api/v1/reports/attendance/daily/export/pdf | student_id, direction |
| /api/v1/reports/leaves/export/pdf | student_id, status |
| /api/v1/reports/complaints/export/pdf | student_id, category, status |

Required: from=YYYY-MM-DD&to=YYYY-MM-DD, inclusive dates in HOSTEL_TIMEZONE
(default Asia/Colombo), at most 366 calendar dates. Student ID is a canonical
lowercase nonzero profile UUID. Directions are IN/OUT. Leave statuses are
pending/approved/rejected; complaint statuses pending/in_progress/resolved;
complaint categories maintenance/disciplinary. Values are case-sensitive.

No limit, offset or format parameter is accepted. Even offset=0 is invalid.
Missing/empty/duplicate/unknown/malformed parameters, wrong-route filters,
invalid/reversed/oversized dates and GET request bodies return HTTP 400
invalid_report_query. Authentication failures return HTTP 401; Student, Sub-Warden,
Security and inactive staff cannot export (403 with valid current authentication).

The successful response is application/pdf, with attachment filename
hostelhive-<report-kind>-<from>-to-<to>.pdf, exact Content-Length,
Cache-Control: no-store and X-Content-Type-Options: nosniff. Generation completes
in memory before headers are sent. A failure returns JSON, never a partial PDF
with HTTP 200. HTTP 503 service_unavailable covers database/render/deadline failures.

## Limits

- At most 1000 Data records, excluding summary and headings.
- At most 200 pages, including summary pages.
- At most 16 MiB of generated PDF bytes.
- Exceeding any limit returns HTTP 413 report_export_too_large without truncation.
  Narrow the selected period or filters. Long records can exceed the page limit
  even when the total is below1000.
- Two concurrent requests across the four PDF routes per shared PDF API instance;
  another receives HTTP 429 report_export_busy. PDF and Excel have separate two-slot
  admission pools; at most four export requests are active per server instance.
- Existing ACCOUNT_MANAGEMENT_TIMEOUT bounds source query and rendering (8s by
  default). Existing HTTP write timeout remains 15s. No new environment setting.

Daily row totals count calendar dates, including zero-activity days, not scans.
Underlying daily movement counts may exceed 1000 because the cap counts Data rows.

## Printable layout and content

A4 landscape, embedded Go regular/bold fonts, navy table headings, alternating row
fills, fixed printable margins, and page N of M footers. Summary pages list all
applied filters, date/status interpretation, full-period totals, exported count
and row limit. Data starts on a separate page. Every page repeats its section and
column headers. Empty record reports display No matching records; empty daily
results still have one zero-count row per selected date.

Daily attendance uses a date/count table. Other reports group labeled fields into
four readable table columns plus a source row number. All fields from the Excel
mapping remain present, including IDs, timestamps, decision and current resolution
facts. Long unbroken IDs/URLs and paragraphs wrap inside their columns. Long
records continue onto later pages; a + after their row number marks continuation.
No user text is parsed as HTML or converted to links, scripts or attachments.

Timestamps preserve the original instant as RFC3339 text in the hostel timezone.
IDs, versions and counts are exact strings; there is no Excel-style numeric
precision loss. Attendance includes recorded outcomes only; legacy missing access
results stay blank. Leave/complaint ranges use submission date, and status means
current status at the query snapshot. Pending decisions, legacy unspecified
resolutions and reopened complaints retain the same blank-field semantics as JSON
and Excel. Resolved complaints use the current matching resolution audit.

### Text display limitation

The embedded font supports Latin, Greek and Cyrillic text. Unsupported glyphs,
control characters and complex scripts (including Sinhala/Tamil) are displayed
as explicit Unicode code point escapes such as \u{0DC3}; backslashes are doubled
and tabs shown as \t. This prevents silent loss or incorrect script shaping but
is NOT native Sinhala/Tamil PDF rendering. A visible Summary note explains the
representation. Excel retains original Unicode text. Native complex-script
shaping/font support is a separate enhancement; it is not claimed complete here.

## Source safety and architecture

One bounded source statement retrieves rows and full-period totals using the
existing repositories and their current staff-authority checks. There is no JSON
pagination loop or cross-request export snapshot. Read-only exports retain
archived/inactive Student and disabled scanner history. Private photo metadata,
object keys, credentials, QR credentials and contact data are excluded. Source
records, permission consumption, images, notifications and realtime projections
are not updated by exports. Downloaded PDFs are local copies, not live grants.

The shared report service maps source facts into the format-neutral ExportDocument
and takes a writer port. Excel and PDF constructors select their own row cap and
suffix. HTTP download validation/error handling is shared; PDF construction lives
in internal/platform/pdfreport. The pinned github.com/signintech/gopdf renderer
and embedded golang.org/x/image fonts need no host fonts, Node, browser, external
PDF service or runtime network access. The independent github.com/ledongthuc/pdf
reader is imported only by tests. Architecture checks keep renderer/font SDKs out
of module handler/service/domain/DTO layers; coverage includes the PDF adapter.

No migration is added; schema 24 suffices. Rebuild and restart the backend to load
the routes. Existing Firebase, R2 and RTDB settings remain unchanged.

## Verification

Run scripts/test-backend-coverage.ps1 with Docker Desktop running. It creates its
own disposable database. Existing 15 mandatory PostgreSQL integration packages
remain required without skips. Unit tests decode PDFs with an independent reader,
verify printable text boundaries, repeated headings, page footers, literal text,
full long-text continuation, empty results, Unicode representation, cancellation,
resource bounds and concurrency/error responses. Database tests compare actual
PDF fields/order/totals against source Excel/SQL and exercise all four routes,
filters, roles, archived/inactive history, stale authority and unchanged source,
queue/image/permission/incident state. Exact row-boundary tests cover both formats.

For synthetic visual samples (no real account data):

```powershell
$env:PDF_ARTIFACT_DIR = Join-Path $PWD 'artifacts\pdf-review'
try {
    go test ./internal/platform/pdfreport -run TestPDFReportsTextPaginationAndGeometry -count=1
    if ($LASTEXITCODE -ne 0) { throw 'PDF sample verification failed.' }
} finally {
    Remove-Item Env:PDF_ARTIFACT_DIR
}
```

Open sample-attendance.pdf, sample-daily.pdf, sample-leaves.pdf and
sample-complaints.pdf. Inspect Summary, first/middle/last Data pages, wrapped IDs,
long-description continuations and page footers. Rendered-page review supplements
text extraction; live Firebase-authenticated API tests and remote CI are separate
acceptance evidence.

Manual API example with existing Admin headers:

```powershell
Invoke-WebRequest -UseBasicParsing `
    -Uri "$baseURL/api/v1/reports/complaints/export/pdf?from=2026-10-01&to=2026-10-15" `
    -Headers $adminHeaders `
    -OutFile artifacts/complaint-report.pdf
```

Check HTTP status and application/pdf before treating a saved file as successful.
Do not count an expired-token 401 as a passed 403 permission test; sign in again as
the same active account. Compare summary totals and Data against the JSON report
using identical filters; avoid concurrent source edits during this comparison.

## Automated and visual evidence - 2026-10-15

Formatting, module verification, go vet, server build, architecture checks and
all 80 test packages passed. All 15 required real PostgreSQL integration packages
passed without skips on disposable schema 24. Statement coverage: application
4994/5844 (85.5%), report 391/408 (95.8%), PDF adapter 183/213 (85.9%).
The real 1000-row PDF export plus independent decoding took about 2.66s locally;
1001 rows returned HTTP 413. The existing 5000-row Excel export also passed. These
measurements are local test evidence, not a production latency guarantee.
After shared files were renamed, report/adapter/architecture checks passed again;
coverage locations were updated only for the unchanged renamed production files.

Synthetic PDFs were rendered with Poppler and checked visually for all four
report types, including Summary, first Data and final long-record pages. A second
reader (pypdf) confirmed complete long-text tail markers and no replacement
characters. Manual API verification was reported successful by the developer. GET-body
rejection remains covered by automated tests where Windows PowerShell cannot
send that request. Native Sinhala/Tamil shaping and remote CI are not claimed
by this evidence.
