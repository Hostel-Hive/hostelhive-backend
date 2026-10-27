package apidocs

func (d *document) reports() {
	for _, report := range []struct{ path, name, result string }{
		{"attendance", "attendance movement", "MovementReport"},
		{"attendance/daily", "daily movement statistics", "DailyReport"},
		{"leaves", "leave request", "LeaveReport"},
		{"complaints", "complaint resolution", "ComplaintReport"},
	} {
		for _, format := range []string{"json", "xlsx", "pdf"} {
			path := "/api/v1/reports/" + report.path
			summary := "Get " + report.name + " report"
			if format == "xlsx" {
				path += "/export"
				summary = "Download " + report.name + " Excel report"
			}
			if format == "pdf" {
				path += "/export/pdf"
				summary = "Download " + report.name + " PDF report"
			}
			o := d.op("Reports", "GET", path, summary, staffReaders, report.result, 200)
			o.responses["400"].(object)["content"].(object)["application/json"].(object)["example"] = object{"error": "invalid_report_query"}
			o.param("query", "from", "Required inclusive local first submission/movement date, YYYY-MM-DD.", object{"type": "string", "format": "date", "example": "2026-10-01"}, true)
			o.param("query", "to", "Required inclusive local last date; maximum366 calendar days and year0001..9998.", object{"type": "string", "format": "date", "example": "2026-10-20"}, true)
			o.query("student_id", uuidQuery(), "Optional Student profile UUID; authorized archived/inactive history retained.")
			if report.path == "attendance" || report.path == "attendance/daily" {
				o.query("direction", enumeration("IN", "OUT"), "Optional recorded movement direction.")
			}
			if report.path == "leaves" {
				o.query("status", enumeration("pending", "approved", "rejected", "cancelled"), "Current request status; date filter applies to submission, not travel interval.")
			}
			if report.path == "complaints" {
				o.query("category", enumeration("maintenance", "disciplinary"), "Complaint category.")
				o.query("status", enumeration("pending", "in_progress", "resolved"), "Current complaint status; date filter applies to submission.")
			}
			o.note("Dates use the configured hostel timezone (default Asia/Colombo). from/to are inclusive local dates; SQL uses an exclusive next-day boundary. Totals cover the FULL filtered period, not just a page. Only recorded attendance is counted; suppressed scans do not inflate statistics. Reads never change requests, attendance, permissions or notification read state. Invalid/missing/duplicate/unsupported filters or a GET body return400 invalid_report_query; unavailable returns503 service_unavailable.")
			if format == "json" {
				o.paging(1000000)
			} else {
				o.fail(413, "report_export_too_large: exceeds format limits; narrow dates/filters.").fail(429, "report_export_busy: retry later; export concurrency is bounded.")
				if format == "xlsx" {
					o.binary(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet").note("All matching rows, maximum5000, no limit/offset accepted. Summary and Data sheets; user strings remain literal cells.")
				} else {
					o.binary(200, "application/pdf").note("All matching rows, maximum1000 rows,200 pages and16MiB. No limit/offset accepted. Printable layout; documented native Sinhala/Tamil shaping limitations apply.")
				}
			}
		}
	}
}
