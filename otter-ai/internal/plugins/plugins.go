package plugins

import (
	"context"
	"fmt"
	"sync"

	"otter-ai/internal/config"
)

// Plugin is the interface all plugins must implement
type Plugin interface {
	// Name returns the plugin name
	Name() string

	// Initialize initializes the plugin
	Initialize(ctx context.Context, config map[string]string) error

	// HandleMessage processes an incoming message
	HandleMessage(ctx context.Context, message *Message) error

	// SendMessage sends a message through the plugin
	SendMessage(ctx context.Context, message *Message) error

	// Shutdown gracefully shuts down the plugin
	Shutdown(ctx context.Context) error
}

// Message represents a plugin message
type Message struct {
	ID        string
	Platform  string
	ChannelID string
	UserID    string
	Username  string
	Content   string
	Timestamp int64
	Metadata  map[string]interface{}
}

// Manager manages all loaded plugins
type Manager struct {
	config  config.PluginConfig
	plugins map[string]Plugin
	mu      sync.RWMutex
}

// NewManager creates a new plugin manager
func NewManager(config config.PluginConfig) *Manager {
	return &Manager{
		config:  config,
		plugins: make(map[string]Plugin),
	}
}

// LoadAll loads all enabled plugins
func (m *Manager) LoadAll(ctx context.Context) error {
	var errors []error

	// Load Discord plugin if enabled
	if m.config.Discord.Enabled {
		plugin, err := NewDiscordPlugin()
		if err != nil {
			errors = append(errors, fmt.Errorf("discord: %w", err))
		} else {
			if err := plugin.Initialize(ctx, m.config.Discord.Config); err != nil {
				errors = append(errors, fmt.Errorf("discord init: %w", err))
			} else {
				m.register(plugin)
			}
		}
	}

	// Load Signal plugin if enabled
	if m.config.Signal.Enabled {
		plugin, err := NewSignalPlugin()
		if err != nil {
			errors = append(errors, fmt.Errorf("signal: %w", err))
		} else {
			if err := plugin.Initialize(ctx, m.config.Signal.Config); err != nil {
				errors = append(errors, fmt.Errorf("signal init: %w", err))
			} else {
				m.register(plugin)
			}
		}
	}

	// Load Telegram plugin if enabled
	if m.config.Telegram.Enabled {
		plugin, err := NewTelegramPlugin()
		if err != nil {
			errors = append(errors, fmt.Errorf("telegram: %w", err))
		} else {
			if err := plugin.Initialize(ctx, m.config.Telegram.Config); err != nil {
				errors = append(errors, fmt.Errorf("telegram init: %w", err))
			} else {
				m.register(plugin)
			}
		}
	}

	// Load Slack plugin if enabled
	if m.config.Slack.Enabled {
		plugin, err := NewSlackPlugin()
		if err != nil {
			errors = append(errors, fmt.Errorf("slack: %w", err))
		} else {
			if err := plugin.Initialize(ctx, m.config.Slack.Config); err != nil {
				errors = append(errors, fmt.Errorf("slack init: %w", err))
			} else {
				m.register(plugin)
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("plugin loading errors: %v", errors)
	}

	return nil
}

// register adds a plugin to the manager
func (m *Manager) register(plugin Plugin) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plugins[plugin.Name()] = plugin
}

// Get retrieves a plugin by name
func (m *Manager) Get(name string) (Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	plugin, exists := m.plugins[name]
	return plugin, exists
}

// HandleMessage routes a message to the appropriate plugin
func (m *Manager) HandleMessage(ctx context.Context, message *Message) error {
	m.mu.RLock()
	plugin, exists := m.plugins[message.Platform]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no plugin for platform: %s", message.Platform)
	}

	return plugin.HandleMessage(ctx, message)
}

// SendMessage sends a message through a specific plugin
func (m *Manager) SendMessage(ctx context.Context, platform string, message *Message) error {
	m.mu.RLock()
	plugin, exists := m.plugins[platform]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no plugin for platform: %s", platform)
	}

	return plugin.SendMessage(ctx, message)
}

// UnloadAll unloads all plugins
func (m *Manager) UnloadAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errors []error

	for name, plugin := range m.plugins {
		if err := plugin.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", name, err))
		}
	}

	m.plugins = make(map[string]Plugin)

	if len(errors) > 0 {
		return fmt.Errorf("plugin unload errors: %v", errors)
	}

	return nil
}

// Plugin stubs

// DiscordPlugin stub
type DiscordPlugin struct{}

func NewDiscordPlugin() (*DiscordPlugin, error) {
	return &DiscordPlugin{}, nil
}

func (p *DiscordPlugin) Name() string {
	return "discord"
}

func (p *DiscordPlugin) Initialize(ctx context.Context, config map[string]string) error {
	return fmt.Errorf("discord plugin not yet implemented")
}

func (p *DiscordPlugin) HandleMessage(ctx context.Context, message *Message) error {
	return fmt.Errorf("not implemented")
}

func (p *DiscordPlugin) SendMessage(ctx context.Context, message *Message) error {
	return fmt.Errorf("not implemented")
}

func (p *DiscordPlugin) Shutdown(ctx context.Context) error {
	return nil
}

// SignalPlugin stub
type SignalPlugin struct{}

func NewSignalPlugin() (*SignalPlugin, error) {
	return &SignalPlugin{}, nil
}

func (p *SignalPlugin) Name() string {
	return "signal"
}

func (p *SignalPlugin) Initialize(ctx context.Context, config map[string]string) error {
	return fmt.Errorf("signal plugin not yet implemented")
}

func (p *SignalPlugin) HandleMessage(ctx context.Context, message *Message) error {
	return fmt.Errorf("not implemented")
}

func (p *SignalPlugin) SendMessage(ctx context.Context, message *Message) error {
	return fmt.Errorf("not implemented")
}

func (p *SignalPlugin) Shutdown(ctx context.Context) error {
	return nil
}

// TelegramPlugin stub
type TelegramPlugin struct{}

func NewTelegramPlugin() (*TelegramPlugin, error) {
	return &TelegramPlugin{}, nil
}

func (p *TelegramPlugin) Name() string {
	return "telegram"
}

func (p *TelegramPlugin) Initialize(ctx context.Context, config map[string]string) error {
	return fmt.Errorf("telegram plugin not yet implemented")
}

func (p *TelegramPlugin) HandleMessage(ctx context.Context, message *Message) error {
	return fmt.Errorf("not implemented")
}

func (p *TelegramPlugin) SendMessage(ctx context.Context, message *Message) error {
	return fmt.Errorf("not implemented")
}

func (p *TelegramPlugin) Shutdown(ctx context.Context) error {
	return nil
}

// SlackPlugin stub
type SlackPlugin struct{}

func NewSlackPlugin() (*SlackPlugin, error) {
	return &SlackPlugin{}, nil
}

func (p *SlackPlugin) Name() string {
	return "slack"
}

func (p *SlackPlugin) Initialize(ctx context.Context, config map[string]string) error {
	return fmt.Errorf("slack plugin not yet implemented")
}

func (p *SlackPlugin) HandleMessage(ctx context.Context, message *Message) error {
	return fmt.Errorf("not implemented")
}

func (p *SlackPlugin) SendMessage(ctx context.Context, message *Message) error {
	return fmt.Errorf("not implemented")
}

func (p *SlackPlugin) Shutdown(ctx context.Context) error {
	return nil
}
