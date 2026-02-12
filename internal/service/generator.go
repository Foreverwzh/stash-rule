package service

import (
	"bytes"
	"reflect"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// GenerateConfig generates the complete Stash YAML configuration
func GenerateConfig(proxies []ProxyNode, overlays ...map[string]interface{}) ([]byte, error) {
	config := BuildConfigMap(proxies)
	for _, overlay := range overlays {
		config = DeepMergeMap(config, overlay)
	}
	return GenerateConfigFromMap(config)
}

// BuildConfigMap generates the default dynamic configuration map.
func BuildConfigMap(proxies []ProxyNode) map[string]interface{} {
	var proxyNames []string
	for _, p := range proxies {
		if name, ok := p["name"].(string); ok {
			proxyNames = append(proxyNames, name)
		}
	}

	regionGroups := make(map[string][]string)
	regions := []string{"HK", "TW", "JP", "SG", "US", "KR"}
	for _, r := range regions {
		regionGroups[r] = []string{}
	}

	for _, name := range proxyNames {
		startRegions := classifyProxyName(name)
		for region, matched := range startRegions {
			if matched {
				regionGroups[region] = append(regionGroups[region], name)
			}
		}
	}

	config := map[string]interface{}{
		// ===== General Settings =====
		"mixed-port":          7890,
		"allow-lan":           true,
		"bind-address":        "*",
		"mode":                "rule",
		"log-level":           "info",
		"ipv6":                false,
		"external-controller": "0.0.0.0:9090",
		// ===== DNS =====
		"dns": map[string]interface{}{
			"enable":             true,
			"ipv6":               false,
			"listen":             "0.0.0.0:53",
			"default-nameserver": []string{"223.5.5.5", "119.29.29.29"},
			"enhanced-mode":      "fake-ip",
			"fake-ip-range":      "198.18.0.1/16",
			"fake-ip-filter": []string{
				"*.lan", "*.local", "*.crashlytics.com", "localhost.ptlogin2.qq.com",
				"+.srv.nintendo.net", "+.stun.playstation.net", "xbox.*.microsoft.com",
				"+.xboxlive.com", "+.msftconnecttest.com", "+.msftncsi.com",
			},
			"nameserver": []string{
				"https://doh.pub/dns-query",
				"https://dns.alidns.com/dns-query",
			},
			"fallback": []string{
				"https://1.1.1.1/dns-query",
				"https://dns.google/dns-query",
			},
			"fallback-filter": map[string]interface{}{
				"geoip":      true,
				"geoip-code": "CN",
				"ipcidr":     []string{"240.0.0.0/4", "0.0.0.0/32"},
			},
		},
		// ===== Proxies =====
		"proxies": proxies,
		// ===== Proxy Groups =====
		"proxy-groups": buildProxyGroups(proxyNames, regionGroups),
		// ===== Rules =====
		"rules": buildRules(),
	}

	return config
}

// GenerateConfigFromMap encodes a config map into YAML bytes.
func GenerateConfigFromMap(config map[string]interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	err := enc.Encode(config)
	return buf.Bytes(), err
}

func toStringMap(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed, true
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(typed))
		for k, v := range typed {
			key, ok := k.(string)
			if !ok {
				continue
			}
			out[key] = v
		}
		return out, true
	default:
		return nil, false
	}
}

func cloneValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for k, v := range typed {
			out[k] = cloneValue(v)
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(typed))
		for k, v := range typed {
			key, ok := k.(string)
			if !ok {
				continue
			}
			out[key] = cloneValue(v)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, v := range typed {
			out = append(out, cloneValue(v))
		}
		return out
	default:
		return value
	}
}

func toInterfaceSlice(value interface{}) ([]interface{}, bool) {
	if value == nil {
		return nil, false
	}

	switch typed := value.(type) {
	case []interface{}:
		return typed, true
	}

	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}

	out := make([]interface{}, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}

func mergeSlices(base, override []interface{}) []interface{} {
	out := make([]interface{}, 0, len(base)+len(override))
	for _, item := range override {
		out = append(out, cloneValue(item))
	}
	for _, item := range base {
		out = append(out, cloneValue(item))
	}
	return out
}

