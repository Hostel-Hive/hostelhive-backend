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
}
