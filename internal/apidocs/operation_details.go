package apidocs

// applyDetails records exceptional response contracts which cannot be inferred
// from DTOs or a route's path. Keep legacy parsing behaviour explicit.
func (d *document) applyDetails() {
	find := func(method, path string) *operation {
		data := d.paths[path].(object)[method].(object)
		return &operation{data: data, responses: data["responses"].(object)}
	}
	for _, path := range []string{"/api/v1/me/student-profile", "/api/v1/me/student-profile/image", "/api/v1/me/qr", "/api/v1/me/attendance/events", "/api/v1/attendance/presence", "/api/v1/attendance/events"} {
		find("get", path).fail(404, "not_found: missing/inaccessible profile, image or source as applicable.")
	}
	for _, path := range []string{"/api/v1/leaves", "/api/v1/leaves/{leaveID}", "/api/v1/late-access-requests", "/api/v1/late-access-requests/{lateAccessID}", "/api/v1/complaints", "/api/v1/complaints/{complaintID}"} {
		find("get", path).fail(409, "active_student_required or a transactional conflict where applicable.")
	}
	find("get", "/api/v1/me/qr").fail(409, "active_student_account_required.")
	find("get", "/api/v1/students/{studentID}/qr").fail(409, "active_student_account_required.").note("The opaque QR identifier is issued lazily on first authorized retrieval; it never changes presence or authorizes a scan by itself.")
	scan := find("post", "/api/v1/device/scans").fail(409, "scanner_direction_required or event_id_conflict.")
	scan.fail(422, "active_student_qr_required: decoded QR must belong to an eligible active Student.")
	scan.responses["422"].(object)["content"].(object)["application/json"].(object)["example"] = object{"error": "active_student_qr_required"}
	find("patch", "/api/v1/users/{userID}/role").note("Maximum1KiB JSON body; invalid/oversized JSON returns400.")
	find("put", "/api/v1/staff/{userID}").note("Maximum8KiB JSON body; invalid/oversized JSON returns400.")
	find("put", "/api/v1/staff/me").note("Maximum8KiB JSON body; invalid/oversized JSON returns400.")
	find("post", "/api/v1/late-access-requests").note("Policy #118: permission is needed for BOTH IN and OUT before opening or at/after closing (defaults06:00 and20:00, Asia/Colombo). Normal-hour requests are rejected. Overnight OUT after closing is supported; this remains one movement, not a trip interval.")
	find("post", "/api/v1/going-home").note("Digital signing-book entry: the app displays 'I am going home now'. Identity and declaration time are recorded by the backend. No destination, expected return, leave approval or allowance. Never substitutes for early/late movement approval.")
	find("get", "/api/v1/me/passes").note("Leave items are historical and read-only under policy #118; do not offer leave cancellation or decision actions. Late-access requests remain active and may be cancelled while pending.")
	for _, route := range []struct{ method, path string }{
		{"post", "/api/v1/leaves"},
		{"post", "/api/v1/leaves/{leaveID}/decision"},
		{"post", "/api/v1/leaves/{leaveID}/cancel"},
		{"get", "/api/v1/me/leave-allowance"},
	} {
		op := find(route.method, route.path)
		op.data["deprecated"] = true
		op.data["summary"] = "Retired ordinary leave workflow (410 Gone)"
		op.data["description"] = "Retired under hostel policy #118. Existing authentication and role checks still apply. Authorized callers receive 410 leave_workflow_retired without source reads or writes, even on exact retries. No approval or yearly allowance applies to ordinary trips. Use Going Home as an optional signing-book declaration and late-access requests for restricted-hour movements. Historical leave list/detail, reports, inbox and combined pass history remain available. No migration rewrites historical decisions."
		delete(op.data, "requestBody")
		for code := range op.responses {
			if code != "401" && code != "403" && code != "503" {
				delete(op.responses, code)
			}
		}
		if route.method == "get" {
			delete(op.data, "parameters")
		}
		op.fail(410, "leave_workflow_retired: ordinary leave approvals and allowance are retired.")
		op.responses["410"].(object)["content"].(object)["application/json"].(object)["example"] = object{"error": "leave_workflow_retired"}
	}
}
