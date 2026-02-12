package main

import (
	"log"
	"net/http"

	"my-stash-rule/internal/config"
	"my-stash-rule/internal/handler"
	"my-stash-rule/internal/service"
	"my-stash-rule/internal/store"
)

func main() {
	config.LoadEnv()

	// Initialize Redis
	if err := store.InitRedis(); err != nil {
		log.Fatal("Failed to initialize Redis:", err)
	}
	service.StartDailyProxyCacheScheduler()

	http.HandleFunc("/", handler.HandleGetConfig)
	http.HandleFunc("/health", handler.HandleHealthCheck)

	// Auth routes
	http.HandleFunc("/login", handler.HandleLogin)
	http.HandleFunc("/logout", handler.HandleLogout)

	// Protected routes
	http.HandleFunc("/admin", handler.AdminAuthMiddleware(handler.HandleAdminPage))
	http.HandleFunc("/admin/config", handler.AdminAuthMiddleware(handler.HandleAdminConfigPage))
	http.HandleFunc("/admin/profiles", handler.AdminAuthMiddleware(handler.HandleAdminProfilesPage))
	http.HandleFunc("/admin/subscribers", handler.AdminAuthMiddleware(handler.HandleAdminSubscribersPage))
	http.HandleFunc("/admin/account", handler.AdminAuthMiddleware(handler.HandleAdminAccountPage))
	http.HandleFunc("/api/config", handler.AdminAuthMiddleware(handler.HandleConfigAPI))
	http.HandleFunc("/api/proxy/cache", handler.AdminAuthMiddleware(handler.HandleProxyCacheAPI))
	http.HandleFunc("/api/stash/profiles", handler.AdminAuthMiddleware(handler.HandleStashProfilesAPI))
	http.HandleFunc("/api/admin/profile", handler.AdminAuthMiddleware(handler.HandleAdminProfileAPI))
	http.HandleFunc("/api/subscribers", handler.AdminAuthMiddleware(handler.HandleSubscribersAPI))
	http.HandleFunc("/api/user/info", handler.AdminAuthMiddleware(handler.HandleGetUserInfo))

	port := config.GetPort()
	addr := ":" + port
	log.Printf("Starting Stash Rule Service on %s", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
