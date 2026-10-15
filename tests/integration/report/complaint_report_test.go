package report_test

import (
	"context"
	"encoding/json"
	"errors"
	complaintmodule "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint"
	ch "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/handler"
	cr "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/repository"
	cs "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/service"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/handler"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/service"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestComplaintReports(t *testing.T) {
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
	prefix := "r83-" + uuid.NewString()[:10] + "-"
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
	warden := user("warden", "warden")
	user("sub", "sub_warden")
	user("security", "security_staff")
	inactive := user("inactive", "admin")
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, inactive)
	student := func(name string) (string, string) {
		uid := user(name, "student")
		sid := scalar(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,$3,'Science',1,'0771234567') RETURNING student_id::text`, uid, prefix+name, "Complaint report "+name)
		return sid, uid
	}
	s1, u1 := student("student1")
	s2, u2 := student("student2")
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		for _, sql := range []string{
			`DELETE FROM hostelhive.complaint_updates WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id IN($1,$2))`,
			`DELETE FROM hostelhive.complaint_images WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id IN($1,$2))`,
			`DELETE FROM hostelhive.complaints WHERE student_id IN($1,$2)`,
			`DELETE FROM hostelhive.students WHERE student_id IN($1,$2)`,
		} {
			if _, err := pool.Exec(clean, sql, s1, s2); err != nil {
				t.Error(err)
			}
		}
		if _, err := pool.Exec(clean, `DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`, prefix+"%"); err != nil {
			t.Error(err)
		}
	}()
	complaint := func(sid, uid, created, category string) string {
		id := scalar(`INSERT INTO hostelhive.complaints(request_id,student_id,submitted_by,category,description,created_at,updated_at) VALUES(gen_random_uuid(),$1,$2,$3,'Report fixture description',$4,$4) RETURNING complaint_id::text`, sid, uid, category, created)
		exec(`INSERT INTO hostelhive.complaint_updates(complaint_id,request_id,actor_id,action,new_status,text,previous_version,version,occurred_at) VALUES($1,gen_random_uuid(),$2,'created','pending','Report fixture description',0,1,$3)`, id, uid, created)
		return id
	}
	change := func(id, actor, status, note, at string) string {
		// Status and audit rows match the existing workflow; report dates do not use this time.
		return scalar(`WITH previous AS(SELECT status,version FROM hostelhive.complaints WHERE complaint_id=$1), changed AS(
  UPDATE hostelhive.complaints SET status=$3,version=version+1,updated_at=$5 WHERE complaint_id=$1 RETURNING complaint_id)
  INSERT INTO hostelhive.complaint_updates(complaint_id,request_id,actor_id,action,previous_status,new_status,text,previous_version,version,occurred_at)
  SELECT c.complaint_id,gen_random_uuid(),$2,'status',p.status,$3,$4,p.version,p.version+1,$5 FROM changed c CROSS JOIN previous p RETURNING update_id::text`, id, actor, status, note, at)
	}
	complaint(s1, u1, "2044-03-09T18:29:59Z", "maintenance")
	pending := complaint(s1, u1, "2044-03-09T18:30:00Z", "maintenance")
	resolved := complaint(s1, u1, "2044-03-10T08:00:00Z", "disciplinary")
	change(resolved, admin, "resolved", "First resolution", "2044-11-01T12:00:00Z")
	change(resolved, warden, "in_progress", "Reopened for investigation", "2044-11-02T12:00:00Z")
	resolutionID := change(resolved, warden, "resolved", "Final recorded resolution", "2044-11-03T12:00:00Z")
	reopened := complaint(s2, u2, "2044-03-10T18:29:59.999999Z", "maintenance")
	change(reopened, admin, "resolved", "Old resolution must not be current", "2044-11-01T12:00:00Z")
	change(reopened, warden, "in_progress", "Reopened", "2044-11-02T12:00:00Z")
	legacy := complaint(s2, u2, "2044-03-10T12:00:00Z", "disciplinary")
	// A legacy/manual resolution after a reopen has no matching current audit.
	// Do not reuse its historical resolution from before that reopen.
	change(legacy, admin, "resolved", "Historical legacy resolution", "2044-11-01T12:00:00Z")
	change(legacy, warden, "in_progress", "Legacy reopen", "2044-11-02T12:00:00Z")
	exec(`UPDATE hostelhive.complaints SET status='resolved' WHERE complaint_id=$1`, legacy)
	complaint(s1, u1, "2044-03-10T18:30:00Z", "maintenance")
	imageID := uuid.NewString()
	privateKey := "complaint-images/" + resolved + "/private-fixture"
	exec(`INSERT INTO hostelhive.complaint_images(complaint_id,image_id,object_key,payload_hash,content_type,size_bytes,width,height,state,uploaded_by,uploaded_at,attached_version,cleanup_at)
 VALUES($1,$2,$3,decode(repeat('aa',32),'hex'),'image/png',1,1,1,'attached',$4,'2044-03-10T09:00:00Z',1,NULL)`, resolved, imageID, privateKey, u1)
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, s1, admin)
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id IN($1,$2)`, u1, u2)
	location, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}
	store := repo.NewComplaints(pool)
	service := svc.NewComplaints(store, location)
	mux := http.NewServeMux()
	auth := m.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), time.Second)
	report.RegisterComplaints(h.NewComplaints(service, 3*time.Second), mux, auth)
	report.RegisterLeaves(h.NewLeaves(svc.NewLeaves(repo.NewLeaves(pool), location), 3*time.Second), mux, auth)
	report.RegisterAttendance(h.NewAttendance(svc.NewAttendance(repo.NewAttendance(pool), location), 3*time.Second), mux, auth)
	complaintStore := cr.New(pool)
	complaintmodule.Register(ch.New(cs.New(complaintStore), 3*time.Second), mux, auth)
	complaintmodule.RegisterImages(ch.NewImageAPI(cs.NewImages(complaintStore, nil), 3*time.Second), mux, auth)
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
			t.Fatal("missing no-store")
		}
		return w
	}
	base := "/api/v1/reports/complaints"
	q := "?from=2044-03-10&to=2044-03-10"
	page := func(query, role string) d.ComplaintPage {
		t.Helper()
		var p d.ComplaintPage
		if err := json.Unmarshal(call(base+query, role, "", 200).Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	all := page(q, "admin")
	expected := d.ComplaintTotals{ByStatus: d.ComplaintStatuses{Pending: 1, InProgress: 1, Resolved: 2}, ByCategory: d.ComplaintCategories{Maintenance: 2, Disciplinary: 2}}
	if all.Total != 4 || all.Summary != expected || len(all.Items) != 4 || all.From != "2044-03-10" || all.To != "2044-03-10" || all.Timezone != "Asia/Colombo" || all.Limit != 20 || all.Offset != 0 || all.HasMore {
		t.Fatal(all)
	}
	if all.Items[0].ComplaintID != reopened || all.Items[1].ComplaintID != legacy || all.Items[2].ComplaintID != resolved || all.Items[3].ComplaintID != pending {
		t.Fatal("midnight boundary or sorting", all.Items)
	}
	for _, item := range all.Items {
		if item.Description != "Report fixture description" || item.RequestID == "" || item.SubmittedBy == "" || item.Version < 1 {
			t.Fatal(item)
		}
		if item.ComplaintID == resolved {
			r := item.Resolution
			if r == nil || r.UpdateID != resolutionID || r.ResolvedBy != warden || r.Note != "Final recorded resolution" || !r.ResolvedAt.Equal(time.Date(2044, 11, 3, 12, 0, 0, 0, time.UTC)) || item.Version != 4 {
				t.Fatal("current resolution audit", item)
			}
		} else if item.Resolution != nil {
			t.Fatal("stale or invented resolution", item)
		}
	}
	raw := call(base+q, "admin", "", 200).Body.String()
	for _, secret := range []string{privateKey, imageID, "object_key", "payload_hash", "image_url", "firebase_uid", "@example.invalid"} {
		if strings.Contains(raw, secret) {
			t.Fatal("private source metadata exposed", secret)
		}
	}
	if !strings.Contains(raw, `"resolution":null`) {
		t.Fatal("missing explicit null resolution")
	}
	other := page(q, "warden")
	if other.Total != all.Total || other.Summary != all.Summary {
		t.Fatal(other)
	}
	p := page(q+"&limit=1&offset=1", "admin")
	if p.Total != 4 || p.Summary != all.Summary || len(p.Items) != 1 || !p.HasMore || p.Items[0].ComplaintID != legacy {
		t.Fatal(p)
	}
	p = page(q+"&offset=999", "admin")
	if p.Total != 4 || p.Summary != all.Summary || len(p.Items) != 0 || p.Items == nil || p.HasMore {
		t.Fatal(p)
	}
	for _, category := range []string{"maintenance", "disciplinary"} {
		p = page(q+"&category="+category, "admin")
		if p.Total != 2 || len(p.Items) != 2 || p.Category != category {
			t.Fatal(p)
		}
		for _, item := range p.Items {
			if item.Category != category {
				t.Fatal(item)
			}
		}
		if category == "maintenance" && p.Summary.ByCategory != (d.ComplaintCategories{Maintenance: 2}) {
			t.Fatal(p)
		}
		if category == "disciplinary" && p.Summary.ByCategory != (d.ComplaintCategories{Disciplinary: 2}) {
			t.Fatal(p)
		}
	}
	for _, status := range []string{"pending", "in_progress", "resolved"} {
		p = page(q+"&status="+status, "admin")
		n := int64(1)
		if status == "resolved" {
			n = 2
		}
		if p.Total != n || int64(len(p.Items)) != n || p.Status != status {
			t.Fatal(p)
		}
		for _, item := range p.Items {
			if item.Status != status {
				t.Fatal(item)
			}
		}
		if p.Total != p.Summary.ByStatus.Pending+p.Summary.ByStatus.InProgress+p.Summary.ByStatus.Resolved || p.Total != p.Summary.ByCategory.Maintenance+p.Summary.ByCategory.Disciplinary {
			t.Fatal("filtered summary", p)
		}
	}
	p = page(q+"&student_id="+s1, "admin")
	if p.Total != 2 || p.StudentID != s1 || p.Summary.ByStatus.InProgress != 0 {
		t.Fatal(p)
	}
	p = page(q+"&student_id="+s1+"&category=disciplinary&status=resolved", "admin")
	if p.Total != 1 || len(p.Items) != 1 || p.Items[0].ComplaintID != resolved || p.Summary.ByStatus.Resolved != 1 || p.Summary.ByCategory.Disciplinary != 1 {
		t.Fatal(p)
	}
	for _, suffix := range []string{"&student_id=" + uuid.NewString(), "&student_id=" + s1 + "&category=maintenance&status=resolved"} {
		p = page(q+suffix, "admin")
		if p.Total != 0 || p.Summary != (d.ComplaintTotals{}) || len(p.Items) != 0 || p.Items == nil || p.HasMore {
			t.Fatal(p)
		}
	}
	// Re-resolving must follow audit version order even if a device/database clock moved backward.
	exec(`UPDATE hostelhive.complaint_updates SET occurred_at='2044-11-05T12:00:00Z' WHERE complaint_id=$1 AND version=2`, resolved)
	p = page(q+"&student_id="+s1+"&status=resolved", "admin")
	if p.Items[0].Resolution.UpdateID != resolutionID {
		t.Fatal("used timestamp instead of revision")
	}
	tie1 := complaint(s1, u1, "2045-05-01T10:00:00Z", "maintenance")
	tie2 := complaint(s1, u1, "2045-05-01T10:00:00Z", "disciplinary")
	first, second := tie1, tie2
	if first < second {
		first, second = second, first
	}
	tq := "?from=2045-05-01&to=2045-05-01&student_id=" + s1 + "&limit=1"
	p = page(tq, "admin")
	next := page(tq+"&offset=1", "admin")
	if p.Total != 2 || !p.HasMore || p.Items[0].ComplaintID != first || next.Items[0].ComplaintID != second || next.HasMore {
		t.Fatal(p, next)
	}
	state := func() string {
		return scalar(`SELECT jsonb_build_object(
 'complaints',(SELECT jsonb_agg(to_jsonb(c) ORDER BY complaint_id) FROM hostelhive.complaints c WHERE student_id IN($1,$2)),
 'updates',(SELECT jsonb_agg(to_jsonb(u) ORDER BY update_id) FROM hostelhive.complaint_updates u WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id IN($1,$2))),
 'images',(SELECT jsonb_agg(to_jsonb(i) ORDER BY image_id) FROM hostelhive.complaint_images i WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id IN($1,$2))),
 'presence',(SELECT jsonb_agg(to_jsonb(p) ORDER BY student_id) FROM hostelhive.student_presence p WHERE student_id IN($1,$2)),
 'attendance',(SELECT count(*) FROM hostelhive.attendance_events WHERE student_id IN($1,$2)),
 'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY event_id) FROM hostelhive.notification_events e WHERE complaint_update_id IN(SELECT update_id FROM hostelhive.complaint_updates WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id IN($1,$2)))),
 'outbox',(SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id,recipient_id) FROM hostelhive.notification_outbox o WHERE event_id IN(SELECT event_id FROM hostelhive.notification_events WHERE complaint_update_id IN(SELECT update_id FROM hostelhive.complaint_updates WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id IN($1,$2))))),
 'notifications',(SELECT jsonb_agg(to_jsonb(n) ORDER BY notification_id) FROM hostelhive.notifications n WHERE event_id IN(SELECT event_id FROM hostelhive.notification_events WHERE complaint_update_id IN(SELECT update_id FROM hostelhive.complaint_updates WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id IN($1,$2)))))
 )::text`, s1, s2)
	}
	before := state()
	for i := 0; i < 3; i++ {
		page(q, "admin")
		page(q+"&category=disciplinary", "warden")
	}
	if state() != before {
		t.Fatal("report mutated workflow, images, attendance or notifications")
	}
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, u2)
	for _, role := range []string{"student2", "sub", "security", "inactive"} {
		call(base+q, role, "", 403)
	}
	call(base+q, "", "", 401)
	// Existing complaint and photo category/ownership rules are unaffected by report access.
	call("/api/v1/complaints/"+pending, "sub", "", 200)
	call("/api/v1/complaints/"+resolved, "sub", "", 404)
	call("/api/v1/complaints/"+resolved, "student2", "", 404)
	call("/api/v1/complaints/"+resolved+"/images", "admin", "", 200)
	call("/api/v1/complaints/"+resolved+"/images", "warden", "", 200)
	call("/api/v1/complaints/"+resolved+"/images", "sub", "", 404)
	call("/api/v1/complaints/"+resolved+"/images", "student2", "", 404)
	call("/api/v1/complaints/"+resolved+"/images", "security", "", 403)
	for _, bad := range []string{"", "?from=2044-03-10", "?from=bad&to=2044-03-10", "?from=2044-03-11&to=2044-03-10", "?from=2044-01-01&to=2045-01-02", q + "&status=Resolved", q + "&category=Maintenance", q + "&category=invalid", q + "&direction=IN", q + "&student_id=bad", q + "&limit=0", q + "&limit=101", q + "&offset=-1", q + "&offset=1000001", q + "&limit=x", q + "&offset=x", q + "&category=", q + "&category=maintenance&category=disciplinary", q + "&status=pending&status=resolved", q + "&unknown=1", q + "&bad=%ZZ"} {
		call(base+bad, "admin", "", 400)
	}
	call(base+q, "admin", "{}", 400)
	for _, route := range []string{"leaves", "attendance", "attendance/daily"} {
		call("/api/v1/reports/"+route+q+"&category=maintenance", "admin", "", 400)
	}
	f := dto.ComplaintFilter{ComplaintQuery: dto.ComplaintQuery{From: "2044-03-10", To: "2044-03-10", Limit: 20}, Start: time.Date(2044, 3, 9, 18, 30, 0, 0, time.UTC), End: time.Date(2044, 3, 10, 18, 30, 0, 0, time.UTC), Timezone: "Asia/Colombo"}
	exec(`UPDATE hostelhive.users SET role='student' WHERE user_id=$1`, admin)
	if _, err := store.Records(ctx, prefix+"admin", f); !errors.Is(err, d.ErrForbidden) {
		t.Fatal(err)
	}
	exec(`UPDATE hostelhive.users SET role='admin' WHERE user_id=$1`, admin)
	closed, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	if _, err := repo.NewComplaints(closed).Records(ctx, prefix+"admin", f); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal(err)
	}
}
