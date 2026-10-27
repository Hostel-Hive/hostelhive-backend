package handler

import "net/http"

// RetiredWorkflow is reached only after the route's normal authentication and
// role checks. No legacy request, decision, allowance or notification is changed.
func RetiredWorkflow(w http.ResponseWriter, r *http.Request) {
	reply(w, http.StatusGone, map[string]string{"error": "leave_workflow_retired"})
}
