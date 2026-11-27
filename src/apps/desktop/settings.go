package main

import (
	"context"
	"strconv"

	"github.com/acoradd/otter/src/internal/apperr"
	"github.com/acoradd/otter/src/internal/store"
)

// EventSettingsChanged is emitted after a setting changes, with the new Settings.
const EventSettingsChanged = "settings.changed"

// Setting keys in the setting table.
const (
	settingLanguage      = "language"
	settingTheme         = "theme"
	settingLastWorkspace = "lastWorkspace"
)

// Themes the app knows. "system" follows the OS.
var themes = map[string]bool{"system": true, "light": true, "dark": true}

// Languages the UI speaks. "" follows the system language.
var languages = map[string]bool{"": true, "en": true, "fr": true}

// Settings are the app's preferences, stored in otter.db.
type Settings struct {
	// Language is "en" or "fr", or "" to follow the system.
	Language string `json:"language"`
	// Theme is "system", "light" or "dark".
	Theme string `json:"theme"`
	// LastWorkspaceID is the workspace shown at start, 0 when none was chosen.
	LastWorkspaceID int64 `json:"lastWorkspaceId"`
}

// loadSettings reads the settings, with their defaults.
func loadSettings(ctx context.Context, db *store.DB) (Settings, error) {
	s := Settings{Theme: "system"}
	if v, ok, err := db.Setting(ctx, settingLanguage); err != nil {
		return s, err
	} else if ok && languages[v] {
		s.Language = v
	}
	if v, ok, err := db.Setting(ctx, settingTheme); err != nil {
		return s, err
	} else if ok && themes[v] {
		s.Theme = v
	}
	if v, ok, err := db.Setting(ctx, settingLastWorkspace); err != nil {
		return s, err
	} else if ok {
		s.LastWorkspaceID, _ = strconv.ParseInt(v, 10, 64)
	}
	return s, nil
}

// SettingsService reads and changes the app's preferences.
type SettingsService struct {
	db   *store.DB
	emit func(name string, data any)
}

func NewSettingsService(db *store.DB, emit func(string, any)) *SettingsService {
	return &SettingsService{db: db, emit: emit}
}

// GetSettings returns the settings.
func (s *SettingsService) GetSettings(ctx context.Context) (Settings, error) {
	return loadSettings(ctx, s.db)
}

// SetLanguage sets the UI language: "en", "fr", or "" to follow the system.
func (s *SettingsService) SetLanguage(ctx context.Context, language string) error {
	if !languages[language] {
		return apperr.New(apperr.InvalidInput)
	}
	return setAndBroadcast(ctx, s.db, s.emit, settingLanguage, language)
}

// SetTheme sets the theme: "system", "light" or "dark".
func (s *SettingsService) SetTheme(ctx context.Context, theme string) error {
	if !themes[theme] {
		return apperr.New(apperr.InvalidInput)
	}
	return setAndBroadcast(ctx, s.db, s.emit, settingTheme, theme)
}

// setAndBroadcast stores a setting and emits the new settings.
func setAndBroadcast(ctx context.Context, db *store.DB, emit func(string, any), key, value string) error {
	if err := db.SetSetting(ctx, key, value); err != nil {
		return err
	}
	settings, err := loadSettings(ctx, db)
	if err != nil {
		return err
	}
	emit(EventSettingsChanged, settings)
	return nil
}
