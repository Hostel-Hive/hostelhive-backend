DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='hostelhive' AND table_name='announcements' AND column_name='faculty') THEN RAISE EXCEPTION 'Faculty column remains after rollback'; END IF;
 IF EXISTS(SELECT 1 FROM public.faculty_announcement_upgrade_evidence b LEFT JOIN hostelhive.announcements a ON a.announcement_id::text=b.id WHERE b.kind='announcement' AND to_jsonb(a) IS DISTINCT FROM b.data) THEN RAISE EXCEPTION 'Rollback changed legacy broadcast'; END IF;
END $$;
