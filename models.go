package main

import (
	"fmt"
	"math/rand"
	"strings"

	"gopkg.in/yaml.v2"
)

// ProxyEntry represents a single proxy configuration
type ProxyEntry struct {
	Name string `yaml:"name" json:"name"`
	Path string `yaml:"path" json:"path"`
	Icon string `json:"icon"` // Random icon for display
}

// ProxyListConfig represents the proxy list configuration
type ProxyListConfig struct {
	ProxyList []ProxyEntry `yaml:"proxyList" json:"proxyList"`
}

// getRandomIcon returns a random icon emoji for proxy boxes
func getRandomIcon() string {
	icons := []string{
		"🌐", "🔗", "🚀", "⚡", "🔧", "🛠️", "📡", "🔌",
		"💻", "🖥️", "📱", "⭐", "🎯", "🔥", "💎", "🎨",
		"🎪", "🎭", "🎨", "🎯", "🎲", "🎳", "🎮", "🎸",
		"🎵", "🎶", "🎺", "🎻", "🥁", "🎹", "🎤", "🎧",
		"🛰️", "📶", "🧭", "🗺️", "🌏", "🧩", "📚", "🦉",
		"📦", "🧰", "🧠", "🧪", "🔬", "🔭", "🧱", "🧲",
		"🗝️", "🔒", "🔑", "🛡️", "⚙️", "🪛", "🔩", "🧵",
		"🧶", "🪄", "🕹️", "🎬", "📣", "📢", "📝", "📌",
		"📍", "📎", "📚", "🗂️", "📊", "📈", "📉", "🗃️",
		"☁️", "🌦️", "🌈", "🌙", "✨", "💡", "🔋", "🔍",
		"🧿", "🧸", "🦾", "🤖", "🦄", "🐳", "🦉", "🦅",
	}
	return icons[rand.Intn(len(icons))]
}

// parseProxyList parses the YAML proxy list configuration and assigns random icons
func parseProxyList(yamlContent string) (*ProxyListConfig, error) {
	if yamlContent == "" {
		return &ProxyListConfig{ProxyList: []ProxyEntry{}}, nil
	}

	var config ProxyListConfig
	if err := yaml.Unmarshal([]byte(yamlContent), &config); err != nil {
		return nil, fmt.Errorf("failed to parse proxy list YAML: %w", err)
	}

	// Assign random icons and normalize paths for each proxy entry
	for i := range config.ProxyList {
		config.ProxyList[i].Icon = getRandomIcon()
		// Ensure path has a leading /
		path := strings.TrimSpace(config.ProxyList[i].Path)
		if path != "" && !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		config.ProxyList[i].Path = path
	}

	return &config, nil
}
