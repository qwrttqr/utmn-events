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
	"utmn-events-bot/internal/telegram/handlers/admin/events"
	"utmn-events-bot/internal/telegram/handlers/admin/events/manage"
	"utmn-events-bot/internal/telegram/handlers/admin/tags"
	adminusers "utmn-events-bot/internal/telegram/handlers/admin/users"
	"utmn-events-bot/internal/telegram/handlers/registration"
	"utmn-events-bot/internal/telegram/handlers/user"
	userevents "utmn-events-bot/internal/telegram/handlers/user/events"
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
	registrationPipeline := registration.New(userRepository)
	eventRepository := repositories.NewEventRepository(pool)
	eventFlow, err := events.New(eventRepository)
	if err != nil {
		return err
	}
	navigator := navigation.New(cfg.AdminTelegramIDs, func(id int64) navigation.Page { return user.DefaultPage(id, cfg.AdminTelegramIDs) })
	b, err := bot.New(cfg.BotToken,
		bot.WithMiddlewares(navigator.Middleware, registrationPipeline.RequireRegistered),
		// Preserve input order for the multi-step registration conversation.
		bot.WithWorkers(1),
		bot.WithNotAsyncHandlers(),
	)
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}
	tagRepository := repositories.NewTagRepository(pool)
	tagFlow := tags.New(tagRepository, eventRepository)
	navigator.Register(b)
	eventManagement := manage.New(eventRepository, eventFlow, userRepository, cfg.AdminTelegramIDs)
	userList := adminusers.New(userRepository, cfg.AdminTelegramIDs)
	registrations := repositories.NewEventRegistrationRepository(pool)
	browser, err := userevents.New(eventRepository, tagRepository, registrations)
	if err != nil {
		return fmt.Errorf("create event browser: %w", err)
	}
	cancelAdminDrafts := admin.Register(b, cfg.AdminTelegramIDs, eventFlow, tagFlow, eventManagement, userList)
	cancelAll := func(id int64) { cancelAdminDrafts(id); browser.Cancel(id) }
	navigator.SetCleanup(cancelAll)
	user.Register(b, cfg.AdminTelegramIDs, cancelAll)
	userevents.Register(b, browser, cancelAdminDrafts)

	slog.Info("bot starting")
	b.Start(ctx)
	slog.Info("bot stopped")

	return nil
}
