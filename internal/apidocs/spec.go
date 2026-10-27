// Package apidocs describes the implemented HTTP API without changing its handlers.
package apidocs

import (
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type object = map[string]any
type document struct {
	paths, schemas object
	tags           []object
}
type operation struct{ data, responses object }

const admin = "admin"
const staffReaders = "admin,warden"
const student = "student"
const allAccounts = "admin,warden,sub_warden,security_staff,student"
const complaintReaders = "admin,warden,sub_warden,student"
const inboxReaders = "admin,warden,sub_warden,student"

// JSON returns deterministic OpenAPI 3.0.3 JSON. Transport models are reflected
// from the real Go types; constraints and operation policies remain explicit.
func JSON() []byte {
	d := &document{paths: object{}, schemas: object{}}
	for _, tag := range []struct{ name, module, description string }{
		{"System", "app", "Unauthenticated liveness and PostgreSQL readiness probes."},
		{"Identity", "identity", "Accounts, roles, activation, approved-email invitations and verified initial setup. Firebase owns login/passwords; there are no Go /auth/login or password-taking Student endpoints."},
		{"Students", "student", "Staff-managed Student/guardian profiles, CSV, private photos, QR and Student-owned profile reads. Faculty is implemented; department is not."},
		{"Staff", "staff", "Admin-managed staff profiles and current staff member's name."},
		{"Inventory", "inventory", "Blocks, rooms and beds. Admin writes; Admin/Warden reads."},
		{"Allocation", "allocation", "Bed assignment, transfer, revocation and permanent accommodation history."},
		{"Attendance", "attendance", "Scanner identities, explicit scan events, presence, incidents and authorized RTDB sessions. Scanner bearer credentials are separate from Firebase tokens."},
		{"Leave", "leave", "Going Home signing-book declarations and historical leave/pass reads. Ordinary leave submission, decisions, cancellation and yearly allowance are retired with410; historical records are preserved. Only recorded scans change physical presence."},
		{"Late Access", "lateaccess", "One movement per request: direction, expected_at and reason. Both IN and OUT require Admin/Warden approval before06:00 or at/after20:00, Asia/Colombo by default. No departure/return interval or destination fields."},
		{"Complaints", "complaint", "Maintenance/disciplinary complaints, versioned updates and private photos. Student-owned; Sub-Warden limited to maintenance. Title and severity are not implemented."},
		{"Announcements", "announcement", "Admin/Warden all-Student or faculty publications, scoped Student feeds and historical delivery/read summaries."},
		{"Notifications", "notification", "Owned inbox, unread state and FCM device registration. Responses contain title/source references, not message previews or a mark-all-read operation."},
		{"Reports", "report", "Read-only attendance, daily, leave and complaint reports, Excel and PDF exports. Admin/Warden only."},
	} {
		d.tags = append(d.tags, object{"name": tag.name, "description": tag.description, "x-module": tag.module})
	}
	d.models()
	d.systemAndIdentity()
	d.studentsAndStaff()
	d.inventoryAndAllocation()
	d.attendance()
	d.requestsAndInbox()
	d.reports()
	d.applyDetails()
	v := object{
		"openapi": "3.0.3",
		"info":    object{"title": "HostelHive Backend API", "version": "1.0.0", "description": "Complete implemented backend contract. Business paths include /api/v1; do not append that prefix to the server URL. Select the API origin hosting these docs. Authorize with a current Firebase ID token (paste token only, without Bearer); Firebase SDK/email setup is separate. Role and active-account state are reloaded server-side. Pending invitation setup permits only GET /api/v1/me and POST /api/v1/me/setup/complete. Scanner routes use ScannerBearer instead. Use dedicated test records: Try it out performs real writes and may queue invitations/notifications. Example UUIDs are fictional: replace resource IDs and generate a NEW request_id per intended operation, retaining it and the identical body for retries. Unsupported methods return405; Go GET routes also accept HEAD. Unless explicitly listed, query parameters are unsupported; strict endpoints reject duplicate/unknown parameters and nonempty read bodies. Nullable/omitted fields are distinct. Pagination and ownership rules are described per endpoint. Future/unimplemented mobile fields are not accepted."},
		"servers": []object{{"url": "/", "description": "Same backend origin as Swagger UI (recommended)"}},
		"tags":    d.tags, "paths": d.paths,
		"security": []object{{"FirebaseBearer": []string{}}},
		"components": object{"schemas": d.schemas, "securitySchemes": object{
			"FirebaseBearer": object{"type": "http", "scheme": "bearer", "bearerFormat": "Firebase ID token", "description": "Current Firebase Authentication ID token; not the web API key, password, refresh token or service-account JSON. Server checks local account/role/setup state."},
			"ScannerBearer":  object{"type": "http", "scheme": "bearer", "bearerFormat": "Scanner credential", "description": "Credential returned once when Admin registers or rotates a scanner. Authorizes only /device/check and /device/scans; never use a Firebase token here."},
		}},
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(data, '\n')
}

func ref(name string) object                 { return object{"$ref": "#/components/schemas/" + name} }
func media(schema object) object             { return object{"schema": schema} }
func (d *document) model(name string, v any) { d.schemas[name] = schema(reflect.TypeOf(v)) }
func schema(t reflect.Type) object {
	if t.Kind() == reflect.Pointer {
		v := schema(t.Elem())
		v["nullable"] = true
		return v
	}
	if t == reflect.TypeOf(time.Time{}) {
		return object{"type": "string", "format": "date-time", "example": "2099-01-15T09:00:00+05:30"}
	}
	switch t.Kind() {
	case reflect.String:
		return object{"type": "string"}
	case reflect.Bool:
		return object{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint64:
		return object{"type": "integer", "format": "int64"}
	case reflect.Float32, reflect.Float64:
		return object{"type": "number"}
	case reflect.Slice, reflect.Array:
		return object{"type": "array", "items": schema(t.Elem())}
	case reflect.Struct:
		props := object{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")
			if !f.IsExported() || tag[0] == "-" {
				continue
			}
			if f.Anonymous && tag[0] == "" {
				embedded := schema(f.Type)
				for k, v := range embedded["properties"].(object) {
					props[k] = v
				}
				if names, ok := embedded["required"].([]string); ok {
					required = append(required, names...)
				}
				continue
			}
			if tag[0] == "" {
				panic("undocumented transport field: " + t.String() + "." + f.Name)
			}
			v := schema(f.Type)
			fieldHints(tag[0], v)
			props[tag[0]] = v
			if !strings.Contains(f.Tag.Get("json"), ",omitempty") {
				required = append(required, tag[0])
			}
		}
		sort.Strings(required)
		v := object{"type": "object", "properties": props}
		if len(required) > 0 {
			v["required"] = required
		}
		return v
	default:
		panic("unsupported transport type: " + t.String())
	}
}
func fieldHints(name string, v object) {
	if v["type"] == "string" && (strings.HasSuffix(name, "_id") || strings.HasSuffix(name, "_by")) && name != "firebase_uid" && name != "movement_id" {
		v["format"] = "uuid"
		v["example"] = "11111111-1111-4111-8111-111111111111"
	}
	switch name {
	case "email":
		v["format"] = "email"
		v["example"] = "example-student@stu.kln.ac.lk"
	case "contact_phone":
		v["example"] = "0771234567"
	case "index_no":
		v["example"] = "SE-EXAMPLE-210"
	case "full_name":
		v["example"] = "Example Student"
	case "faculty":
		v["example"] = "Computing"
	case "relationship":
		v["example"] = "Parent"
	case "year":
		v["example"] = 2
	case "direction":
		v["enum"] = []string{"IN", "OUT"}
		v["example"] = "IN"
	case "request_id":
		v["description"] = "Generate once per intended operation; preserve with identical payload on retry. Not a source/detail ID."
	case "expected_version":
		v["minimum"] = 1
		v["maximum"] = 9007199254740990
		v["example"] = 1
	case "password":
		v["format"] = "password"
		v["writeOnly"] = true
		v["description"] = "Legacy Admin provisioning only. Never use a university password."
	case "token":
		v["writeOnly"] = true
		v["description"] = "FCM registration token from the device; not a Firebase login token."
	}
}
func (d *document) constrain(name, field string, values object) {
	s := d.schemas[name].(object)
	p := s["properties"].(object)[field].(object)
	for k, v := range values {
		p[k] = v
	}
}
func (d *document) optional(name string, fields ...string) {
	s := d.schemas[name].(object)
	names, _ := s["required"].([]string)
	keep := []string{}
	for _, name := range names {
		omit := false
		for _, f := range fields {
			if f == name {
				omit = true
			}
		}
		if !omit {
			keep = append(keep, name)
		}
	}
	s["required"] = keep
}
func (d *document) input(name string, v any) {
	d.model(name, v)
	d.schemas[name].(object)["additionalProperties"] = false
}

var pathParameter = regexp.MustCompile(`\{([^}]+)\}`)

func (d *document) op(tag, method, path, summary, roles, result string, status int) *operation {
	responses := object{}
	o := &operation{data: object{"tags": []string{tag}, "summary": summary, "operationId": strings.ToLower(tag[:1]) + strings.ReplaceAll(tag[1:], " ", "") + strings.NewReplacer("/", "_", "{", "", "}", "", "-", "_").Replace("_"+strings.ToLower(method)+path), "responses": responses, "x-roles": strings.Split(roles, ",")}, responses: responses}
	o.note("Access: " + roles + ". Account must be active; pending setup is blocked except /me and /me/setup/complete. No request body unless one is explicitly shown.")
	o.success(status, result, "Successful operation")
	for _, code := range []int{400, 401, 403, 503} {
		o.fail(code, "")
	}
	if strings.Contains(path, "{") {
		o.fail(404, "not_found (including concealed inaccessible records where applicable)")
	}
	for _, match := range pathParameter.FindAllStringSubmatch(path, -1) {
		o.param("path", match[1], "Resource UUID from a previous response. Fictional example must be replaced.", object{"type": "string", "format": "uuid"}, true)
	}
	paths, ok := d.paths[path].(object)
	if !ok {
		paths = object{}
		d.paths[path] = paths
	}
	key := strings.ToLower(method)
	if paths[key] != nil {
		panic("duplicate documented route " + method + " " + path)
	}
	paths[key] = o.data
	return o
}
func (o *operation) note(s string) *operation {
	old, _ := o.data["description"].(string)
	o.data["description"] = strings.TrimSpace(old + "\n\n" + s)
	return o
}
func (o *operation) success(code int, name, description string) *operation {
	r := object{"description": description}
	if code != 204 {
		r["content"] = object{"application/json": media(ref(name))}
	}
	o.responses[strconv.Itoa(code)] = r
	return o
}
func (o *operation) fail(code int, description string) *operation {
	if description == "" {
		description = map[int]string{400: "invalid_input: invalid fields, identifiers, query or body. Report routes use invalid_report_query.", 401: "unauthorized: missing, expired or invalid credential.", 403: "forbidden or setup_required: current role/account/setup does not permit access.", 404: "not_found: resource missing or inaccessible.", 409: "Conflict with current state, eligibility, idempotency payload or version. Refresh the source before a new action.", 413: "Payload or export exceeds the documented limit.", 415: "Unsupported Content-Type; send the documented raw media type.", 429: "Rate limited or export busy. Respect Retry-After when present.", 503: "service_unavailable or module-specific unavailable code: bounded retry, preserving write idempotency."}[code]
	}
	r := object{"description": description, "content": object{"application/json": object{"schema": ref("Error"), "example": object{"error": map[int]string{400: "invalid_input", 401: "unauthorized", 403: "forbidden", 404: "not_found", 409: "conflict", 413: "payload_too_large", 415: "unsupported_media_type", 429: "rate_limited", 503: "service_unavailable"}[code]}}}}
	if code == 429 {
		r["headers"] = object{"Retry-After": object{"description": "Seconds to wait when supplied.", "schema": object{"type": "integer"}}}
	}
	o.responses[strconv.Itoa(code)] = r
	return o
}
func (o *operation) param(in, name, description string, s object, required bool) *operation {
	p := object{"in": in, "name": name, "description": description, "required": required, "schema": s}
	old, _ := o.data["parameters"].([]object)
	o.data["parameters"] = append(old, p)
	return o
}
func (o *operation) query(name string, s object, description string) *operation {
	return o.param("query", name, description, s, false)
}
func (o *operation) paging(maxOffset int) *operation {
	o.query("limit", object{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}, "Page size.")
	s := object{"type": "integer", "minimum": 0, "default": 0}
	if maxOffset > 0 {
		s["maximum"] = maxOffset
	}
	return o.query("offset", s, "Zero-based offset. Pages are separate snapshots and can shift after writes.")
}
func enumeration(values ...string) object { return object{"type": "string", "enum": values} }
func uuidQuery() object                   { return object{"type": "string", "format": "uuid"} }
func (o *operation) body(name string) *operation {
	o.data["requestBody"] = object{"required": true, "content": object{"application/json": media(ref(name))}, "description": "JSON only; unknown fields are rejected. See schema constraints and operation rules."}
	o.fail(415, "")
	// Older account/profile parsers validate JSON but do not enforce MIME type.
	switch name {
	case "ProvisionAccount", "ChangeRole", "CreateStudent", "StudentDetails", "StaffDetails", "SelfStaffDetails":
		delete(o.responses, "415")
	}
	return o
}
func (o *operation) binary(code int, types ...string) *operation {
	content := object{}
	for _, typ := range types {
		content[typ] = media(object{"type": "string", "format": "binary"})
	}
	o.responses[strconv.Itoa(code)] = object{"description": "Raw bytes, not JSON or a public object-storage URL.", "content": content, "headers": object{"Content-Disposition": object{"schema": object{"type": "string"}}, "Cache-Control": object{"schema": object{"type": "string"}, "description": "Private downloads use no-store."}, "X-Content-Type-Options": object{"schema": object{"type": "string"}, "description": "Private images use nosniff."}}}
	return o
}
func (o *operation) upload() *operation {
	o.data["requestBody"] = object{"required": true, "description": "Raw JPEG or PNG bytes, maximum5MiB; not multipart, JSON or base64. Decoded image dimensions are also validated.", "content": object{"image/jpeg": media(object{"type": "string", "format": "binary", "maxLength": 5242880}), "image/png": media(object{"type": "string", "format": "binary", "maxLength": 5242880})}}
	o.fail(413, "")
	o.fail(415, "")
	return o
}
func (o *operation) csv(header string) *operation {
	o.data["requestBody"] = object{"required": true, "description": "Raw UTF-8 CSV with exact header, maximum2MiB. See this operation's record limit. Guardians is a JSON array in a quoted CSV cell. Download the matching template first; no multipart wrapper.", "content": object{"text/csv": object{"schema": object{"type": "string", "format": "binary"}, "example": header + "\n"}}}
	o.fail(413, "import_too_large")
	o.fail(415, "")
	return o
}
func (o *operation) public() *operation {
	o.data["security"] = []object{}
	o.data["x-roles"] = []string{}
	o.data["description"] = "Public probe; no authentication or request body."
	for _, code := range []string{"400", "401", "403", "503"} {
		delete(o.responses, code)
	}
	return o
}
func (o *operation) scanner() *operation {
	o.data["security"] = []object{{"ScannerBearer": []string{}}}
	o.data["x-roles"] = []string{"scanner"}
	o.data["description"] = "Active scanner credential only. Firebase ID tokens do not authorize this endpoint."
	delete(o.responses, "403")
	return o
}

// Register serves only explicitly embedded documentation assets. API handlers and
// their authentication policies are unchanged. No directory listing or CDN is used.
func Register(mux *http.ServeMux) { registerUI(mux) }
