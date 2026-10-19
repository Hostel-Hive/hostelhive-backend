INSERT INTO hostelhive.users(user_id,firebase_uid,email,role,is_active) VALUES
 ('95000000-0000-4000-8000-000000000003','migration95-pending','migration95-pending@stu.kln.ac.lk','student',true);
INSERT INTO hostelhive.students(student_id,user_id,index_no,full_name,faculty,year,contact_phone)
 VALUES('95000000-0000-4000-8000-000000000013','95000000-0000-4000-8000-000000000003','MIGRATION-95-PENDING','Pending Student','Science',1,'0771234567');
INSERT INTO hostelhive.student_invitations(invitation_id,request_id,user_id,student_id,email,created_by,request_ip,payload_hash)
 VALUES('95000000-0000-4000-8000-000000000023','95000000-0000-4000-8000-000000000033','95000000-0000-4000-8000-000000000003','95000000-0000-4000-8000-000000000013','migration95-pending@stu.kln.ac.lk','95000000-0000-4000-8000-000000000002','127.0.0.1',decode(repeat('00',32),'hex'));