// DeepMergeMap deep merges override into base. Map values are merged recursively.
// Map values are merged recursively; arrays are concatenated (override + base);
// other value types are replaced by override.
func DeepMergeMap(base, override map[string]interface{}) map[string]interface{} {
	if base == nil && override == nil {
		return map[string]interface{}{}
	}
	if base == nil {
		if override == nil {
			return map[string]interface{}{}
		}
		cloned, _ := cloneValue(override).(map[string]interface{})
		return cloned
	}
	if override == nil {
		cloned, _ := cloneValue(base).(map[string]interface{})
		return cloned
	}

	result := make(map[string]interface{}, len(base))
	for key, baseValue := range base {
		result[key] = cloneValue(baseValue)
	}

	for key, overrideValue := range override {
		currentValue, exists := result[key]
		overrideMap, overrideIsMap := toStringMap(overrideValue)
		currentMap, currentIsMap := toStringMap(currentValue)
		overrideSlice, overrideIsSlice := toInterfaceSlice(overrideValue)
		currentSlice, currentIsSlice := toInterfaceSlice(currentValue)

		if exists && currentIsMap && overrideIsMap {
			result[key] = DeepMergeMap(currentMap, overrideMap)
			continue
		}
		if exists && currentIsSlice && overrideIsSlice {
			result[key] = mergeSlices(currentSlice, overrideSlice)
			continue
		}
		result[key] = cloneValue(overrideValue)
	}

	return result
}

func classifyProxyName(name string) map[string]bool {
	lower := strings.ToLower(name)
	hasKeyword := func(keywords ...string) bool {
		for _, keyword := range keywords {
			if strings.Contains(lower, keyword) {
				return true
			}
		}
		return false
	}
	matchCodeToken := func(code string) bool {
		re := regexp.MustCompile(`(?i)(^|[^a-z0-9])` + regexp.QuoteMeta(code) + `([^a-z0-9]|$)`)
		return re.MatchString(name)
	}

	return map[string]bool{
		"HK": hasKeyword("🇭🇰", "香港", "hong kong") || matchCodeToken("HK"),
		"TW": hasKeyword("🇹🇼", "台湾", "taiwan") || matchCodeToken("TW"),
		"JP": hasKeyword("🇯🇵", "日本", "japan") || matchCodeToken("JP"),
		"SG": hasKeyword("🇸🇬", "新加坡", "狮城", "坡县", "singapore") || matchCodeToken("SG") || matchCodeToken("SGP"),
		"US": hasKeyword("🇺🇸", "美国", "united states", "america") || matchCodeToken("US") || matchCodeToken("USA"),
		"KR": hasKeyword("🇰🇷", "韩国", "korea") || matchCodeToken("KR"),
	}
}

