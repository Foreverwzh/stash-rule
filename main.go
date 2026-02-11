package main

import (
	"log"
	"net/http"

	"my-stash-rule/internal/config"
	"my-stash-rule/internal/handler"
	"my-stash-rule/internal/store"
)

func main() {
	config.LoadEnv()

	// Initialize Redis
	if err := store.InitRedis(); err != nil {
		log.Fatal("Failed to initialize Redis:", err)
	}

	http.HandleFunc("/", handler.HandleGetConfig)
	http.HandleFunc("/health", handler.HandleHealthCheck)

	// Auth routes
	http.HandleFunc("/login", handler.HandleLogin)
	http.HandleFunc("/logout", handler.HandleLogout)

	// Protected routes
	http.HandleFunc("/admin", handler.AdminAuthMiddleware(handler.HandleAdminPage))
	http.HandleFunc("/api/config", handler.AdminAuthMiddleware(handler.HandleConfigAPI))
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
