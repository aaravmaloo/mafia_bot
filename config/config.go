package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"go.mau.fi/whatsmeow/types"
	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	GroupJID           string        `yaml:"group_jid"`
	Prefix             string        `yaml:"prefix"`
	MinPlayers         int           `yaml:"min_players"`
	NightDuration      time.Duration `yaml:"night_duration"`
	DayDuration        time.Duration `yaml:"day_duration"`
	NominationDuration time.Duration `yaml:"nomination_duration"`
	TrialDuration      time.Duration `yaml:"trial_duration"`
	VotingDuration     time.Duration `yaml:"voting_duration"`
	DatabaseDriver     string        `yaml:"database_driver"`
	DatabaseDSN        string        `yaml:"database_dsn"`
	LogLevel           string        `yaml:"log_level"`
}

func Load(path string) (AppConfig, error) {
	_ = godotenv.Load()

	cfg := AppConfig{
		Prefix:             "!",
		MinPlayers:         5,
		NightDuration:      120 * time.Second,
		DayDuration:        5 * time.Minute,
		NominationDuration: 60 * time.Second,
		TrialDuration:      60 * time.Second,
		VotingDuration:     60 * time.Second,
		DatabaseDriver:     "sqlite3",
		DatabaseDSN:        "file:mafia-bot.db?_foreign_keys=on",
		LogLevel:           "INFO",
	}

	if raw, err := os.ReadFile(path); err == nil {
		if err = yaml.Unmarshal(raw, &cfg); err != nil {
			return AppConfig{}, fmt.Errorf("parse config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return AppConfig{}, fmt.Errorf("read config: %w", err)
	}

	overrideString(&cfg.GroupJID, "BOT_GROUP_JID")
	overrideString(&cfg.Prefix, "BOT_PREFIX")
	overrideString(&cfg.DatabaseDriver, "BOT_DB_DRIVER")
	overrideString(&cfg.DatabaseDSN, "BOT_DB_DSN")
	overrideString(&cfg.LogLevel, "BOT_LOG_LEVEL")
	overrideInt(&cfg.MinPlayers, "BOT_MIN_PLAYERS")
	overrideDuration(&cfg.NightDuration, "BOT_NIGHT_DURATION")
	overrideDuration(&cfg.DayDuration, "BOT_DAY_DURATION")
	overrideDuration(&cfg.NominationDuration, "BOT_NOMINATION_DURATION")
	overrideDuration(&cfg.TrialDuration, "BOT_TRIAL_DURATION")
	overrideDuration(&cfg.VotingDuration, "BOT_VOTING_DURATION")

	cfg.Prefix = strings.TrimSpace(cfg.Prefix)
	if cfg.Prefix == "" {
		cfg.Prefix = "!"
	}
	if cfg.MinPlayers < 4 {
		cfg.MinPlayers = 4
	}

	return cfg, nil
}

func (cfg AppConfig) ParsedGroupJID() (types.JID, error) {
	if strings.TrimSpace(cfg.GroupJID) == "" {
		return types.EmptyJID, nil
	}

	jid, err := types.ParseJID(cfg.GroupJID)
	if err != nil {
		return types.EmptyJID, fmt.Errorf("parse group_jid: %w", err)
	}

	return jid, nil
}

func overrideString(target *string, key string) {
	value := strings.TrimSpace(os.Getenv(key))
	if value != "" {
		*target = value
	}
}

func overrideInt(target *int, key string) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return
	}

	parsed, err := strconv.Atoi(value)
	if err == nil {
		*target = parsed
	}
}

func overrideDuration(target *time.Duration, key string) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return
	}

	parsed, err := time.ParseDuration(value)
	if err == nil {
		*target = parsed
	}
}
