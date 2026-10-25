package apidocs

import (
	allocation "github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/domain"
	allocationdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/dto"
	announcement "github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/domain"
	announcementdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/dto"
	attendance "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	attendancedto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	complaint "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/domain"
	complaintdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/dto"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	identitydto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
	identityservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
	inventory "github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	inventorydto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/dto"
	late "github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/domain"
	latedto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/dto"
	leave "github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	leavedto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
	notification "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	notificationdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	report "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	staff "github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/domain"
	staffdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/dto"
	studentdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	studentdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
)

func (d *document) models() {
	for name, v := range map[string]any{
		"OwnStudentImageResult": studentdomain.OwnImageResult{},
		"Account":               identity.Account{}, "AccountPage": identity.AccountPage{}, "AccountChange": identity.AccountChange{}, "ProvisionResult": identitydto.Result{},
		"Invitation": identity.StudentInvitation{}, "InvitationPage": identity.InvitationPage{}, "SetupResult": identity.SetupResult{}, "InvitationImportReport": identityservice.InvitationImportReport{},
		"StudentProfile": studentdomain.Profile{}, "StudentPage": studentdomain.Page{}, "OwnStudentProfile": studentdomain.OwnProfile{}, "StudentImportReport": studentdomain.ImportReport{},
		"StaffProfile": staff.Profile{}, "StaffPage": staff.Page{},
		"Block": inventory.Block{}, "Room": inventory.Room{}, "Bed": inventory.Bed{}, "BlockPage": inventory.Page[inventory.Block]{}, "RoomPage": inventory.Page[inventory.Room]{}, "BedPage": inventory.Page[inventory.Bed]{},
		"Allocation": allocation.Allocation{}, "AllocationPage": allocation.Page{},
		"ScannerDevice": attendance.Device{}, "ScannerPage": attendance.DevicePage{}, "IssuedScannerCredential": attendancedto.IssuedCredential{}, "ScannerCheck": attendancedto.DeviceCheck{},
		"ScanEvent": attendance.Event{}, "EventPage": attendance.EventPage{}, "Presence": attendance.Presence{}, "PresencePage": attendance.PresencePage{}, "PresenceSummary": attendance.PresenceSummary{},
		"RealtimeSession": attendance.RealtimeSession{}, "PublicationStatus": attendance.PublicationStatus{}, "Incident": attendance.Incident{}, "IncidentPage": attendance.IncidentPage{}, "StudentMovementPage": attendance.StudentHistoryPage{},
		"LeaveRequest": leave.Request{}, "LeavePage": leave.Page{}, "LeaveAllowance": leave.AllowanceSummary{}, "PassPage": leave.PassPage{}, "GoingHomeDeclaration": leave.GoingHomeDeclaration{},
		"LateAccessRequest": late.Request{}, "LateAccessPage": late.Page{},
		"Complaint": complaint.Complaint{}, "ComplaintPage": complaint.Page{}, "ComplaintUpdate": complaint.Update{}, "ComplaintUpdatePage": complaint.UpdatePage{}, "ComplaintImageResult": complaint.ImageResult{}, "ComplaintImageList": complaint.ImageList{}, "ComplaintImageRemoval": complaint.ImageRemoval{},
		"Announcement": announcement.Announcement{}, "AnnouncementPublication": announcement.Publication{}, "AnnouncementPage": announcement.Page{}, "AnnouncementDeliverySummary": announcement.DeliverySummary{},
		"Notification": notification.Notification{}, "NotificationPage": notification.Page{}, "UnreadCount": notification.Unread{}, "PushDevice": notification.PushDevice{}, "PushDevicePage": notification.PushDevicePage{},
		"MovementReport": report.MovementPage{}, "DailyReport": report.DailyPage{}, "LeaveReport": report.LeavePage{}, "ComplaintReport": report.ComplaintPage{},
	} {
		d.model(name, v)
	}
	d.model("RegistrationResult", struct {
		Invitation identity.StudentInvitation `json:"invitation"`
		Replayed   bool                       `json:"replayed"`
	}{})
	d.model("Probe", struct {
		Status string `json:"status"`
	}{})
	d.model("Error", struct {
		Error string `json:"error"`
	}{})
	for name, v := range map[string]any{
		"ProvisionAccount": identitydto.Input{}, "ChangeRole": identitydto.RoleInput{}, "RegisterStudent": identitydto.StudentInvitationInput{}, "ResendInvitation": identitydto.InvitationResend{},
		"CreateStudent": studentdto.CreateInput{}, "StudentDetails": studentdto.Details{}, "PatchOwnStudentProfile": studentdto.OwnProfilePatch{}, "StaffDetails": staffdto.Details{}, "SelfStaffDetails": staffdto.SelfDetails{},
		"BlockDetails": inventorydto.BlockDetails{}, "CreateRoom": inventorydto.CreateRoom{}, "RoomDetails": inventorydto.RoomDetails{}, "CreateBed": inventorydto.CreateBed{}, "BedDetails": inventorydto.BedDetails{},
		"AssignBed": allocationdto.Assign{}, "TransferBed": allocationdto.Transfer{}, "RegisterScanner": attendancedto.RegisterDevice{}, "SetScannerDirection": attendancedto.SetDirection{}, "RecordScan": attendancedto.Scan{},
		"SubmitLeave": leavedto.Submit{}, "LeaveDecision": leavedto.Decision{}, "GoingHomeInput": leavedto.GoingHomeInput{}, "SubmitLateAccess": latedto.Submit{}, "LateAccessDecision": latedto.Decision{},
		"SubmitComplaint": complaintdto.Submit{}, "ComplaintNote": complaintdto.Change{}, "ComplaintStatus": complaintdto.Change{}, "PublishAnnouncement": announcementdto.Publish{}, "RegisterPushDevice": notificationdto.PushRegistration{},
	} {
		d.input(name, v)
	}
	for _, name := range []string{"RegisterStudent", "CreateStudent", "StudentDetails"} {
		for field, max := range map[string]int{"index_no": 64, "full_name": 200, "faculty": 120} {
			d.constrain(name, field, object{"minLength": 1, "maxLength": max})
		}
		d.constrain(name, "year", object{"minimum": 1, "maximum": 10})
		d.constrain(name, "guardians", object{"minItems": 1, "maxItems": 10})
		items := d.schemas[name].(object)["properties"].(object)["guardians"].(object)["items"].(object)
		items["additionalProperties"] = false
	}
	d.constrain("RegisterStudent", "email", object{"maxLength": 254, "description": "Approved Student email; trimmed/lowercased. Gmail (gmail.com) and university Student email (stu.kln.ac.lk) are allowed by default. STUDENT_EMAIL_DOMAINS can override the exact domain allowlist. Never send a university password."})
	roles := []string{"admin", "warden", "sub_warden", "security_staff", "student"}
	for _, name := range []string{"Account", "ProvisionAccount", "ChangeRole", "StaffProfile"} {
		d.constrain(name, "role", object{"enum": roles})
	}
	d.constrain("ProvisionAccount", "password", object{"minLength": 12, "maxLength": 128, "example": "REPLACE_WITH_NEW_TEST_PASSWORD"})
	for _, name := range []string{"SubmitLeave", "SubmitLateAccess"} {
		d.constrain(name, "reason", object{"minLength": 1, "maxLength": 1000, "example": "Family visit"})
	}
	d.constrain("SubmitLateAccess", "expected_at", object{"example": "2099-01-15T21:00:00+05:30"})
	d.constrain("SubmitLeave", "ends_at", object{"example": "2099-01-16T09:00:00+05:30"})
	for _, name := range []string{"LeaveDecision", "LateAccessDecision"} {
		d.constrain(name, "status", object{"enum": []string{"approved", "rejected"}, "example": "approved"})
		d.constrain(name, "note", object{"maxLength": 1000, "description": "Required and nonblank when rejecting; optional for approval."})
		d.optional(name, "note")
	}
	for _, name := range []string{"LeaveRequest", "LateAccessRequest"} {
		d.constrain(name, "status", object{"enum": []string{"pending", "approved", "rejected", "cancelled"}})
	}
	d.constrain("SubmitComplaint", "category", object{"enum": []string{"maintenance", "disciplinary"}, "example": "maintenance"})
	d.constrain("SubmitComplaint", "description", object{"minLength": 1, "maxLength": 2000, "example": "The bathroom tap is leaking."})
	// Notes and status updates share a Go DTO but have different allowed fields.
	note := d.schemas["ComplaintNote"].(object)
	delete(note["properties"].(object), "status")
	d.optional("ComplaintNote", "status")
	d.constrain("ComplaintNote", "text", object{"minLength": 1, "maxLength": 2000, "example": "The leak is continuing."})
	d.constrain("ComplaintStatus", "status", object{"enum": []string{"in_progress", "resolved"}, "example": "in_progress"})
	d.schemas["ComplaintStatus"].(object)["required"] = []string{"request_id", "expected_version", "status"}
	d.constrain("ComplaintStatus", "text", object{"maxLength": 2000, "description": "Nonblank resolution text is required for resolved; optional for in_progress."})
	d.constrain("PublishAnnouncement", "title", object{"minLength": 1, "maxLength": 160, "example": "Hostel notice"})
	d.constrain("PublishAnnouncement", "content", object{"minLength": 1, "maxLength": 5000, "example": "The common room will close at 8 PM."})
	d.constrain("PublishAnnouncement", "audience", object{"enum": []string{"all_students", "faculty"}, "default": "all_students"})
	d.constrain("PublishAnnouncement", "faculty", object{"maxLength": 120, "example": "Science", "description": "Required and nonblank for audience=faculty; omit for all_students. Matches an existing unarchived Student faculty, ignoring case and surrounding whitespace; stored lowercase. Unknown faculty returns400."})
	d.constrain("Announcement", "faculty", object{"maxLength": 120, "description": "Normalized target faculty; null for all_students."})
	d.constrain("RegisterScanner", "name", object{"minLength": 1, "maxLength": 120, "example": "Test entry scanner"})
	d.constrain("RecordScan", "qr", object{"description": "Exact Student QR payload, obtained by decoding the authenticated QR PNG; never use the Student UUID.", "example": "REPLACE_WITH_DECODED_STUDENT_QR"})
	d.constrain("RegisterPushDevice", "platform", object{"enum": []string{"android", "ios"}, "example": "android"})
	d.constrain("RegisterPushDevice", "token", object{"minLength": 20, "maxLength": 4096, "pattern": "^[A-Za-z0-9:_-]+$"})
	d.constrain("LeaveAllowance", "total_days", object{"enum": []int{18}})
	d.constrain("Incident", "incident_type", enumeration("early_exit", "late_exit", "late_entry"))
	d.constrain("Probe", "status", object{"enum": []string{"ok", "ready", "not_ready"}, "example": "ok"})
	d.schemas["PatchOwnStudentProfile"].(object)["minProperties"] = 1
	d.constrain("PatchOwnStudentProfile", "contact_phone", object{"nullable": false, "minLength": 7, "maxLength": 32, "pattern": "^[+()0-9 -]{7,32}$", "description": "Omit to preserve. Trimmed; at least seven digits required. Null/empty is invalid."})
	d.constrain("PatchOwnStudentProfile", "guardians", object{"nullable": false, "minItems": 1, "maxItems": 10, "description": "Omit to preserve existing guardians and IDs. When supplied, replaces the complete list atomically; use the IDs returned in the response, not old IDs. Null/empty and guardian_id input are invalid."})
	guardianPatch := d.schemas["PatchOwnStudentProfile"].(object)["properties"].(object)["guardians"].(object)["items"].(object)
	guardianPatch["additionalProperties"] = false
	for name, max := range map[string]int{"name": 200, "relationship": 80, "contact_phone": 32} {
		property := guardianPatch["properties"].(object)[name].(object)
		property["minLength"], property["maxLength"] = 1, max
		if name == "contact_phone" {
			property["minLength"] = 7
			property["pattern"] = "^[+()0-9 -]{7,32}$"
		}
	}
	for name, fields := range map[string]map[string]int{
		"BlockDetails": {"name": 120}, "RoomDetails": {"room_no": 32}, "CreateRoom": {"room_no": 32},
		"BedDetails": {"bed_no": 32}, "CreateBed": {"bed_no": 32},
		"StaffDetails": {"full_name": 200, "designation": 120}, "SelfStaffDetails": {"full_name": 200},
	} {
		for field, max := range fields {
			d.constrain(name, field, object{"minLength": 1, "maxLength": max})
		}
	}
}
