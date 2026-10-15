BEGIN;
-- Bound global recorded-movement date-range reports without indexing suppressed scans.
CREATE INDEX attendance_events_recorded_report
ON hostelhive.attendance_events(occurred_at DESC,device_id,event_id)
WHERE outcome='recorded';
COMMIT;
