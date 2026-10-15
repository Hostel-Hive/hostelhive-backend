package dto

// Export queries intentionally have no pagination: one bounded snapshot includes
// every matching row, or fails explicitly when the export limit is exceeded.
type ExportQuery struct{ From, To, StudentID, Direction, Category, Status string }
