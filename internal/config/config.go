package config

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	TelegramProxyURL string  `env:"BOT_PROXY_URL"`
	BotToken         string  `env:"BOT_TOKEN" env-required:"true"`
	DatabaseURL      string  `env:"DATABASE_URL" env-required:"true"`
	AdminTelegramIDs []int64 `env:"BOT_ADMIN_TELEGRAM_IDS" env-separator:";" env-required:"true"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return nil, fmt.Errorf("configuration: %w", err)
	}
	return &cfg, nil
}
