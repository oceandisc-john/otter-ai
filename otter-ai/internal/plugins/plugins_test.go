package plugins

import (
	"context"
	"testing"

	"otter-ai/internal/config"
)

// --- Manager ---

func TestNewManager(t *testing.T) {
	cfg := config.PluginConfig{}
	m := NewManager(cfg)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
}

func TestManager_Get_Empty(t *testing.T) {
	m := NewManager(config.PluginConfig{})
	_, ok := m.Get("discord")
	if ok {
		t.Error("expected false for unloaded plugin")
	}
}

func TestManager_LoadAll_NoneEnabled(t *testing.T) {
	m := NewManager(config.PluginConfig{})
	err := m.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
}

func TestManager_LoadAll_AllEnabled(t *testing.T) {
	cfg := config.PluginConfig{
		Discord:  config.PluginSettings{Enabled: true, Config: map[string]string{}},
		Signal:   config.PluginSettings{Enabled: true, Config: map[string]string{}},
		Telegram: config.PluginSettings{Enabled: true, Config: map[string]string{}},
		Slack:    config.PluginSettings{Enabled: true, Config: map[string]string{}},
	}
	m := NewManager(cfg)
	err := m.LoadAll(context.Background())
	// All plugins return "not yet implemented" from Initialize, so this should error
	if err == nil {
		t.Error("expected errors from stub plugin Initialize")
	}
}

func TestManager_HandleMessage_NoPlatform(t *testing.T) {
	m := NewManager(config.PluginConfig{})
	err := m.HandleMessage(context.Background(), &Message{Platform: "discord"})
	if err == nil {
		t.Error("expected error for missing platform")
	}
}

func TestManager_SendMessage_NoPlatform(t *testing.T) {
	m := NewManager(config.PluginConfig{})
	err := m.SendMessage(context.Background(), "discord", &Message{})
	if err == nil {
		t.Error("expected error for missing platform")
	}
}

func TestManager_UnloadAll_Empty(t *testing.T) {
	m := NewManager(config.PluginConfig{})
	err := m.UnloadAll(context.Background())
	if err != nil {
		t.Fatalf("UnloadAll: %v", err)
	}
}

// --- Plugin stubs ---

func TestDiscordPlugin(t *testing.T) {
	p, err := NewDiscordPlugin()
	if err != nil {
		t.Fatalf("NewDiscordPlugin: %v", err)
	}
	if p.Name() != "discord" {
		t.Errorf("Name() = %q", p.Name())
	}
	if err := p.Initialize(context.Background(), nil); err == nil {
		t.Error("expected error from stub Initialize")
	}
	if err := p.HandleMessage(context.Background(), &Message{}); err == nil {
		t.Error("expected error from stub HandleMessage")
	}
	if err := p.SendMessage(context.Background(), &Message{}); err == nil {
		t.Error("expected error from stub SendMessage")
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

func TestSignalPlugin(t *testing.T) {
	p, err := NewSignalPlugin()
	if err != nil {
		t.Fatalf("NewSignalPlugin: %v", err)
	}
	if p.Name() != "signal" {
		t.Errorf("Name() = %q", p.Name())
	}
	if err := p.Initialize(context.Background(), nil); err == nil {
		t.Error("expected error from stub")
	}
	if err := p.HandleMessage(context.Background(), &Message{}); err == nil {
		t.Error("expected error from stub")
	}
	if err := p.SendMessage(context.Background(), &Message{}); err == nil {
		t.Error("expected error from stub")
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

func TestTelegramPlugin(t *testing.T) {
	p, err := NewTelegramPlugin()
	if err != nil {
		t.Fatalf("NewTelegramPlugin: %v", err)
	}
	if p.Name() != "telegram" {
		t.Errorf("Name() = %q", p.Name())
	}
	if err := p.Initialize(context.Background(), nil); err == nil {
		t.Error("expected error from stub")
	}
	if err := p.HandleMessage(context.Background(), &Message{}); err == nil {
		t.Error("expected error from stub")
	}
	if err := p.SendMessage(context.Background(), &Message{}); err == nil {
		t.Error("expected error from stub")
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

func TestSlackPlugin(t *testing.T) {
	p, err := NewSlackPlugin()
	if err != nil {
		t.Fatalf("NewSlackPlugin: %v", err)
	}
	if p.Name() != "slack" {
		t.Errorf("Name() = %q", p.Name())
	}
	if err := p.Initialize(context.Background(), nil); err == nil {
		t.Error("expected error from stub")
	}
	if err := p.HandleMessage(context.Background(), &Message{}); err == nil {
		t.Error("expected error from stub")
	}
	if err := p.SendMessage(context.Background(), &Message{}); err == nil {
		t.Error("expected error from stub")
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

// --- Manual register + Get ---

func TestManager_RegisterAndGet(t *testing.T) {
	m := NewManager(config.PluginConfig{})
	p, _ := NewDiscordPlugin()
	m.register(p)

	got, ok := m.Get("discord")
	if !ok {
		t.Error("expected to find registered plugin")
	}
	if got.Name() != "discord" {
		t.Errorf("Name() = %q", got.Name())
	}
}

// --- UnloadAll with registered plugins ---

func TestManager_UnloadAll_WithPlugins(t *testing.T) {
	m := NewManager(config.PluginConfig{})
	p, _ := NewDiscordPlugin()
	m.register(p)

	err := m.UnloadAll(context.Background())
	if err != nil {
		t.Fatalf("UnloadAll: %v", err)
	}

	// Should be empty after unload
	_, ok := m.Get("discord")
	if ok {
		t.Error("plugin should be unloaded")
	}
}
