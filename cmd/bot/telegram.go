package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/jackc/pgx/v5/pgxpool"

	"utmn-events-bot/internal/config"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/repositories"
	"utmn-events-bot/internal/telegram/middleware"
	"utmn-events-bot/internal/telegram/navigation"
	"utmn-events-bot/internal/telegram/routes/admin"
	"utmn-events-bot/internal/telegram/routes/admin/announcements"
	"utmn-events-bot/internal/telegram/routes/admin/eventcreation"
	"utmn-events-bot/internal/telegram/routes/admin/eventediting"
	"utmn-events-bot/internal/telegram/routes/admin/eventmanagement"
	"utmn-events-bot/internal/telegram/routes/admin/tags"
	adminusers "utmn-events-bot/internal/telegram/routes/admin/users"
	"utmn-events-bot/internal/telegram/routes/registration"
	"utmn-events-bot/internal/telegram/routes/user"
	userevents "utmn-events-bot/internal/telegram/routes/user/events"
	usermenu "utmn-events-bot/internal/telegram/routes/user/menu"
	"utmn-events-bot/internal/telegram/routes/user/profile"
)

const telegramPollTimeout = time.Minute

func runTelegram(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool) error {
	client, err := newTelegramHTTPClient(cfg.TelegramProxyURL)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	httpOption := bot.WithHTTPClient(telegramPollTimeout, client)

	machine := fsm.NewInMemoryFSM()

	userRepository := repositories.NewUserRepository(pool)
	profileService := profile.NewService(machine, userRepository)
	registrationService := registration.NewService(machine, userRepository)
	isAdmin := middleware.NewAdminChecker(cfg.AdminTelegramIDs, userRepository.RegistrationStatus)

	eventRepository := repositories.NewEventRepository(pool)
	eventCreationService, err := eventcreation.NewService(machine, eventRepository)

	if err != nil {
		return err
	}

	eventEditingService, err := eventediting.NewService(machine, eventRepository)
	if err != nil {
		return err
	}

	navigator := navigation.New(usermenu.DefaultPage, isAdmin)
	registrationHandler := registration.NewHandler(navigator, registrationService)

	tagRepository := repositories.NewTagRepository(pool)
	tagService := tags.NewService(machine, tagRepository, eventRepository)

	eventManagement := eventmanagement.NewService(machine, eventRepository, userRepository, cfg.AdminTelegramIDs)
	userList := adminusers.NewService(machine, userRepository, cfg.AdminTelegramIDs)
	announcementsService := announcements.NewService(ctx, machine, userRepository)

	registrations := repositories.NewEventRegistrationRepository(pool)
	browser, err := userevents.NewService(machine, eventRepository, tagRepository, registrations)

	if err != nil {
		return fmt.Errorf("create event browser: %w", err)
	}

	router, err := bot.New(cfg.BotToken, httpOption, bot.WithSkipGetMe(), bot.WithNotAsyncHandlers(),
		bot.WithMiddlewares(middleware.LogUpdateDuration, registrationHandler.RequireRegistered))
	if err != nil {
		return fmt.Errorf("create update router: %w", err)
	}
	registrationHandler.Register(router)
	navigator.Register(router)
	admin.Register(router, navigator, isAdmin, eventCreationService, tagService, eventManagement, userList, announcementsService, eventEditingService, machine.Clear)
	user.Register(router, navigator, isAdmin, browser, profileService, machine.Clear)

	dispatcher := newUpdateDispatcher(router)
	poller, err := bot.New(cfg.BotToken,
		httpOption,
		bot.WithDefaultHandler(dispatcher.enqueue),
		bot.WithWorkers(1),
		bot.WithNotAsyncHandlers(),
	)
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}

	navigator.SetCleanup(machine.Clear)

	slog.Info("bot starting")
	poller.Start(ctx)
	dispatcher.workers.Wait()
	slog.Info("bot stopped")

	return nil
}

func newTelegramHTTPClient(proxyURL string) (*http.Client, error) {
	client := &http.Client{Timeout: telegramPollTimeout}
	if strings.TrimSpace(proxyURL) == "" {
		return client, nil
	}
	proxy, err := url.Parse(strings.TrimSpace(proxyURL))
	if err != nil || (proxy.Scheme != "socks5" && proxy.Scheme != "socks5h") ||
		proxy.Hostname() == "" || proxy.Path != "" || proxy.RawQuery != "" || proxy.Fragment != "" {
		return nil, fmt.Errorf("BOT_PROXY_URL must be a SOCKS5 URL such as socks5://host:1080")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxy)
	client.Transport = transport
	return client, nil
}