func buildProxyGroups(proxyNames []string, regionGroups map[string][]string) []map[string]interface{} {
	var groups []map[string]interface{}

	// Node Select
	groups = append(groups, map[string]interface{}{
		"name": "Proxies",
		"type": "select",
		"proxies": append([]string{
			"自动选择", "香港节点", "台湾节点", "日本节点",
			"新加坡节点", "美国节点", "韩国节点", "DIRECT",
		}, proxyNames...),
	})

	// Auto Select
	groups = append(groups, map[string]interface{}{
		"name":      "自动选择",
		"type":      "url-test",
		"proxies":   proxyNames,
		"url":       "http://www.gstatic.com/generate_204",
		"interval":  300,
		"tolerance": 50,
	})

	// Region Groups
	regionConfigs := []struct {
		Name string
		Key  string
	}{
		{"香港节点", "HK"},
		{"台湾节点", "TW"},
		{"日本节点", "JP"},
		{"新加坡节点", "SG"},
		{"美国节点", "US"},
		{"韩国节点", "KR"},
	}

	for _, rc := range regionConfigs {
		members := regionGroups[rc.Key]
		if len(members) > 0 {
			groups = append(groups, map[string]interface{}{
				"name":      rc.Name,
				"type":      "url-test",
				"proxies":   members,
				"url":       "http://www.gstatic.com/generate_204",
				"interval":  300,
				"tolerance": 50,
			})
		} else {
			// Fallback if no nodes for region
			groups = append(groups, map[string]interface{}{
				"name":    rc.Name,
				"type":    "select",
				"proxies": []string{"自动选择", "DIRECT"},
			})
		}
	}

	// Application Groups
	groups = append(groups, map[string]interface{}{
		"name": "YouTube",
		"type": "select",
		"proxies": []string{
			"Proxies", "香港节点", "日本节点", "新加坡节点", "台湾节点", "美国节点",
		},
	})
	groups = append(groups, map[string]interface{}{
		"name": "Disney",
		"type": "select",
		"proxies": []string{
			"Proxies", "香港节点", "日本节点", "新加坡节点", "台湾节点", "美国节点",
		},
	})
	groups = append(groups, map[string]interface{}{
		"name": "Hbomax",
		"type": "select",
		"proxies": []string{
			"Proxies", "香港节点", "日本节点", "新加坡节点", "台湾节点", "美国节点",
		},
	})
	groups = append(groups, map[string]interface{}{
		"name": "Netflix",
		"type": "select",
		"proxies": []string{
			"Proxies", "香港节点", "日本节点", "新加坡节点", "台湾节点", "美国节点",
		},
	})
	groups = append(groups, map[string]interface{}{
		"name": "Bahamut",
		"type": "select",
		"proxies": []string{
			"Proxies", "香港节点", "台湾节点",
		},
	})
	groups = append(groups, map[string]interface{}{
		"name": "Bilibili",
		"type": "select",
		"proxies": []string{
			"DIRECT", "香港节点", "台湾节点",
		},
	})
	groups = append(groups, map[string]interface{}{
		"name": "Spotify",
		"type": "select",
		"proxies": []string{
			"Proxies", "DIRECT", "香港节点", "日本节点", "新加坡节点", "台湾节点", "美国节点",
		},
	})
	groups = append(groups, map[string]interface{}{
		"name": "Steam",
		"type": "select",
		"proxies": []string{
			"Proxies", "DIRECT", "香港节点", "日本节点", "新加坡节点", "台湾节点", "美国节点",
		},
	})
	groups = append(groups, map[string]interface{}{
		"name": "Telegram",
		"type": "select",
		"proxies": []string{
			"Proxies", "香港节点", "日本节点", "新加坡节点", "台湾节点", "美国节点",
		},
	})
	groups = append(groups, map[string]interface{}{
		"name": "Google",
		"type": "select",
		"proxies": []string{
			"Proxies", "香港节点", "日本节点", "新加坡节点", "台湾节点", "美国节点",
		},
	})
	groups = append(groups, map[string]interface{}{
		"name": "Microsoft",
		"type": "select",
		"proxies": []string{
			"Proxies", "DIRECT", "香港节点", "日本节点", "新加坡节点", "台湾节点", "美国节点",
		},
	})
	groups = append(groups, map[string]interface{}{
		"name": "OpenAI",
		"type": "select",
		"proxies": []string{
			"Proxies", "香港节点", "日本节点", "新加坡节点", "台湾节点", "美国节点",
		},
	})
	groups = append(groups, map[string]interface{}{
		"name": "PayPal",
		"type": "select",
		"proxies": []string{
			"Proxies", "DIRECT", "香港节点", "日本节点", "新加坡节点", "台湾节点", "美国节点",
		},
	})
	groups = append(groups, map[string]interface{}{
		"name": "Apple",
		"type": "select",
		"proxies": []string{
			"Proxies", "DIRECT", "香港节点", "日本节点", "新加坡节点", "台湾节点", "美国节点",
		},
	})

	// Final Fallback
	groups = append(groups, map[string]interface{}{
		"name":    "Final",
		"type":    "select",
		"proxies": []string{"Proxies", "DIRECT"},
	})

	return groups
}

func buildRules() []string {
	return []string{}
}
