package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/config"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation"
	allocationhandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/handler"
	allocationrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/repository"
	allocationservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement"
	announcementhandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/handler"
	announcementrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/repository"
	announcementservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance"
	attendancehandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/handler"
	attendancerepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/repository"
	attendanceservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint"
	complainthandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/handler"
	complaintrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/repository"
	complaintservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/service"
	identityhandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory"
	ihandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/handler"
	irepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/repository"
	iservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave"
	leavehandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/handler"
	leaverepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/repository"
	leaveservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification"
	notificationhandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/handler"
	notificationrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/repository"
	notificationservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff"
	staffhandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/handler"
	staffrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/repository"
	staffservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student"
	shandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	srepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/repository"
	sservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	platformfirebase "github.com/Hostel-Hive/hostelhive-backend/internal/platform/firebase"
	"github.com/Hostel-Hive/hostelhive-backend/internal/platform/objectstorage"
	database "github.com/Hostel-Hive/hostelhive-backend/internal/platform/postgres"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/workers"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := database.Open(ctx, cfg.DatabaseURL, cfg.DatabaseCheckTimeout)
	if err != nil {
		return err
	}
	defer pool.Close()
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on HTTP_ADDR: %w", err)
	}
	defer listener.Close()
	firebaseClient, err := platformfirebase.NewFirebaseClient(ctx, cfg.FirebaseProjectID)
	if err != nil {
		return err
	}
	protected := authentication.Middleware(platformfirebase.NewVerifier(firebaseClient), repository.NewPostgresAccounts(pool), cfg.AuthenticationTimeout)
	userService := service.NewProvisioningService(repository.NewPostgresRepository(pool), platformfirebase.NewFirebaseIdentities(firebaseClient))
	managementStore := repository.NewStore(pool)
	revoker := platformfirebase.NewFirebaseRevoker(firebaseClient)
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		workers.RunUserRevocations(workerCtx, managementStore, revoker, cfg.AccountManagementTimeout, 30*time.Second)
	}()
	defer func() { stopWorker(); <-workerDone }()
	handler := newAPIHandler(pool.Ping, cfg.DatabaseCheckTimeout, protected, identityhandler.Handler(userService, cfg.ProvisioningTimeout), identityhandler.NewAPI(service.NewAccounts(managementStore, revoker), service.NewActivation(managementStore, platformfirebase.NewFirebaseReactivator(firebaseClient)), cfg.AccountManagementTimeout))
	announcement.Register(announcementhandler.New(announcementservice.New(announcementrepo.New(pool)), cfg.AccountManagementTimeout), handler, protected)
	notificationStore := notificationrepo.New(pool)
	notification.Register(notificationhandler.New(notificationservice.New(notificationStore), cfg.AccountManagementTimeout), handler, protected)
	notificationCtx, stopNotifications := context.WithCancel(ctx)
	notificationDone := make(chan struct{})
	go func() {
		defer close(notificationDone)
		workers.RunNotificationDelivery(notificationCtx, notificationStore, logger)
	}()
	defer func() { stopNotifications(); <-notificationDone }()
	complaintStore := complaintrepo.New(pool)
	complaint.Register(complainthandler.New(complaintservice.New(complaintStore), cfg.AccountManagementTimeout), handler, protected)
	leave.Register(leavehandler.New(leaveservice.New(leaverepo.New(pool)), cfg.AccountManagementTimeout), handler, protected)
	allocation.Register(allocationhandler.New(allocationservice.New(allocationrepo.New(pool)), cfg.InventoryTimeout), handler, protected)
	attendanceStore := attendancerepo.NewDevices(pool)
	var realtimePublisher attendanceservice.RealtimePublisher
	if cfg.RTDBEnabled {
		realtimePublisher, err = platformfirebase.NewRealtimeDatabase(ctx, cfg.FirebaseProjectID, cfg.FirebaseDatabaseURL)
		if err != nil {
			return err
		}
		realtimeCtx, stopRealtime := context.WithCancel(ctx)
		realtimeDone := make(chan struct{})
		go func() {
			defer close(realtimeDone)
			workers.RunAttendanceRealtime(realtimeCtx, attendanceStore, realtimePublisher, logger)
		}()
		defer func() { stopRealtime(); <-realtimeDone }()
	}
	attendance.RegisterRealtime(attendancehandler.NewRealtimeAPI(attendanceservice.NewRealtime(attendanceStore, realtimePublisher, cfg.FirebaseDatabaseURL), cfg.AccountManagementTimeout), handler, protected)
	deviceAPI := attendancehandler.New(attendanceservice.NewDevices(attendanceStore), cfg.AccountManagementTimeout, cfg.AuthenticationTimeout)
	attendance.Register(deviceAPI, handler, protected)
	attendance.RegisterScans(attendancehandler.NewScanAPI(attendanceservice.NewScans(attendanceStore, cfg.AttendanceCooldown), cfg.AccountManagementTimeout), deviceAPI, handler, protected)
	attendance.RegisterDashboard(attendancehandler.NewDashboardAPI(attendanceservice.NewDashboard(attendanceStore), cfg.AccountManagementTimeout), handler, protected)
	inventory.Register(ihandler.New(iservice.New(irepo.New(pool)), cfg.InventoryTimeout), handler, protected)
	staff.Register(staffhandler.New(staffservice.New(staffrepo.New(pool)), cfg.AccountManagementTimeout), handler, protected)
	studentStore := srepo.NewStore(pool)
	student.Register(shandler.NewAPI(sservice.New(studentStore), cfg.StudentProfileTimeout), handler, protected)
	student.RegisterImport(shandler.NewImportAPI(sservice.NewImporter(studentStore), cfg.StudentProfileTimeout), handler, protected)
	student.RegisterQR(shandler.NewQRAPI(sservice.NewQRs(studentStore), cfg.StudentProfileTimeout), handler, protected)
	var images shandler.ImageService
	var complaintObjects complaintservice.ImageObjects
	if cfg.R2Enabled {
		objects := objectstorage.New(cfg.R2AccountID, cfg.R2Bucket, cfg.R2AccessKeyID, cfg.R2SecretAccessKey, cfg.StudentProfileTimeout)
		images = sservice.NewImages(studentStore, objects)
		complaintObjects = objects
		cleanupCtx, stopCleanup := context.WithCancel(ctx)
		cleanupDone := make(chan struct{})
		go func() {
			defer close(cleanupDone)
			workers.RunStudentImageCleanup(cleanupCtx, studentStore, objects, cfg.StudentProfileTimeout, 30*time.Second)
		}()
		complaintCleanupDone := make(chan struct{})
		go func() {
			defer close(complaintCleanupDone)
			workers.RunComplaintImageCleanup(cleanupCtx, complaintStore, objects, cfg.StudentProfileTimeout, 30*time.Second)
		}()
		defer func() { stopCleanup(); <-cleanupDone; <-complaintCleanupDone }()
	}
	student.RegisterImages(shandler.NewImageAPI(images, cfg.StudentProfileTimeout), handler, protected)
	complaint.RegisterImages(complainthandler.NewImageAPI(complaintservice.NewImages(complaintStore, complaintObjects), cfg.StudentProfileTimeout), handler, protected)
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
	logger.Info("HTTP server listening", "address", listener.Addr().String(), "environment", cfg.Environment)
	return serve(ctx, srv, listener, cfg)
}
