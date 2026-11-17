package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	"utmn-events-bot/internal/config"
	"utmn-events-bot/internal/repositories"
	"utmn-events-bot/internal/telegram/handlers/admin"
	"utmn-events-bot/internal/telegram/handlers/admin/announcements"
	"utmn-events-bot/internal/telegram/handlers/admin/event_creation"
	"utmn-events-bot/internal/telegram/handlers/admin/event_editing"
	"utmn-events-bot/internal/telegram/handlers/admin/event_management"
	"utmn-events-bot/internal/telegram/handlers/admin/tags"
	adminusers "utmn-events-bot/internal/telegram/handlers/admin/users"
	"utmn-events-bot/internal/telegram/handlers/registration"
	"utmn-events-bot/internal/telegram/handlers/user"
	userevents "utmn-events-bot/internal/telegram/handlers/user/events"
	usermenu "utmn-events-bot/internal/telegram/handlers/user/menu"
	"utmn-events-bot/internal/telegram/navigation"

	"github.com/go-telegram/bot"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		logger.Error("bot stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		// Connection-string parser errors may contain database credentials.
		return fmt.Errorf("invalid DATABASE_URL")
	}
	poolConfig.ConnConfig.RuntimeParams["timezone"] = "UTC"
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	defer pool.Close()
	pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
	err = pool.Ping(pingCtx)
	pingCancel()
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	userRepository := repositories.NewUserRepository(pool)
	registrationService := registration.NewService(userRepository)
	registrationHandler := registration.NewHandler(registrationService)
	eventRepository := repositories.NewEventRepository(pool)
	eventCreationService, err := event_creation.NewService(eventRepository)
	if err != nil {
		return err
	}
	eventEditingService, err := event_editing.NewService(eventRepository)
	if err != nil {
		return err
	}

	navigator := navigation.New(cfg.AdminTelegramIDs, func(id int64) navigation.Page { return usermenu.DefaultPage(id, cfg.AdminTelegramIDs) })
	b, err := bot.New(cfg.BotToken,
		bot.WithMiddlewares(navigator.Middleware, registrationHandler.RequireRegistered),
		// Preserve input order for the multi-step registration conversation.
		bot.WithWorkers(1),
		bot.WithNotAsyncHandlers(),
	)
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}
	registration.Register(b, registrationService)

	tagRepository := repositories.NewTagRepository(pool)
	tagService := tags.NewService(tagRepository, eventRepository)
	navigator.Register(b)
	eventManagement := event_management.NewService(eventRepository, userRepository, cfg.AdminTelegramIDs)
	userList := adminusers.NewService(userRepository, cfg.AdminTelegramIDs)
	announcementsService := announcements.NewService(ctx, userRepository)
	defer announcementsService.Close()
	registrations := repositories.NewEventRegistrationRepository(pool)
	browser, err := userevents.NewService(eventRepository, tagRepository, registrations)
	if err != nil {
		return fmt.Errorf("create event browser: %w", err)
	}
	cancelAdminDrafts := admin.Register(b, cfg.AdminTelegramIDs, eventCreationService, tagService, eventManagement, userList, announcementsService, eventEditingService)
	cancelAll := user.Register(b, cfg.AdminTelegramIDs, browser, cancelAdminDrafts)
	navigator.SetCleanup(cancelAll)

	slog.Info("bot starting")
	b.Start(ctx)
	slog.Info("bot stopped")

	return nil
}
