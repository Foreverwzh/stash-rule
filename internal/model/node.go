package model

// ProxyNode definition
type ProxyNode map[string]interface{}

// ProxyGroup definition
type ProxyGroup struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"`
	Proxies []string `yaml:"proxies,omitempty"`
	Url     string   `yaml:"url,omitempty"`
	Interval int     `yaml:"interval,omitempty"`
}
