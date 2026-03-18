package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gosystem/internal/config"
	"gosystem/internal/db"
	"gosystem/internal/httpapi"
	"gosystem/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	logger := log.New(os.Stdout, "[gosystem] ", log.LstdFlags)

	database, err := db.Open(ctx, cfg.Database)
	if err != nil {
		logger.Fatalf("db connect failed: %v", err)
	}
	defer func() {
		_ = database.Close()
	}()

	taskStore := store.NewTaskStore(database)
	projectStore := store.NewProjectStore(database)
	commentStore := store.NewCommentStore(database)
	userStore := store.NewUserStore(database)
	assigneeStore := store.NewAssigneeStore(database)
	memberStore := store.NewProjectMemberStore(database)
	checklistStore := store.NewChecklistStore(database)
	activityStore := store.NewActivityStore(database)
	tagStore := store.NewTagStore(database)
	handler := httpapi.NewHandler(
		taskStore,
		projectStore,
		commentStore,
		userStore,
		assigneeStore,
		memberStore,
		checklistStore,
		activityStore,
		tagStore,
		logger,
	)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Printf("http server starting on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Println("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	case err := <-errCh:
		logger.Fatalf("server failed: %v", err)
	}
}
