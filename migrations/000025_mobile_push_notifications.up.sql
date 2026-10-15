BEGIN;
CREATE TABLE hostelhive.push_devices (
 registration_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE CASCADE,
 device_id UUID NOT NULL CHECK(device_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 platform VARCHAR(8) NOT NULL CHECK(platform IN ('android','ios')),
 token VARCHAR(4096) NOT NULL,
 token_hash BYTEA NOT NULL CHECK(octet_length(token_hash)=32),
 active BOOLEAN NOT NULL DEFAULT true,
 generation BIGINT NOT NULL DEFAULT 1 CHECK(generation>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 bound_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(user_id,device_id),
 CHECK((active AND length(token)>=20) OR (NOT active AND token=''))
);
CREATE UNIQUE INDEX push_devices_active_token ON hostelhive.push_devices(token_hash) WHERE active;
CREATE INDEX push_devices_owner ON hostelhive.push_devices(user_id) WHERE active;
CREATE TABLE hostelhive.push_deliveries (
 delivery_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 notification_id UUID NOT NULL REFERENCES hostelhive.notifications(notification_id) ON DELETE CASCADE,
 registration_id UUID NOT NULL REFERENCES hostelhive.push_devices(registration_id) ON DELETE CASCADE,
 generation BIGINT NOT NULL,
 state VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','accepted','failed','skipped','expired')),
 attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 8),
 next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 expires_at TIMESTAMPTZ NOT NULL,
 provider_message_id VARCHAR(512),
 accepted_at TIMESTAMPTZ,
 last_code VARCHAR(40),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(notification_id,registration_id),
 CHECK((state='accepted')=(accepted_at IS NOT NULL))
);
CREATE INDEX push_deliveries_due ON hostelhive.push_deliveries(next_attempt_at,delivery_id) WHERE state='pending';
-- Local intent commits with inbox creation; FCM is never called in that transaction.
-- Existing inbox rows are NOT backfilled. Newly registered/reactivated devices
-- cannot receive events that predate their current token binding.
CREATE FUNCTION hostelhive.enqueue_mobile_push() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE e hostelhive.notification_events%ROWTYPE;
BEGIN
 SELECT * INTO STRICT e FROM hostelhive.notification_events WHERE event_id=NEW.event_id;
 IF e.kind NOT IN ('leave_decided','late_access_decided','attendance_incident') THEN RETURN NEW; END IF;
 INSERT INTO hostelhive.push_deliveries(notification_id,registration_id,generation,expires_at)
 SELECT NEW.notification_id,d.registration_id,d.generation,e.occurred_at+interval '1 hour'
 FROM hostelhive.push_devices d WHERE d.user_id=NEW.recipient_id AND d.active
 AND d.bound_at<=e.occurred_at AND e.occurred_at+interval '1 hour'>clock_timestamp()
 ON CONFLICT(notification_id,registration_id) DO NOTHING;
 RETURN NEW;
END;
$$;
CREATE TRIGGER mobile_push_intent AFTER INSERT ON hostelhive.notifications
 FOR EACH ROW EXECUTE FUNCTION hostelhive.enqueue_mobile_push();
COMMIT;
