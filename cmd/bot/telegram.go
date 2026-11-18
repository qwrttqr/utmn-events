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
	"utmn-events-bot/internal/telegram/handlers/admin"
	"utmn-events-bot/internal/telegram/handlers/admin/announcements"
	"utmn-events-bot/internal/telegram/handlers/admin/eventcreation"
	"utmn-events-bot/internal/telegram/handlers/admin/eventediting"
	"utmn-events-bot/internal/telegram/handlers/admin/eventmanagement"
	"utmn-events-bot/internal/telegram/handlers/admin/tags"
	adminusers "utmn-events-bot/internal/telegram/handlers/admin/users"
	"utmn-events-bot/internal/telegram/handlers/registration"
	"utmn-events-bot/internal/telegram/handlers/user"
	userevents "utmn-events-bot/internal/telegram/handlers/user/events"
	usermenu "utmn-events-bot/internal/telegram/handlers/user/menu"
	"utmn-events-bot/internal/telegram/handlers/user/profile"
	"utmn-events-bot/internal/telegram/navigation"
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
	profileService := profile.NewService(machine, userRepository, cfg.AdminTelegramIDs)
	registrationService := registration.NewService(machine, userRepository)

	eventRepository := repositories.NewEventRepository(pool)
	eventCreationService, err := eventcreation.NewService(machine, eventRepository)

	if err != nil {
		return err
	}

	eventEditingService, err := eventediting.NewService(machine, eventRepository)
	if err != nil {
		return err
	}

	navigator := navigation.New(cfg.AdminTelegramIDs, func(id int64) navigation.Page { return usermenu.DefaultPage(id, cfg.AdminTelegramIDs) })
	registrationHandler := registration.NewHandler(navigator, registrationService)

	b, err := bot.New(cfg.BotToken,
		httpOption,
		bot.WithMiddlewares(registrationHandler.RequireRegistered),
		bot.WithSkipGetMe(),
		bot.WithNotAsyncHandlers(),
	)
	if err != nil {
		return fmt.Errorf("create update router: %w", err)
	}
	dispatcher := newUpdateDispatcher(b)

	poller, err := bot.New(cfg.BotToken,
		httpOption,
		bot.WithDefaultHandler(dispatcher.enqueue),
		bot.WithWorkers(1),
		bot.WithNotAsyncHandlers(),
	)

	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}

	registrationHandler.Register(b)

	tagRepository := repositories.NewTagRepository(pool)
	tagService := tags.NewService(machine, tagRepository, eventRepository)

	navigator.Register(b)

	eventManagement := eventmanagement.NewService(machine, eventRepository, userRepository, cfg.AdminTelegramIDs)
	userList := adminusers.NewService(machine, userRepository, cfg.AdminTelegramIDs)
	announcementsService := announcements.NewService(ctx, machine, userRepository)

	defer announcementsService.Close()

	registrations := repositories.NewEventRegistrationRepository(pool)
	browser, err := userevents.NewService(machine, eventRepository, tagRepository, registrations, cfg.AdminTelegramIDs)

	if err != nil {
		return fmt.Errorf("create event browser: %w", err)
	}

	admin.Register(b, navigator, cfg.AdminTelegramIDs, eventCreationService, tagService, eventManagement, userList, announcementsService, eventEditingService, machine.Clear)
	user.Register(b, navigator, cfg.AdminTelegramIDs, browser, profileService, machine.Clear)

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
