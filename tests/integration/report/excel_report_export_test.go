package report_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/handler"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/platform/spreadsheet"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExcelReportExports(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL migrated to24")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := "r85-" + uuid.NewString()[:10] + "-"
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	scalar := func(sql string, args ...any) string {
		t.Helper()
		var v string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	user := func(name, role string) string {
		return scalar(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+name, prefix+name+"@example.invalid", role)
	}
	admin := user("admin", "admin")
	user("warden", "warden")
	user("sub", "sub_warden")
	user("security", "security_staff")
	user("other-student", "student")
	inactive := user("inactive", "admin")
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, inactive)
	owner := user("owner", "student")
	sid := scalar(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Excel report Student','Science',1,'0771234567') RETURNING student_id::text`, owner, prefix+"student")
	dev := scalar(`INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,created_by,updated_by) VALUES(gen_random_uuid(),$1,decode(repeat('ab',32),'hex'),$2,$2) RETURNING device_id::text`, prefix+"device", admin)
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 20*time.Second)
		defer c()
		for _, sql := range []string{
			`UPDATE hostelhive.late_access_requests SET consumed_at=NULL,consumed_device_id=NULL,consumed_event_id=NULL WHERE student_id=$1`,
			`DELETE FROM hostelhive.student_presence WHERE student_id=$1`,
			`DELETE FROM hostelhive.attendance_events WHERE student_id=$1`,
			`DELETE FROM hostelhive.late_access_requests WHERE student_id=$1`,
			`DELETE FROM hostelhive.complaint_updates WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id=$1)`,
			`DELETE FROM hostelhive.complaint_images WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id=$1)`,
			`DELETE FROM hostelhive.complaints WHERE student_id=$1`,
			`DELETE FROM hostelhive.leave_requests WHERE student_id=$1`,
			`DELETE FROM hostelhive.students WHERE student_id=$1`,
		} {
			if _, err := pool.Exec(clean, sql, sid); err != nil {
				t.Error(err)
			}
		}
		if _, err := pool.Exec(clean, `DELETE FROM hostelhive.scanner_devices WHERE device_id=$1`, dev); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(clean, `DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`, prefix+"%"); err != nil {
			t.Error(err)
		}
	}()
	// More than a JSON maximum page: every exported record must be included once.
	exec(`INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at,access_result)
 SELECT gen_random_uuid(),$1,$2,decode(repeat('cd',32),'hex'),CASE WHEN i%2=1 THEN 'IN' ELSE 'OUT' END,'recorded',
 '2046-06-10T00:00:00Z'::timestamptz+i*interval '1 second',CASE WHEN i=1 THEN NULL ELSE 'not_required' END FROM generate_series(1,103) i`, dev, sid)
	exec(`INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at,access_result)
 VALUES(gen_random_uuid(),$1,$2,decode(repeat('cd',32),'hex'),'IN','cooldown','2046-06-10T01:00:00Z','not_evaluated')`, dev, sid)
	firstEvent := scalar(`SELECT event_id::text FROM hostelhive.attendance_events WHERE device_id=$1 AND outcome='recorded' ORDER BY occurred_at LIMIT 1`, dev)
	exec(`INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id) VALUES($1,'IN','2046-06-10T00:00:01Z',$2,$3)`, sid, dev, firstEvent)
	permitEvent := scalar(`SELECT event_id::text FROM hostelhive.attendance_events WHERE device_id=$1 AND direction='OUT' ORDER BY occurred_at LIMIT 1`, dev)
	permission := scalar(`INSERT INTO hostelhive.late_access_requests(request_id,student_id,submitted_by,direction,expected_at,reason) VALUES(gen_random_uuid(),$1,$2,'OUT','2046-06-10T00:00:02Z','Export fixture') RETURNING late_access_id::text`, sid, owner)
	exec(`UPDATE hostelhive.late_access_requests SET status='approved',decided_at='2046-06-09T12:00:00Z',decided_by=$2 WHERE late_access_id=$1`, permission, admin)
	exec(`UPDATE hostelhive.attendance_events SET access_result='approved',late_access_id=$3 WHERE device_id=$1 AND event_id=$2`, dev, permitEvent, permission)
	exec(`UPDATE hostelhive.late_access_requests SET consumed_at='2046-06-10T00:00:02Z',consumed_device_id=$2,consumed_event_id=$3 WHERE late_access_id=$1`, permission, dev, permitEvent)
	incidentEvent := scalar(`SELECT event_id::text FROM hostelhive.attendance_events WHERE device_id=$1 AND direction='IN' AND access_result IS NOT NULL ORDER BY occurred_at LIMIT 1`, dev)
	exec(`UPDATE hostelhive.attendance_events SET access_result='unapproved' WHERE device_id=$1 AND event_id=$2`, dev, incidentEvent)
	incident := scalar(`INSERT INTO hostelhive.attendance_incidents(device_id,event_id,student_id,direction,incident_type,occurred_at,hostel_timezone,exit_opening_minute,entry_deadline_minute,permission_window_seconds)
 VALUES($1,$2,$3,'IN','late_entry','2046-06-10T00:00:03Z','Asia/Colombo',360,1200,1800) RETURNING incident_id::text`, dev, incidentEvent, sid)
	reason := "=HYPERLINK(\"https://example.invalid\",\"leave\")\nසිංහල தமிழ்"
	exec(`INSERT INTO hostelhive.leave_requests(request_id,student_id,submitted_by,starts_at,ends_at,reason,created_at)
 SELECT gen_random_uuid(),$1,$2,'2047-01-01T00:00:00Z'::timestamptz+i*interval '2 days','2047-01-02T00:00:00Z'::timestamptz+i*interval '2 days',$3,
 '2046-06-10T00:00:00Z'::timestamptz+i*interval '1 second' FROM generate_series(1,103) i`, sid, owner, reason)
	exec(`UPDATE hostelhive.leave_requests SET status='approved',decided_at='2046-11-01T00:00:00Z',decided_by=$2,decision_note='+SUM(1,2)' WHERE leave_id IN(SELECT leave_id FROM hostelhive.leave_requests WHERE student_id=$1 ORDER BY leave_id LIMIT 34)`, sid, admin)
	exec(`UPDATE hostelhive.leave_requests SET status='rejected',decided_at='2046-11-01T00:00:00Z',decided_by=$2,decision_note='@rejected' WHERE leave_id IN(SELECT leave_id FROM hostelhive.leave_requests WHERE student_id=$1 AND status='pending' ORDER BY leave_id LIMIT 34)`, sid, admin)
	description := "=HYPERLINK(\"https://example.invalid\",\"complaint\")\nPrivate text"
	exec(`INSERT INTO hostelhive.complaints(request_id,student_id,submitted_by,category,description,created_at,updated_at)
 SELECT gen_random_uuid(),$1,$2,CASE WHEN i%2=0 THEN 'maintenance' ELSE 'disciplinary' END,$3,
 '2046-06-10T00:00:00Z'::timestamptz+i*interval '1 second','2046-06-10T00:00:00Z'::timestamptz+i*interval '1 second' FROM generate_series(1,103) i`, sid, owner, description)
	resolved := scalar(`SELECT complaint_id::text FROM hostelhive.complaints WHERE student_id=$1 AND category='disciplinary' ORDER BY complaint_id LIMIT 1`, sid)
	reopened := scalar(`SELECT complaint_id::text FROM hostelhive.complaints WHERE student_id=$1 AND category='maintenance' ORDER BY complaint_id LIMIT 1`, sid)
	exec(`UPDATE hostelhive.complaints SET status=CASE WHEN category='maintenance' THEN 'in_progress' ELSE 'resolved' END WHERE student_id=$1 AND complaint_id NOT IN($2,$3)`, sid, resolved, reopened)
	change := func(id, status, note string) {
		exec(`WITH previous AS(SELECT status,version FROM hostelhive.complaints WHERE complaint_id=$1), changed AS(UPDATE hostelhive.complaints SET status=$3,version=version+1,updated_at='2046-11-01T00:00:00Z' WHERE complaint_id=$1 RETURNING complaint_id)
 INSERT INTO hostelhive.complaint_updates(complaint_id,request_id,actor_id,action,previous_status,new_status,text,previous_version,version,occurred_at)
 SELECT c.complaint_id,gen_random_uuid(),$2,'status',p.status,$3,$4,p.version,p.version+1,'2046-11-01T00:00:00Z' FROM changed c CROSS JOIN previous p`, id, admin, status, note)
	}
	change(resolved, "resolved", "Old resolution")
	change(resolved, "in_progress", "Reopened")
	change(resolved, "resolved", "-Latest recorded resolution")
	change(reopened, "resolved", "Historical resolution")
	change(reopened, "in_progress", "Reopened; no current resolution")
	privateKey := "complaint-images/" + resolved + "/secret"
	exec(`INSERT INTO hostelhive.complaint_images(complaint_id,image_id,object_key,payload_hash,content_type,size_bytes,width,height,state,uploaded_by,uploaded_at,attached_version,cleanup_at)
 VALUES($1,gen_random_uuid(),$2,decode(repeat('aa',32),'hex'),'image/png',1,1,1,'attached',$3,'2046-06-10T00:00:00Z',1,NULL)`, resolved, privateKey, owner)
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, sid, admin)
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, owner)
	exec(`UPDATE hostelhive.scanner_devices SET is_active=false WHERE device_id=$1`, dev)
	loc, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}
	a, l, c := repo.NewAttendance(pool), repo.NewLeaves(pool), repo.NewComplaints(pool)
	exports := svc.NewExports(a, l, c, spreadsheet.NewReportWorkbooks(), loc)
	mux := http.NewServeMux()
	auth := m.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), time.Second)
	report.RegisterExports(h.NewExports(exports, 30*time.Second), mux, auth)
	report.RegisterAttendance(h.NewAttendance(svc.NewAttendance(a, loc), 3*time.Second), mux, auth)
	report.RegisterLeaves(h.NewLeaves(svc.NewLeaves(l, loc), 3*time.Second), mux, auth)
	report.RegisterComplaints(h.NewComplaints(svc.NewComplaints(c, loc), 3*time.Second), mux, auth)
	call := func(path, role, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", path, strings.NewReader(body))
		if role != "" {
			r.Header.Set("Authorization", "Bearer "+prefix+role)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s as %s: %d %s", path, role, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cache header")
		}
		if want != 200 && (w.Header().Get("Content-Disposition") != "" || w.Header().Get("Content-Type") == d.ExcelContentType) {
			t.Fatal("binary error headers")
		}
		return w
	}
	workbook := func(path, role string) ([][]string, map[string]string) {
		t.Helper()
		w := call(path, role, "", 200)
		if w.Header().Get("Content-Type") != d.ExcelContentType || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Length") != strconv.Itoa(w.Body.Len()) {
			t.Fatal(w.Header())
		}
		media, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
		if err != nil || media != "attachment" || !strings.HasPrefix(params["filename"], "hostelhive-") || !strings.HasSuffix(params["filename"], ".xlsx") {
			t.Fatal(params, err)
		}
		f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		rows, err := f.GetRows("Data")
		if err != nil {
			t.Fatal(err)
		}
		summaryRows, err := f.GetRows("Summary")
		if err != nil {
			t.Fatal(err)
		}
		summary := map[string]string{}
		for _, row := range summaryRows {
			if len(row) >= 2 {
				summary[row[0]] = row[1]
			}
		}
		for _, row := range rows {
			for _, value := range row {
				if value == privateKey || strings.Contains(value, "@example.invalid") {
					t.Fatal("private information in export")
				}
			}
		}
		return rows, summary
	}
	state := func() string {
		return scalar(`SELECT jsonb_build_object(
 'leaves',(SELECT jsonb_agg(to_jsonb(l) ORDER BY leave_id) FROM hostelhive.leave_requests l WHERE student_id=$1),
 'complaints',(SELECT jsonb_agg(to_jsonb(c) ORDER BY complaint_id) FROM hostelhive.complaints c WHERE student_id=$1),
 'updates',(SELECT jsonb_agg(to_jsonb(u) ORDER BY update_id) FROM hostelhive.complaint_updates u WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id=$1)),
 'images',(SELECT jsonb_agg(to_jsonb(i) ORDER BY image_id) FROM hostelhive.complaint_images i WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id=$1)),
 'presence',(SELECT to_jsonb(p) FROM hostelhive.student_presence p WHERE student_id=$1),
 'movements',(SELECT jsonb_agg(to_jsonb(e) ORDER BY event_id) FROM hostelhive.attendance_events e WHERE student_id=$1),
 'permissions',(SELECT jsonb_agg(to_jsonb(p) ORDER BY late_access_id) FROM hostelhive.late_access_requests p WHERE student_id=$1),
 'incidents',(SELECT jsonb_agg(to_jsonb(i) ORDER BY incident_id) FROM hostelhive.attendance_incidents i WHERE student_id=$1),
 'notification_events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY event_id) FROM hostelhive.notification_events e),
 'outbox',(SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id,recipient_id) FROM hostelhive.notification_outbox o),
 'notifications',(SELECT jsonb_agg(to_jsonb(n) ORDER BY notification_id) FROM hostelhive.notifications n))::text`, sid)
	}
	before := state()
	base := "/api/v1/reports/"
	q := "?from=2046-06-10&to=2046-06-10&student_id=" + sid
	value := func(row []string, column int) string {
		if column >= len(row) {
			return ""
		}
		return row[column]
	}
	for _, route := range []string{"attendance", "attendance/daily", "leaves", "complaints"} {
		rows, summary := workbook(base+route+"/export"+q, "admin")
		wantRows := 103
		if route == "attendance/daily" {
			wantRows = 1
		}
		if len(rows) != wantRows+1 || summary["Total Matching Rows"] != strconv.Itoa(wantRows) || summary["Exported Rows"] != strconv.Itoa(wantRows) || summary["Timezone"] != "Asia/Colombo" || summary["Student ID"] != sid || summary["From"] != "2046-06-10" || summary["To"] != "2046-06-10" || summary["Export Row Limit"] != "5000" {
			t.Fatal(route, len(rows), summary)
		}
		page := call(base+route+q+"&limit=1", "admin", "", 200)
		var jsonPage struct {
			Total int64             `json:"total"`
			Items []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(page.Body.Bytes(), &jsonPage); err != nil || jsonPage.Total != int64(wantRows) || len(jsonPage.Items) != 1 {
			t.Fatal(route, jsonPage, err)
		}
		var ids string
		switch route {
		case "attendance":
			ids = scalar(`SELECT string_agg(event_id::text,',' ORDER BY occurred_at DESC,device_id,event_id) FROM hostelhive.attendance_events WHERE device_id=$1 AND outcome='recorded'`, dev)
			if summary["Entries"] != "52" || summary["Exits"] != "51" || summary["Distinct Students"] != "1" {
				t.Fatal(summary)
			}
			legacy, approved, unapproved := false, false, false
			for _, row := range rows[1:] {
				if row[0] == firstEvent {
					legacy = true
					if value(row, 5) != "" {
						t.Fatal("legacy result fabricated", row)
					}
				}
				if row[0] == permitEvent {
					approved = true
					if value(row, 5) != "approved" || value(row, 6) != permission {
						t.Fatal(row)
					}
				}
				if row[0] == incidentEvent {
					unapproved = true
					if value(row, 7) != incident {
						t.Fatal(row)
					}
				}
			}
			if !legacy || !approved || !unapproved {
				t.Fatal("missing movement metadata")
			}
			if rows[1][4] != "2046-06-10T05:31:43+05:30" {
				t.Fatal("timezone or sorting", rows[1])
			}
		case "attendance/daily":
			if rows[1][0] != "2046-06-10" || rows[1][1] != "52" || rows[1][2] != "51" {
				t.Fatal(rows)
			}
		case "leaves":
			ids = scalar(`SELECT string_agg(leave_id::text,',' ORDER BY created_at DESC,leave_id DESC) FROM hostelhive.leave_requests WHERE student_id=$1`, sid)
			if summary["Pending"] != "35" || summary["Approved"] != "34" || summary["Rejected"] != "34" {
				t.Fatal(summary)
			}
			for _, row := range rows[1:] {
				if row[6] != reason {
					t.Fatal("text changed", row)
				}
				if row[7] == "pending" {
					if value(row, 9) != "" || value(row, 10) != "" {
						t.Fatal("pending decision fabricated")
					}
				} else if value(row, 10) != admin {
					t.Fatal("decision actor missing", row)
				}
			}
		case "complaints":
			ids = scalar(`SELECT string_agg(complaint_id::text,',' ORDER BY created_at DESC,complaint_id DESC) FROM hostelhive.complaints WHERE student_id=$1`, sid)
			for _, row := range rows[1:] {
				if row[5] != description {
					t.Fatal("description changed", row)
				}
				if row[0] == resolved {
					if value(row, 10) != "Yes" || value(row, 13) != admin || value(row, 14) != "-Latest recorded resolution" {
						t.Fatal("resolution changed", row)
					}
				} else if value(row, 10) != "No" || value(row, 11) != "" || value(row, 12) != "" {
					t.Fatal("stale/legacy resolution invented", row)
				}
			}
		}
		if ids != "" {
			got := []string{}
			for _, row := range rows[1:] {
				got = append(got, row[0])
			}
			if strings.Join(got, ",") != ids {
				t.Fatal("missing/duplicate/unsorted export records", route)
			}
		}
		wardRows, _ := workbook(base+route+"/export"+q, "warden")
		if len(wardRows) != len(rows) {
			t.Fatal("Warden export differs")
		}
		for _, role := range []string{"other-student", "sub", "security", "inactive"} {
			call(base+route+"/export"+q, role, "", 403)
		}
		call(base+route+"/export"+q, "", "", 401)
		for _, bad := range []string{"", q + "&limit=1", q + "&offset=0", q + "&unknown=1", q + "&from=2046-06-10", q + "&student_id=bad", "?from=2046-01-01&to=2047-01-02"} {
			call(base+route+"/export"+bad, "admin", "", 400)
		}
		call(base+route+"/export"+q, "admin", "{}", 400)
		emptyRows, emptySummary := workbook(base+route+"/export?from=2046-06-10&to=2046-06-10&student_id="+uuid.NewString(), "admin")
		emptyWant := 0
		if route == "attendance/daily" {
			emptyWant = 1
		}
		if len(emptyRows) != emptyWant+1 || emptySummary["Total Matching Rows"] != strconv.Itoa(emptyWant) {
			t.Fatal("empty workbook", route, emptyRows, emptySummary)
		}
	}
	days, daySummary := workbook(base+"attendance/daily/export?from=2046-06-10&to=2046-06-12&student_id="+sid, "admin")
	if len(days) != 4 || days[1][0] != "2046-06-12" || days[1][1] != "0" || daySummary["Distinct Students"] != "1" {
		t.Fatal(days, daySummary)
	}
	for _, tc := range []struct {
		route, filter string
		count         int
	}{{"attendance", "direction=OUT", 51}, {"attendance/daily", "direction=OUT", 1}, {"leaves", "status=approved", 34}, {"complaints", "category=disciplinary&status=resolved", 52}} {
		rows, summary := workbook(base+tc.route+"/export"+q+"&"+tc.filter, "admin")
		if len(rows) != tc.count+1 || summary["Total Matching Rows"] != strconv.Itoa(tc.count) {
			t.Fatal(tc, len(rows), summary)
		}
	}
	for _, tc := range []struct{ route, filter string }{{"attendance", "status=pending"}, {"attendance/daily", "category=maintenance"}, {"leaves", "category=maintenance"}, {"complaints", "direction=IN"}} {
		call(base+tc.route+"/export"+q+"&"+tc.filter, "admin", "", 400)
	}
	exec(`UPDATE hostelhive.users SET role='student' WHERE user_id=$1`, admin)
	for _, kind := range []d.ReportKind{d.MovementReport, d.DailyReport, d.LeaveReport, d.ComplaintReport} {
		if _, err := exports.Export(ctx, prefix+"admin", kind, dto.ExportQuery{From: "2046-06-10", To: "2046-06-10", StudentID: sid}); !errors.Is(err, d.ErrForbidden) {
			t.Fatal("stale authority accepted", kind, err)
		}
	}
	exec(`UPDATE hostelhive.users SET role='admin' WHERE user_id=$1`, admin)
	if state() != before {
		t.Fatal("export mutated source, permission consumption, images or notifications")
	}
	// Allow CI race-instrumentation overhead in this test; production retains its configured timeout.
	exec(`INSERT INTO hostelhive.complaints(request_id,student_id,submitted_by,category,description,created_at,updated_at)
 SELECT gen_random_uuid(),$1,$2,'maintenance',repeat('Export boundary ',100),'2050-01-01T00:00:00Z','2050-01-01T00:00:00Z' FROM generate_series(1,5000)`, sid, owner)
	boundary := base + "complaints/export?from=2050-01-01&to=2050-01-01&student_id=" + sid
	started := time.Now()
	rows, summary := workbook(boundary, "admin")
	t.Logf("5000-row Excel export and decode: %s", time.Since(started))
	if len(rows) != 5001 || summary["Exported Rows"] != "5000" {
		t.Fatal(len(rows), summary)
	}
	exec(`INSERT INTO hostelhive.complaints(request_id,student_id,submitted_by,category,description,created_at,updated_at) VALUES(gen_random_uuid(),$1,$2,'maintenance','Over export limit','2050-01-01T00:00:00Z','2050-01-01T00:00:00Z')`, sid, owner)
	if w := call(boundary, "admin", "", 413); !strings.Contains(w.Body.String(), "report_export_too_large") {
		t.Fatal(w.Body.String())
	}
}
